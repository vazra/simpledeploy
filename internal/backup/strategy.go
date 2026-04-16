package backup

import (
	"context"
	"io"

	"github.com/vazra/simpledeploy/internal/compose"
)

type Strategy interface {
	Type() string
	Detect(cfg *compose.AppConfig) []DetectedService
	Backup(ctx context.Context, opts BackupOpts) (*BackupResult, error)
	Restore(ctx context.Context, opts RestoreOpts) error
}

type DetectedService struct {
	ServiceName   string            `json:"service_name"`
	ContainerName string            `json:"container_name"`
	Label         string            `json:"label"`
	Paths         []string          `json:"paths,omitempty"`
	Metadata      map[string]string `json:"metadata,omitempty"`
}

type BackupOpts struct {
	ContainerName string
	Paths         []string
	Credentials   map[string]string
}

type BackupResult struct {
	Reader   io.ReadCloser
	Filename string
}

type RestoreOpts struct {
	ContainerName string
	Paths         []string
	Credentials   map[string]string
	Reader        io.ReadCloser
}
