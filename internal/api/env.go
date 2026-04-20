package api

import (
	"bufio"
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
	"strings"
)

type envVar struct {
	Key   string `json:"key"`
	Value string `json:"value"`
}

func parseEnvFile(path string) ([]envVar, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	var vars []envVar
	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		idx := strings.IndexByte(line, '=')
		if idx < 0 {
			continue
		}
		vars = append(vars, envVar{
			Key:   line[:idx],
			Value: line[idx+1:],
		})
	}
	return vars, scanner.Err()
}

func writeEnvFile(path string, vars []envVar) error {
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	defer f.Close()

	w := bufio.NewWriter(f)
	for _, v := range vars {
		if _, err := w.WriteString(v.Key + "=" + v.Value + "\n"); err != nil {
			return err
		}
	}
	return w.Flush()
}

func (s *Server) handleGetEnv(w http.ResponseWriter, r *http.Request) {
	slug := r.PathValue("slug")
	app, err := s.store.GetAppBySlug(slug)
	if err != nil {
		http.Error(w, "app not found", http.StatusNotFound)
		return
	}

	envPath := filepath.Join(filepath.Dir(app.ComposePath), ".env")
	vars, err := parseEnvFile(envPath)
	if err != nil {
		if os.IsNotExist(err) {
			w.Header().Set("Content-Type", "application/json")
			w.Write([]byte("[]"))
			return
		}
		http.Error(w, "failed to read .env", http.StatusInternalServerError)
		return
	}

	if vars == nil {
		vars = []envVar{}
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(vars)
}

func (s *Server) handlePutEnv(w http.ResponseWriter, r *http.Request) {
	slug := r.PathValue("slug")
	app, err := s.store.GetAppBySlug(slug)
	if err != nil {
		http.Error(w, "app not found", http.StatusNotFound)
		return
	}

	var vars []envVar
	if err := json.NewDecoder(r.Body).Decode(&vars); err != nil {
		http.Error(w, "invalid request body", http.StatusBadRequest)
		return
	}

	envPath := filepath.Join(filepath.Dir(app.ComposePath), ".env")
	if err := writeEnvFile(envPath, vars); err != nil {
		http.Error(w, "failed to write .env", http.StatusInternalServerError)
		return
	}
	s.EnqueueGitCommit([]string{envPath}, "env:"+slug)

	w.Header().Set("Content-Type", "application/json")
	w.Write([]byte(`{"status":"ok"}`))
}
