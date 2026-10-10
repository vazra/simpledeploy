package api

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"net/url"
	"strings"
	"sync/atomic"
	"time"

	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/api/types/filters"
	"github.com/gorilla/websocket"
	"github.com/vazra/simpledeploy/internal/audit"
	"github.com/vazra/simpledeploy/internal/auth"
	"github.com/vazra/simpledeploy/internal/logbuf"
)

var upgrader = websocket.Upgrader{
	CheckOrigin: checkWebSocketOrigin,
}

// checkWebSocketOrigin validates the Origin header for WebSocket connections.
// Browsers always send Origin on WS upgrade; cookie-authed callers must
// match the request Host exactly, port included (another service on the
// same hostname but a different port is a different origin). Bearer-authed
// callers (CLI, curl, integrations) can omit Origin since they do not
// depend on cookie ambient credentials, so cross-origin CSRF is
// structurally impossible for them.
func checkWebSocketOrigin(r *http.Request) bool {
	origin := r.Header.Get("Origin")
	if origin == "" {
		// Allow only when the caller is Bearer-authed; cookie-authed
		// upgrades MUST present an Origin so we can compare it to Host.
		auth := r.Header.Get("Authorization")
		return strings.HasPrefix(auth, "Bearer ")
	}
	// Browser-computed and not settable by page scripts. Accepting it keeps
	// same-origin dashboards working behind proxies that drop the port
	// from Host (nginx "$host").
	if r.Header.Get("Sec-Fetch-Site") == "same-origin" {
		return true
	}
	u, err := url.Parse(origin)
	if err != nil || u.Host == "" {
		return false
	}
	return strings.EqualFold(hostWithPort(u.Host, u.Scheme), hostWithPort(r.Host, u.Scheme))
}

// hostWithPort returns host as host:port, filling in the default port of
// scheme when host has none, so Origin "https://example.com" matches Host
// "example.com" and "example.com:443" alike.
func hostWithPort(host, scheme string) string {
	if _, _, err := net.SplitHostPort(host); err == nil {
		return host
	}
	var port string
	switch strings.ToLower(scheme) {
	case "https", "wss":
		port = "443"
	case "http", "ws":
		port = "80"
	default:
		return host
	}
	return net.JoinHostPort(strings.TrimSuffix(strings.TrimPrefix(host, "["), "]"), port)
}

// wsAuthRecheckNanos is how often long-lived WebSockets re-validate the
// caller. Atomic so tests can shorten it while sockets from earlier tests
// are still winding down.
var wsAuthRecheckNanos atomic.Int64

func init() { wsAuthRecheckNanos.Store(int64(60 * time.Second)) }

func wsAuthRecheckInterval() time.Duration {
	return time.Duration(wsAuthRecheckNanos.Load())
}

// wsAuth records how a WebSocket was authenticated so the socket can be
// re-checked for its whole lifetime: it must not outlive a logout, password
// or role change (all bump token_version), API key revocation, or deletion
// of the user.
type wsAuth struct {
	user *AuthUser
	// session is the JWT of a cookie session. Its token version must keep
	// matching the user's.
	session string
	// apiKeyHash identifies the API key of a Bearer session. The key must
	// still exist and be unexpired.
	apiKeyHash string
}

// newWSAuth captures the caller's credentials. It returns nil for an
// unauthenticated request.
func (s *Server) newWSAuth(r *http.Request) *wsAuth {
	u := GetAuthUser(r)
	if u == nil {
		return nil
	}
	a := &wsAuth{user: u}
	// authMiddleware records which credential it accepted.
	switch audit.From(r.Context()).ActorSource {
	case "ui":
		if c, err := r.Cookie("session"); err == nil {
			a.session = c.Value
		}
	case "api":
		if h := r.Header.Get("Authorization"); strings.HasPrefix(h, "Bearer ") {
			a.apiKeyHash = auth.HashAPIKey(strings.TrimPrefix(h, "Bearer "), s.masterSecret)
		}
	}
	return a
}

// wsAuthStillValid re-runs the auth middleware's checks: the user still
// exists with the same role, and the session token or API key used to open
// the socket is still accepted.
func (s *Server) wsAuthStillValid(a *wsAuth) bool {
	u, err := s.store.GetUserByID(a.user.ID)
	if err != nil || u == nil || u.Role != a.user.Role {
		return false
	}
	if a.session != "" {
		if s.jwt == nil {
			return false
		}
		claims, err := s.jwt.Validate(a.session)
		if err != nil || claims.UserID != u.ID || claims.TokenVersion != u.TokenVersion {
			return false
		}
	}
	if a.apiKeyHash != "" {
		key, keyUser, err := s.store.GetAPIKeyByHash(a.apiKeyHash)
		if err != nil || key == nil || keyUser == nil || keyUser.ID != u.ID {
			return false
		}
		if key.ExpiresAt != nil && time.Now().After(*key.ExpiresAt) {
			return false
		}
	}
	return true
}

// wsCanAccessApp reports whether the socket's user may still see app slug.
func (s *Server) wsCanAccessApp(a *wsAuth, slug string) bool {
	if a.user.Role == "super_admin" {
		return true
	}
	ok, _ := s.store.HasAppAccess(a.user.ID, slug)
	return ok
}

// watchWSAuth runs check every wsAuthRecheckInterval() until ctx ends. When a
// check fails it sends a policy-violation close frame and calls cancel.
func watchWSAuth(ctx context.Context, conn *websocket.Conn, check func() bool, cancel context.CancelFunc) {
	t := time.NewTicker(wsAuthRecheckInterval())
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			if !check() {
				closeWSRevoked(conn)
				cancel()
				return
			}
		}
	}
}

