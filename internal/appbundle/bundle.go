// Package appbundle builds and parses per-app config ZIP bundles
// used for export/import.
package appbundle

import (
	"archive/zip"
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// SchemaVersion is the current bundle schema version.
const SchemaVersion = 1

// File names inside the bundle.
const (
	fileCompose    = "docker-compose.yml"
	fileSidecar    = "simpledeploy.yml"
	fileEnvExample = "env.example"
	fileManifest   = "manifest.json"
	fileEnvSource  = ".env"
)

// Manifest is the bundle metadata.
type Manifest struct {
	SchemaVersion             int       `json:"schema_version"`
	ExportedAt                time.Time `json:"exported_at"`
	SourceSimpleDeployVersion string    `json:"source_simpledeploy_version"`
	App                       AppMeta   `json:"app"`
	Redacted                  []string  `json:"redacted"`
}

// AppMeta describes the source app.
type AppMeta struct {
	Slug        string `json:"slug"`
	DisplayName string `json:"display_name,omitempty"`
}

// Bundle is the parsed contents of a bundle ZIP.
type Bundle struct {
	Manifest   Manifest
	Compose    []byte
	Sidecar    []byte
	EnvExample []byte
}

// Build reads files from appDir and produces a ZIP byte slice.
func Build(appDir, slug, displayName, version string) ([]byte, error) {
	composePath := filepath.Join(appDir, fileCompose)
	composeBytes, err := os.ReadFile(composePath)
	if err != nil {
		return nil, fmt.Errorf("read docker-compose.yml: %w", err)
	}

	var sidecarBytes []byte
	if b, err := os.ReadFile(filepath.Join(appDir, fileSidecar)); err == nil {
		sidecarBytes = b
	} else if !errors.Is(err, os.ErrNotExist) {
		return nil, fmt.Errorf("read simpledeploy.yml: %w", err)
	}

	var envExample []byte
	if b, err := os.ReadFile(filepath.Join(appDir, fileEnvSource)); err == nil {
		envExample = redactEnv(b)
	} else if !errors.Is(err, os.ErrNotExist) {
		return nil, fmt.Errorf("read .env: %w", err)
	}

	manifest := Manifest{
		SchemaVersion:             SchemaVersion,
		ExportedAt:                time.Now().UTC(),
		SourceSimpleDeployVersion: version,
		App:                       AppMeta{Slug: slug, DisplayName: displayName},
		Redacted:                  []string{"env_values", "secrets"},
	}
	manifestBytes, err := json.MarshalIndent(manifest, "", "  ")
	if err != nil {
		return nil, fmt.Errorf("marshal manifest: %w", err)
	}

	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)

	write := func(name string, data []byte) error {
		w, err := zw.Create(name)
		if err != nil {
			return err
		}
		_, err = w.Write(data)
		return err
	}

	if err := write(fileCompose, composeBytes); err != nil {
		return nil, err
	}
	if sidecarBytes != nil {
		if err := write(fileSidecar, sidecarBytes); err != nil {
			return nil, err
		}
	}
	if envExample != nil {
		if err := write(fileEnvExample, envExample); err != nil {
			return nil, err
		}
	}
	if err := write(fileManifest, manifestBytes); err != nil {
		return nil, err
	}
	if err := zw.Close(); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

// Parse validates a ZIP and returns its contents.
func Parse(zipBytes []byte) (*Bundle, error) {
	zr, err := zip.NewReader(bytes.NewReader(zipBytes), int64(len(zipBytes)))
	if err != nil {
		return nil, fmt.Errorf("open zip: %w", err)
	}

	files := map[string]*zip.File{}
	for _, f := range zr.File {
		name := f.Name
		// zip-slip / path safety: reject absolute paths and parent traversal.
		if strings.HasPrefix(name, "/") || strings.Contains(name, "..") {
			return nil, fmt.Errorf("unsafe zip entry: %q", name)
		}
		// Reject nested paths; bundle is flat.
		if strings.ContainsAny(name, `\`) {
			return nil, fmt.Errorf("unsafe zip entry: %q", name)
		}
		clean := filepath.ToSlash(filepath.Clean(name))
		if clean != name {
			return nil, fmt.Errorf("unsafe zip entry: %q", name)
		}
		files[name] = f
	}

	manifestFile, ok := files[fileManifest]
	if !ok {
		return nil, errors.New("manifest.json missing")
	}
	manifestBytes, err := readZipFile(manifestFile)
	if err != nil {
		return nil, fmt.Errorf("read manifest: %w", err)
	}
	var manifest Manifest
	if err := json.Unmarshal(manifestBytes, &manifest); err != nil {
		return nil, fmt.Errorf("parse manifest: %w", err)
	}
	if manifest.SchemaVersion != SchemaVersion {
		return nil, fmt.Errorf("unsupported schema_version %d", manifest.SchemaVersion)
	}

	composeFile, ok := files[fileCompose]
	if !ok {
		return nil, errors.New("docker-compose.yml missing")
	}
	composeBytes, err := readZipFile(composeFile)
	if err != nil {
		return nil, fmt.Errorf("read compose: %w", err)
	}

	b := &Bundle{Manifest: manifest, Compose: composeBytes}

	if f, ok := files[fileSidecar]; ok {
		data, err := readZipFile(f)
		if err != nil {
			return nil, fmt.Errorf("read sidecar: %w", err)
		}
		b.Sidecar = data
	}
	if f, ok := files[fileEnvExample]; ok {
		data, err := readZipFile(f)
		if err != nil {
			return nil, fmt.Errorf("read env.example: %w", err)
		}
		b.EnvExample = data
	}
	return b, nil
}

func readZipFile(f *zip.File) ([]byte, error) {
	rc, err := f.Open()
	if err != nil {
		return nil, err
	}
	defer rc.Close()
	return io.ReadAll(rc)
}

// redactEnv strips values from KEY=VALUE lines while preserving comments,
// blank lines, and lines without `=`.
func redactEnv(data []byte) []byte {
	var out bytes.Buffer
	// Preserve trailing-newline behavior: split keeping line structure.
	lines := strings.SplitAfter(string(data), "\n")
	for _, line := range lines {
		if line == "" {
			continue
		}
		// Separate trailing newline (if any) for clean output.
		nl := ""
		body := line
		if strings.HasSuffix(body, "\n") {
			nl = "\n"
			body = strings.TrimSuffix(body, "\n")
		}
		trimmed := strings.TrimLeft(body, " \t")
		if trimmed == "" || strings.HasPrefix(trimmed, "#") {
			out.WriteString(body)
			out.WriteString(nl)
			continue
		}
		idx := strings.Index(body, "=")
		if idx < 0 {
			out.WriteString(body)
			out.WriteString(nl)
			continue
		}
		out.WriteString(body[:idx+1])
		out.WriteString(nl)
	}
	return out.Bytes()
}
