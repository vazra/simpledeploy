package backup

import (
	"context"
	"fmt"
	"time"
)

const (
	HookTypeStop       = "stop"
	HookTypeStart      = "start"
	HookTypeFlushRedis = "flush_redis"
	HookTypeFlushMySQL = "flush_mysql"
	HookTypeExec       = "exec"
)

type Hook struct {
	Type    string `json:"type"`
	Service string `json:"service"`
	Command string `json:"command,omitempty"`
	Timeout int    `json:"timeout,omitempty"` // seconds, 0 = use default
}

type ContainerExecutor interface {
	ExecInContainer(ctx context.Context, container, command string) (string, error)
	StopContainer(ctx context.Context, container string) error
	StartContainer(ctx context.Context, container string) error
}

type HookRunner struct {
	exec           ContainerExecutor
	defaultTimeout time.Duration
}

func NewHookRunner(exec ContainerExecutor, defaultTimeout time.Duration) *HookRunner {
	return &HookRunner{exec: exec, defaultTimeout: defaultTimeout}
}

// RunPre executes hooks sequentially, aborting on first failure.
func (hr *HookRunner) RunPre(ctx context.Context, hooks []Hook) error {
	for i, h := range hooks {
		if err := hr.execute(ctx, h); err != nil {
			return fmt.Errorf("pre-hook[%d] %s(%s) failed: %w", i, h.Type, h.Service, err)
		}
	}
	return nil
}

// RunPost executes hooks sequentially, collecting warnings on failure.
func (hr *HookRunner) RunPost(ctx context.Context, hooks []Hook) []string {
	var warnings []string
	for i, h := range hooks {
		if err := hr.execute(ctx, h); err != nil {
			warnings = append(warnings, fmt.Sprintf("post-hook[%d] %s(%s): %v", i, h.Type, h.Service, err))
		}
	}
	return warnings
}

func (hr *HookRunner) execute(ctx context.Context, h Hook) error {
	timeout := hr.defaultTimeout
	if h.Timeout > 0 {
		timeout = time.Duration(h.Timeout) * time.Second
	}
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	switch h.Type {
	case HookTypeStop:
		return hr.exec.StopContainer(ctx, h.Service)
	case HookTypeStart:
		return hr.exec.StartContainer(ctx, h.Service)
	case HookTypeFlushRedis:
		_, err := hr.exec.ExecInContainer(ctx, h.Service, "redis-cli BGSAVE")
		return err
	case HookTypeFlushMySQL:
		cmd := "mysql -u root -e 'FLUSH TABLES WITH READ LOCK; SYSTEM sleep 0; UNLOCK TABLES;'"
		_, err := hr.exec.ExecInContainer(ctx, h.Service, cmd)
		return err
	case HookTypeExec:
		if h.Command == "" {
			return fmt.Errorf("exec hook requires command")
		}
		_, err := hr.exec.ExecInContainer(ctx, h.Service, h.Command)
		return err
	default:
		return fmt.Errorf("unknown hook type: %s", h.Type)
	}
}
