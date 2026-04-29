package api

import (
	"net/http"
	"time"

	"github.com/gorilla/websocket"
	"github.com/vazra/simpledeploy/internal/deployer"
)

const (
	deployLogReadDeadline = 5 * time.Minute
	deployLogPingInterval = 60 * time.Second
	deployLogWriteTimeout = 10 * time.Second
)

func (s *Server) handleDeployLogs(w http.ResponseWriter, r *http.Request) {
	slug := r.PathValue("slug")

	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		return
	}
	defer conn.Close()

	// Browsers do not auto-ping, so without a server-driven keepalive a
	// long deploy (slow image pull) would hit our 5min read deadline and
	// the connection would die mid-deploy. Refresh the deadline whenever
	// a pong arrives, and drive that pong by emitting a ping every minute.
	conn.SetReadLimit(wsMaxFrameSize)
	conn.SetReadDeadline(time.Now().Add(deployLogReadDeadline))
	conn.SetPongHandler(func(string) error {
		conn.SetReadDeadline(time.Now().Add(deployLogReadDeadline))
		return nil
	})

	pingDone := make(chan struct{})
	defer close(pingDone)
	go func() {
		ticker := time.NewTicker(deployLogPingInterval)
		defer ticker.Stop()
		for {
			select {
			case <-pingDone:
				return
			case <-ticker.C:
				_ = conn.WriteControl(websocket.PingMessage, nil, time.Now().Add(deployLogWriteTimeout))
			}
		}
	}()

	// Wait up to 3s for deploy to start (race between async POST and WS connect)
	var ch <-chan deployer.OutputLine
	var unsub func()
	var ok bool
	for range 30 {
		ch, unsub, ok = s.reconciler.SubscribeDeployLog(slug)
		if ok {
			break
		}
		time.Sleep(100 * time.Millisecond)
	}
	if !ok {
		conn.WriteJSON(deployer.OutputLine{Done: true, Action: "none"})
		return
	}
	defer unsub()

	// drain reads to detect disconnect (and let pong handler fire)
	go func() {
		for {
			if _, _, err := conn.ReadMessage(); err != nil {
				return
			}
		}
	}()

	for line := range ch {
		if err := conn.WriteJSON(line); err != nil {
			return
		}
		if line.Done {
			return
		}
	}
}
