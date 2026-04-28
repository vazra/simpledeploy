package api

import (
	"encoding/json"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/gorilla/websocket"
	"github.com/vazra/simpledeploy/internal/events"
)

// inboundFrame is a client-to-server WS message.
type inboundFrame struct {
	Op    string `json:"op"`
	Topic string `json:"topic"`
}

// outboundFrame is a server-to-client WS message. Either Type or Op is set.
type outboundFrame struct {
	Type   string    `json:"type,omitempty"`
	Op     string    `json:"op,omitempty"`
	Topic  string    `json:"topic,omitempty"`
	Reason string    `json:"reason,omitempty"`
	Ts     time.Time `json:"ts,omitempty"`
}

const (
	wsWriteWait    = 10 * time.Second
	wsPongWait     = 60 * time.Second
	wsPingPeriod   = 30 * time.Second
	wsMaxFrameSize = 1 << 14
)

// allowedTopics computes the topic ACL for a given user. super_admin sees all.
// admin/regular see global app/backup/alert/audit topics plus app:<slug> for
// every slug they have access to.
func (s *Server) allowedTopics(user *AuthUser) map[string]bool {
	out := map[string]bool{
		events.TopicGlobalApps:    true,
		events.TopicGlobalBackups: true,
		events.TopicGlobalAlerts:  true,
		events.TopicGlobalAudit:   true,
	}
	if user.Role == "super_admin" {
		out[events.TopicGlobalUsers] = true
		out[events.TopicGlobalRegistries] = true
		out[events.TopicGlobalSettings] = true
		out[events.TopicGlobalDocker] = true
	}
	return out
}

// canSubscribeApp checks whether the user can subscribe to app:<slug>.
func (s *Server) canSubscribeApp(user *AuthUser, slug string) bool {
	if user.Role == "super_admin" {
		return true
	}
	ok, _ := s.store.HasAppAccess(user.ID, slug)
	return ok
}

// authorizeTopic returns true if the user is currently allowed to subscribe to
// the given topic.
func (s *Server) authorizeTopic(user *AuthUser, topic string, allowed map[string]bool) bool {
	if allowed[topic] {
		return true
	}
	if strings.HasPrefix(topic, "app:") {
		return s.canSubscribeApp(user, strings.TrimPrefix(topic, "app:"))
	}
	return false
}

// handleEventsWS is the realtime notify-only WebSocket. Clients subscribe to
// topics; server pushes type+topic frames. No payload data flows over the
// socket; UI refetches via REST.
func (s *Server) handleEventsWS(w http.ResponseWriter, r *http.Request) {
	user := GetAuthUser(r)
	if user == nil {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}
	if s.bus == nil {
		http.Error(w, "events bus unavailable", http.StatusServiceUnavailable)
		return
	}

	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		return
	}
	defer conn.Close()

	conn.SetReadLimit(wsMaxFrameSize)
	conn.SetReadDeadline(time.Now().Add(wsPongWait))
	conn.SetPongHandler(func(string) error {
		conn.SetReadDeadline(time.Now().Add(wsPongWait))
		return nil
	})

	allowed := s.allowedTopics(user)

	var (
		mu       sync.Mutex // protects subs and conn writes
		subs     = map[string]bool{}
		closeReq = make(chan struct{}, 1)
	)
	uid := user.ID

	// Filter: deliver only events whose topic this conn is subscribed to.
	filter := func(e events.Event) bool {
		mu.Lock()
		defer mu.Unlock()
		return subs[e.Topic]
	}
	ch, cancel, sub := s.bus.Subscribe(filter)
	defer cancel()

	writeFrame := func(f outboundFrame) error {
		mu.Lock()
		defer mu.Unlock()
		conn.SetWriteDeadline(time.Now().Add(wsWriteWait))
		return conn.WriteJSON(f)
	}

	// Read pump: parse client frames.
	go func() {
		defer func() {
			select {
			case closeReq <- struct{}{}:
			default:
			}
		}()
		for {
			_, data, err := conn.ReadMessage()
			if err != nil {
				return
			}
			var in inboundFrame
			if err := json.Unmarshal(data, &in); err != nil {
				continue
			}
			switch in.Op {
			case "ping":
				_ = writeFrame(outboundFrame{Op: "pong"})
			case "sub":
				if in.Topic == "" {
					continue
				}
				if !s.authorizeTopic(user, in.Topic, allowed) {
					_ = writeFrame(outboundFrame{Op: "err", Topic: in.Topic, Reason: "forbidden"})
					continue
				}
				mu.Lock()
				subs[in.Topic] = true
				mu.Unlock()
			case "unsub":
				mu.Lock()
				delete(subs, in.Topic)
				mu.Unlock()
			}
		}
	}()

	// Ping ticker.
	pingT := time.NewTicker(wsPingPeriod)
	defer pingT.Stop()

	for {
		select {
		case <-closeReq:
			return
		case <-r.Context().Done():
			return
		case <-pingT.C:
			mu.Lock()
			conn.SetWriteDeadline(time.Now().Add(wsWriteWait))
			err := conn.WriteMessage(websocket.PingMessage, nil)
			mu.Unlock()
			if err != nil {
				return
			}
		case e, ok := <-ch:
			if !ok {
				return
			}
			// Authz revocation: close on access/user changes touching this user.
			if e.Topic == events.TopicGlobalUsers && e.ActorID != nil && *e.ActorID == uid {
				// Always emit then close so the client can reconnect.
				_ = writeFrame(outboundFrame{Type: e.Type, Topic: e.Topic, Ts: e.Ts})
				return
			}
			if err := writeFrame(outboundFrame{Type: e.Type, Topic: e.Topic, Ts: e.Ts}); err != nil {
				return
			}
			if sub.Stale() {
				sub.Reset()
				_ = writeFrame(outboundFrame{Type: "resync", Topic: "*"})
			}
		}
	}
}
