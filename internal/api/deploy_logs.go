package api

import (
	"net/http"

	"github.com/vazra/simpledeploy/internal/deployer"
)

func (s *Server) handleDeployLogs(w http.ResponseWriter, r *http.Request) {
	slug := r.PathValue("slug")

	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		return
	}
	defer conn.Close()

	ch, unsub, ok := s.reconciler.SubscribeDeployLog(slug)
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
