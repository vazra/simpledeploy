package api

import (
	"context"
	"encoding/binary"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/api/types/filters"
	"github.com/gorilla/websocket"
)

var upgrader = websocket.Upgrader{
	CheckOrigin: checkWebSocketOrigin,
}

// checkWebSocketOrigin validates the Origin header for WebSocket connections.
// Allows same-origin requests and rejects cross-origin requests.
func checkWebSocketOrigin(r *http.Request) bool {
	origin := r.Header.Get("Origin")
	if origin == "" {
		return true // non-browser clients
	}
	host := r.Host
	// Strip port from origin for comparison
	originHost := origin
	for _, prefix := range []string{"https://", "http://"} {
		originHost = strings.TrimPrefix(originHost, prefix)
	}
	// Strip port for comparison if present
	if i := strings.LastIndex(originHost, ":"); i != -1 {
		originHost = originHost[:i]
	}
	hostOnly := host
	if i := strings.LastIndex(hostOnly, ":"); i != -1 {
		hostOnly = hostOnly[:i]
	}
	return originHost == hostOnly
}

func (s *Server) handleLogs(w http.ResponseWriter, r *http.Request) {
	slug := r.PathValue("slug")

	_, err := s.store.GetAppBySlug(slug)
	if err != nil {
		http.Error(w, "not found", http.StatusNotFound)
		return
	}

	follow := r.URL.Query().Get("follow") != "false"
	tail := r.URL.Query().Get("tail")
	if tail == "" {
		tail = "100"
	}
	since := r.URL.Query().Get("since")
	service := r.URL.Query().Get("service")
	if service == "" {
		service = "web"
	}

	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		return
	}
	defer conn.Close()

	conn.SetReadDeadline(time.Now().Add(5 * time.Minute))
	conn.SetPongHandler(func(string) error {
		conn.SetReadDeadline(time.Now().Add(5 * time.Minute))
		return nil
	})

	ctx, cancel := context.WithCancel(r.Context())
	defer cancel()

	go func() {
		for {
			if _, _, err := conn.ReadMessage(); err != nil {
				cancel()
				return
			}
		}
	}()

	// Find container by compose labels
	project := fmt.Sprintf("simpledeploy-%s", slug)
	f := filters.NewArgs(
		filters.Arg("label", fmt.Sprintf("com.docker.compose.project=%s", project)),
		filters.Arg("label", fmt.Sprintf("com.docker.compose.service=%s", service)),
	)
	ctrs, err := s.docker.ContainerList(ctx, container.ListOptions{Filters: f, All: true})
	if err != nil || len(ctrs) == 0 {
		conn.WriteJSON(map[string]string{"error": "container not found"})
		return
	}
	containerID := ctrs[0].ID

	logOpts := container.LogsOptions{
		ShowStdout: true, ShowStderr: true,
		Follow: follow, Tail: tail, Timestamps: true,
	}
	if since != "" {
		logOpts.Since = since
	}

	reader, err := s.docker.ContainerLogs(ctx, containerID, logOpts)
	if err != nil {
		conn.WriteJSON(map[string]string{"error": err.Error()})
		return
	}
	defer reader.Close()

	hdr := make([]byte, 8)
	for {
		_, err := io.ReadFull(reader, hdr)
		if err != nil {
			break
		}

		streamType := "stdout"
		if hdr[0] == 2 {
			streamType = "stderr"
		}

		size := binary.BigEndian.Uint32(hdr[4:8])
		frame := make([]byte, size)
		_, err = io.ReadFull(reader, frame)
		if err != nil {
			break
		}

		lineStr := strings.TrimRight(string(frame), "\n")
		msg := map[string]string{"stream": streamType, "line": lineStr}

		if idx := strings.Index(lineStr, " "); idx > 20 {
			msg["ts"] = lineStr[:idx]
			msg["line"] = strings.TrimRight(lineStr[idx+1:], "\n")
		}

		if err := conn.WriteJSON(msg); err != nil {
			break
		}
	}
}
