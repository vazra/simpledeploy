package api

import (
	"bufio"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"github.com/vazra/simpledeploy/internal/compose"
	"github.com/vazra/simpledeploy/internal/fsutil"
	"github.com/vazra/simpledeploy/internal/store"
)

type envVar struct {
	Key   string `json:"key"`
	Value string `json:"value"`
	// Masked is set (with an empty Value) when the caller may see which
	// variables exist but not their values (viewer role).
	Masked bool `json:"masked,omitempty"`
}

// canSeeEnvValues reports whether role may read .env values. Values are
// app secrets, so only roles that can also change them get them.
func canSeeEnvValues(role string) bool {
	return role == "super_admin" || role == "manage"
}

// envFilePath returns the app's .env path after verifying that neither it
// nor its app directory is a symlink, so reads and writes cannot be
// redirected outside the apps dir.
func (s *Server) envFilePath(app *store.App) (string, error) {
	appDir := filepath.Dir(app.ComposePath)
	envPath := filepath.Join(appDir, ".env")
	root := s.appsDir
	if root == "" {
		root = appDir
	}
	if err := fsutil.EnsureNoSymlinks(root, envPath); err != nil {
		return "", err
	}
	return envPath, nil
}

// validateEnvVars rejects entries that would corrupt or smuggle additional
// lines into the .env file: empty/invalid keys, keys containing whitespace
// or `=`, and values containing CR/LF/NUL.
func validateEnvVars(vars []envVar) error {
	for i, v := range vars {
		if v.Key == "" {
			return fmt.Errorf("entry %d: empty key", i)
		}
		if strings.ContainsAny(v.Key, " \t\r\n\x00=") {
			return fmt.Errorf("entry %d: key contains invalid character", i)
		}
		if strings.ContainsAny(v.Value, "\r\n\x00") {
			return fmt.Errorf("entry %d: value contains newline or NUL", i)
		}
		if v.Masked {
			// Saving a masked entry would blank the real value.
			return fmt.Errorf("entry %d: masked value cannot be saved", i)
		}
	}
	return nil
}

