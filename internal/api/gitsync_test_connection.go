package api

import (
	"context"
	"encoding/json"
	"net/http"
	"time"

	"github.com/vazra/simpledeploy/internal/auth"
	"github.com/vazra/simpledeploy/internal/gitsync"
)

type testConnResponse struct {
	OK          bool   `json:"ok"`
	Code        string `json:"code"`
	Message     string `json:"message"`
	BranchFound bool   `json:"branch_found"`
	RawError    string `json:"raw_error"`
}

var testConnMessages = map[string]string{
	"ok":             "Connected. Branch found on remote.",
	"auth_failed":    "Authentication failed. Check token or SSH key.",
	"not_found":      "Repository not found at the given remote URL.",
	"branch_missing": "Connected, but the configured branch does not exist on the remote.",
	"network":        "Could not reach remote (network error).",
	"unknown":        "Connection failed.",
}

func (s *Server) handleTestGitConnection(w http.ResponseWriter, r *http.Request) {
	var req gitConfigRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid JSON", http.StatusBadRequest)
		return
	}
	if req.Remote == "" {
		http.Error(w, "remote is required", http.StatusBadRequest)
		return
	}

	existing, err := s.store.GetGitSyncConfig()
	if err != nil {
		httpError(w, err, http.StatusInternalServerError)
		return
	}

	probe := gitsync.Config{
		Remote:        req.Remote,
		Branch:        req.Branch,
		SSHKeyPath:    req.SSHKeyPath,
		HTTPSUsername: req.HTTPSUsername,
	}
	switch {
	case req.HTTPSToken == nil:
		if existing["https_token_enc"] != "" {
			dec, decErr := auth.Decrypt(existing["https_token_enc"], s.masterSecret)
			if decErr != nil {
				resp := testConnResponse{
					OK:      false,
					Code:    "unknown",
					Message: "Could not decrypt stored token. Re-enter the token to continue.",
				}
				w.Header().Set("Content-Type", "application/json")
				json.NewEncoder(w).Encode(resp)
				return
			}
			probe.HTTPSToken = dec
		}
	case *req.HTTPSToken != "":
		probe.HTTPSToken = *req.HTTPSToken
	}

	ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
	defer cancel()

	resCh := make(chan gitsync.RemoteCheck, 1)
	go func() { resCh <- gitsync.CheckRemote(probe) }()

	var res gitsync.RemoteCheck
	select {
	case res = <-resCh:
	case <-ctx.Done():
		res = gitsync.RemoteCheck{Code: "network", RawError: "timed out after 10s"}
	}

	msg, ok := testConnMessages[res.Code]
	if !ok {
		msg = testConnMessages["unknown"]
	}
	resp := testConnResponse{
		OK:          res.OK,
		Code:        res.Code,
		Message:     msg,
		BranchFound: res.BranchFound,
		RawError:    res.RawError,
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(resp)
}
