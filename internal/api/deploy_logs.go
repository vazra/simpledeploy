package api

import (
	"net/http"
	"time"

	"github.com/vazra/simpledeploy/internal/deployer"
)

func (s *Server) handleDeployLogs(w http.ResponseWriter, r *http.Request) {
	slug := r.PathValue("slug")

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

	// drain reads to detect disconnect
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