// parseEnvFile reads a .env file. It refuses symlinks and non-regular
// files; a missing file returns an error satisfying os.IsNotExist.
func parseEnvFile(path string) ([]envVar, error) {
	f, err := fsutil.OpenRegularFile(path)
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

// envFileContent renders vars as .env file content.
func envFileContent(vars []envVar) []byte {
	var b strings.Builder
	for _, v := range vars {
		b.WriteString(v.Key + "=" + v.Value + "\n")
	}
	return []byte(b.String())
}

// writeEnvFile atomically replaces the .env file (temp file + rename, so a
// symlink at path is replaced rather than followed).
func writeEnvFile(path string, content []byte) error {
	// 0600: .env files contain app secrets; only the simpledeploy/docker
	// daemon user should read them.
	return fsutil.WriteFileAtomic(path, content, 0o600)
}

// checkEnvAgainstCompose parses the app's compose file with the new .env
// content: ${VAR} values can change what runs and which domains the app
// routes, so they get the same checks as a compose edit. Writes a response
// and returns false when the values are refused. Without a compose file
// there is nothing to check; deploy validates with the .env later.
func (s *Server) checkEnvAgainstCompose(w http.ResponseWriter, r *http.Request, app *store.App, dotEnv []byte) bool {
	composeData, err := fsutil.ReadRegularFile(app.ComposePath)
	if errors.Is(err, os.ErrNotExist) {
		return true
	}
	if err != nil {
		log.Printf("[env] %s: read compose: %v", app.Slug, err)
		if errors.Is(err, fsutil.ErrNotRegular) {
			http.Error(w, "docker-compose.yml is a link or not a regular file; replace it with a regular file before changing variables", http.StatusConflict)
			return false
		}
		http.Error(w, "failed to read compose file", http.StatusInternalServerError)
		return false
	}
	parsed, violations, err := checkComposeWithEnv(composeData, app, dotEnv)
	if err != nil || len(violations) > 0 {
		if errors.Is(err, compose.ErrDotEnv) {
			writeViolations(w, http.StatusBadRequest, "docker compose cannot read these values (for example a value with an unclosed quote)", []string{err.Error()})
			return false
		}
		// Not the new values' fault when the compose file already fails.
		if refuseEnvEditOnUnsafeCompose(w, app, composeData) {
			return false
		}
		if err != nil {
			writeViolations(w, http.StatusBadRequest, "the compose file does not load with these values", []string{err.Error()})
		} else {
			writeViolations(w, http.StatusBadRequest, "with these values the compose file contains disallowed directives", violations)
		}
		return false
	}
	var current []compose.EndpointConfig
	if cfg, err := parseComposeForDisplay(app.ComposePath, app.Slug); err == nil {
		current = cfg.Endpoints
	}
	if code, msg := s.checkEndpointDomains(r, app.Slug, parsed.Endpoints, current); code != 0 {
		http.Error(w, msg, code)
		return false
	}
	return true
}

// checkComposeWithEnv parses composeData with dotEnv as the app's .env and
// returns its rule violations. err is set when it does not load.
func checkComposeWithEnv(composeData []byte, app *store.App, dotEnv []byte) (*compose.AppConfig, []string, error) {
	parsed, err := compose.ParseContent(composeData, app.ComposePath, app.Slug, dotEnv)
	if v, ok := violationsOf(err); ok {
		return nil, v, nil
	}
	if err != nil {
		return nil, nil, err
	}
	return parsed, compose.ValidateComposeSecurity(parsed), nil
}

// refuseEnvEditOnUnsafeCompose writes the 409 other edits get on an unsafe
// compose file when the compose already fails with the current .env. An
// unusable current .env counts as empty, since this edit may fix it.
// Returns true when a response was written.
func refuseEnvEditOnUnsafeCompose(w http.ResponseWriter, app *store.App, composeData []byte) bool {
	current, err := compose.ReadDotEnv(filepath.Dir(app.ComposePath))
	if err != nil {
		current = nil
	}
	_, violations, err := checkComposeWithEnv(composeData, app, current)
	if errors.Is(err, compose.ErrDotEnv) {
		_, violations, err = checkComposeWithEnv(composeData, app, nil)
	}
	switch {
	case len(violations) > 0:
		writeViolations(w, http.StatusConflict, unsafeComposeEdit, violations)
	case err != nil:
		http.Error(w, "this app's compose file could not be read; fix it and redeploy before changing these settings", http.StatusConflict)
	default:
		return false
	}
	return true
}

func (s *Server) handleGetEnv(w http.ResponseWriter, r *http.Request) {
	slug := r.PathValue("slug")
	app, err := s.store.GetAppBySlug(slug)
	if err != nil {
		http.Error(w, "app not found", http.StatusNotFound)
		return
	}

	envPath, err := s.envFilePath(app)
	if err != nil {
		log.Printf("[env] %s: %v", slug, err)
		http.Error(w, "failed to read .env", http.StatusInternalServerError)
		return
	}
	vars, err := parseEnvFile(envPath)
	if err != nil {
		if os.IsNotExist(err) {
			w.Header().Set("Content-Type", "application/json")
			w.Write([]byte("[]"))
			return
		}
		log.Printf("[env] read %s: %v", slug, err)
		http.Error(w, "failed to read .env", http.StatusInternalServerError)
		return
	}

	if vars == nil {
		vars = []envVar{}
	}
	if user := GetAuthUser(r); user == nil || !canSeeEnvValues(user.Role) {
		for i := range vars {
			vars[i] = envVar{Key: vars[i].Key, Masked: true}
		}
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
	if err := validateEnvVars(vars); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	envPath, err := s.envFilePath(app)
	if err != nil {
		log.Printf("[env] %s: %v", slug, err)
		http.Error(w, "failed to write .env", http.StatusInternalServerError)
		return
	}

	content := envFileContent(vars)
	if !s.checkEnvAgainstCompose(w, r, app, content) {
		return
	}

	// Capture before-keys for audit (key-only; values are not logged to avoid
	// leaking secrets into the audit trail).
	var beforeKeys []string
	if oldVars, err := parseEnvFile(envPath); err == nil {
		for _, v := range oldVars {
			beforeKeys = append(beforeKeys, v.Key)
		}
	}

	if err := writeEnvFile(envPath, content); err != nil {
		log.Printf("[env] write %s: %v", slug, err)
		http.Error(w, "failed to write .env", http.StatusInternalServerError)
		return
	}
	s.EnqueueGitCommit([]string{envPath}, "env:"+slug)

	afterKeys := make([]string, 0, len(vars))
	for _, v := range vars {
		afterKeys = append(afterKeys, v.Key)
	}
	s.recordAudit(r, app, "env", "changed",
		map[string]any{"keys": beforeKeys},
		map[string]any{"keys": afterKeys},
	)

	w.Header().Set("Content-Type", "application/json")
	w.Write([]byte(`{"status":"ok"}`))
}