// closeWSRevoked tells the client its authorization is gone. WriteControl
// is safe to call concurrently with other writers.
func closeWSRevoked(conn *websocket.Conn) {
	_ = conn.WriteControl(websocket.CloseMessage,
		websocket.FormatCloseMessage(websocket.ClosePolicyViolation, "authorization changed"),
		time.Now().Add(wsWriteWait))
}

// maxTTYLogLine caps one line of a raw (TTY) container log stream.
const maxTTYLogLine = 64 * 1024

// errLogClientGone means sending to the WebSocket client failed.
var errLogClientGone = errors.New("log client gone")

// streamContainerLogs reads a Docker log stream and calls send per entry.
// TTY streams are raw bytes split on newlines (long lines truncated).
// Other streams are demultiplexed; frames above logbuf.MaxDockerFrameSize
// end the stream with logbuf.ErrDockerFrameTooLarge instead of being
// allocated. A clean end of stream returns nil, a failed send
// errLogClientGone.
func streamContainerLogs(r io.Reader, tty bool, send func(stream, line string) error) error {
	if !tty {
		d := logbuf.NewDockerStreamReader(r)
		for {
			stream, payload, err := d.Next()
			if errors.Is(err, io.EOF) {
				return nil
			}
			if err != nil {
				return err
			}
			if err := send(stream, string(payload)); err != nil {
				return errLogClientGone
			}
		}
	}

	var sendErr error
	split := logbuf.NewLineSplitter(maxTTYLogLine, func(line []byte) {
		if sendErr == nil {
			sendErr = send("stdout", string(line))
		}
	})
	buf := make([]byte, 32*1024)
	for {
		n, err := r.Read(buf)
		if n > 0 {
			_, _ = split.Write(buf[:n])
		}
		if errors.Is(err, io.EOF) {
			split.Flush()
			err = nil
			if sendErr == nil {
				return nil
			}
		}
		if sendErr != nil {
			return errLogClientGone
		}
		if err != nil {
			return err
		}
	}
}

// logLineMessage builds the WS frame for one log entry, splitting off the
// Docker timestamp prefix when present.
func logLineMessage(stream, line string) map[string]string {
	line = strings.TrimRight(line, "\r\n")
	msg := map[string]string{"stream": stream, "line": line}
	if idx := strings.Index(line, " "); idx > 20 {
		msg["ts"] = line[:idx]
		msg["line"] = line[idx+1:]
	}
	return msg
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

	// Cap inbound frames; this WS is server-to-client only and should not
	// receive payloads of any size from the browser.
	conn.SetReadLimit(wsMaxFrameSize)
	conn.SetReadDeadline(time.Now().Add(5 * time.Minute))
	conn.SetPongHandler(func(string) error {
		conn.SetReadDeadline(time.Now().Add(5 * time.Minute))
		return nil
	})

	ctx, cancel := context.WithCancel(r.Context())
	defer cancel()

	// Server-driven keepalive: send a ping every 30s so the browser pong
	// refreshes the read deadline and the goroutines exit promptly when
	// the client disconnects abruptly. Closes audit L-15.
	pingTicker := time.NewTicker(30 * time.Second)
	defer pingTicker.Stop()
	go func() {
		for {
			select {
			case <-ctx.Done():
				return
			case <-pingTicker.C:
				_ = conn.WriteControl(websocket.PingMessage, nil, time.Now().Add(10*time.Second))
			}
		}
	}()

	// Periodic auth recheck: long-lived log streams should not outlive a
	// password change, role change, logout, API key revocation or lost app
	// access. Closes L-14.
	if a := s.newWSAuth(r); a != nil {
		go watchWSAuth(ctx, conn, func() bool {
			return s.wsAuthStillValid(a) && s.wsCanAccessApp(a, slug)
		}, cancel)
	}

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

	// Only non-TTY containers frame their logs; a TTY stream is raw bytes
	// and must not be parsed as 8-byte headers.
	info, err := s.docker.ContainerInspect(ctx, containerID)
	if err != nil {
		log.Printf("[logs] inspect container for %s/%s: %v", slug, service, err)
		conn.WriteJSON(map[string]string{"error": "could not read container logs"})
		return
	}
	tty := info.Config != nil && info.Config.Tty

	logOpts := container.LogsOptions{
		ShowStdout: true, ShowStderr: true,
		Follow: follow, Tail: tail, Timestamps: true,
	}
	if since != "" {
		logOpts.Since = since
	}

	reader, err := s.docker.ContainerLogs(ctx, containerID, logOpts)
	if err != nil {
		// Raw Docker errors can leak host details; log them, send a generic message.
		log.Printf("[logs] open logs for %s/%s: %v", slug, service, err)
		conn.WriteJSON(map[string]string{"error": "could not read container logs"})
		return
	}
	defer reader.Close()
	// Unblock a pending read when the client leaves or loses access.
	go func() {
		<-ctx.Done()
		reader.Close()
	}()

	err = streamContainerLogs(reader, tty, func(stream, line string) error {
		return conn.WriteJSON(logLineMessage(stream, line))
	})
	if err != nil && !errors.Is(err, errLogClientGone) && ctx.Err() == nil {
		log.Printf("[logs] stream for %s/%s: %v", slug, service, err)
		msg := "log stream ended unexpectedly"
		if errors.Is(err, logbuf.ErrDockerFrameTooLarge) {
			msg = "log stream closed: a log entry was larger than 1 MiB"
		}
		conn.WriteJSON(map[string]string{"error": msg})
	}
}
