package backup

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"slices"
	"strings"
	"time"

	"github.com/vazra/simpledeploy/internal/compose"
)

const (
	HookTypeStop       = "stop"
	HookTypeStart      = "start"
	HookTypeFlushRedis = "flush_redis"
	HookTypeFlushMySQL = "flush_mysql"
	HookTypeExec       = "exec"
)

// Limits on stored hooks, checked on save and again before a run.
const (
	MaxHooks          = 20
	MaxHookTimeout    = 3600 // seconds
	MaxHookCommandLen = 4096

	defaultHookTimeout = 30 * time.Second
)

var hookTypes = []string{HookTypeStop, HookTypeStart, HookTypeFlushRedis, HookTypeFlushMySQL, HookTypeExec}

type Hook struct {
	Type    string `json:"type"`
	Service string `json:"service"`
	Command string `json:"command,omitempty"`
	Timeout int    `json:"timeout,omitempty"` // seconds, 0 = use default
}

// ContainerExecutor runs hook actions on one compose service of one compose
// project. Implementations must act only on containers labeled with both
// the project and the service, so a hook can never reach another app's
// containers, SimpleDeploy itself, or other containers on the host.
type ContainerExecutor interface {
	ExecInService(ctx context.Context, project, service, command string) (string, error)
	StopService(ctx context.Context, project, service string) error
	StartService(ctx context.Context, project, service string) error
}

type HookRunner struct {
	exec           ContainerExecutor
	project        string
	defaultTimeout time.Duration
}

// NewHookRunner returns a runner whose hooks act only on services of the
// compose project (simpledeploy-<slug>).
func NewHookRunner(exec ContainerExecutor, project string, defaultTimeout time.Duration) *HookRunner {
	return &HookRunner{exec: exec, project: project, defaultTimeout: defaultTimeout}
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
		return hr.exec.StopService(ctx, hr.project, h.Service)
	case HookTypeStart:
		return hr.exec.StartService(ctx, hr.project, h.Service)
	case HookTypeFlushRedis:
		_, err := hr.exec.ExecInService(ctx, hr.project, h.Service, "redis-cli BGSAVE")
		return err
	case HookTypeFlushMySQL:
		cmd := "mysql -u root -e 'FLUSH TABLES WITH READ LOCK; SYSTEM sleep 0; UNLOCK TABLES;'"
		_, err := hr.exec.ExecInService(ctx, hr.project, h.Service, cmd)
		return err
	case HookTypeExec:
		if h.Command == "" {
			return fmt.Errorf("exec hook requires command")
		}
		_, err := hr.exec.ExecInService(ctx, hr.project, h.Service, h.Command)
		return err
	default:
		return fmt.Errorf("unknown hook type: %s", h.Type)
	}
}

// ParseHooks decodes a stored hooks value: a JSON list, with "" or "null"
// meaning none. Entries whose service is an object were saved by an older
// backup wizard and never ran; they are skipped and counted in legacy.
func ParseHooks(raw string) (hooks []Hook, legacy int, err error) {
	raw = strings.TrimSpace(raw)
	if raw == "" || raw == "null" {
		return nil, 0, nil
	}
	var entries []struct {
		Type    string          `json:"type"`
		Service json.RawMessage `json:"service"`
		Command string          `json:"command"`
		Timeout int             `json:"timeout"`
	}
	if err := json.Unmarshal([]byte(raw), &entries); err != nil {
		return nil, 0, fmt.Errorf("hooks must be a JSON list of {type, service, command, timeout} entries: %v", err)
	}
	for i, e := range entries {
		svc := bytes.TrimSpace(e.Service)
		if len(svc) > 0 && svc[0] == '{' {
			legacy++
			continue
		}
		var service string
		if len(svc) > 0 && string(svc) != "null" {
			if err := json.Unmarshal(svc, &service); err != nil {
				return nil, 0, fmt.Errorf("hook %d: service must be a service name", i+1)
			}
		}
		hooks = append(hooks, Hook{
			Type:    strings.TrimSpace(e.Type),
			Service: strings.TrimSpace(service),
			Command: strings.TrimSpace(e.Command),
			Timeout: e.Timeout,
		})
	}
	return hooks, legacy, nil
}

// ValidateHooks checks one hook list; phase (PreHookPhase, PostHookPhase)
// prefixes messages. services, when non-nil, lists the app's compose
// services every hook must name. A pre-hook may not stop any of
// strategyServices (the services the backup and restore commands may run
// inside), since those commands need it running. Errors are human-readable.
func ValidateHooks(phase string, hooks []Hook, services, strategyServices []string) error {
	if len(hooks) > MaxHooks {
		return fmt.Errorf("%s hooks: at most %d are allowed", phase, MaxHooks)
	}
	for i, h := range hooks {
		n := i + 1
		if !slices.Contains(hookTypes, h.Type) {
			return fmt.Errorf("%s hook %d: unknown type %q. Use one of: %s", phase, n, h.Type, strings.Join(hookTypes, ", "))
		}
		if h.Service == "" {
			return fmt.Errorf("%s hook %d: choose the service it runs on", phase, n)
		}
		if services != nil && !slices.Contains(services, h.Service) {
			return fmt.Errorf("%s hook %d: %q is not a service of this app. Services in this app: %s", phase, n, h.Service, strings.Join(services, ", "))
		}
		if h.Timeout < 0 || h.Timeout > MaxHookTimeout {
			return fmt.Errorf("%s hook %d: timeout must be between 0 and %d seconds (0 uses the %d second default)", phase, n, MaxHookTimeout, int(defaultHookTimeout.Seconds()))
		}
		if h.Type == HookTypeExec && h.Command == "" {
			return fmt.Errorf("%s hook %d: enter the command to run", phase, n)
		}
		if len(h.Command) > MaxHookCommandLen {
			return fmt.Errorf("%s hook %d: command is longer than %d characters", phase, n, MaxHookCommandLen)
		}
		if strings.ContainsRune(h.Command, 0) {
			return fmt.Errorf("%s hook %d: command contains an invalid character", phase, n)
		}
		if phase == PreHookPhase && h.Type == HookTypeStop && slices.Contains(strategyServices, h.Service) {
			return fmt.Errorf("%s hook %d: the %q service cannot be stopped, because the backup runs inside it and needs it running. Stop the services that write to its data instead", phase, n, h.Service)
		}
	}
	return nil
}

// Phase names for ValidateHooks.
const (
	PreHookPhase  = "Pre-backup"
	PostHookPhase = "Post-backup"
)

// CheckHooks parses and validates a config's stored pre and post hooks.
// With composeCfg (nil when the compose file can't be read) hooks must
// name one of its services, and with strategy too a pre-hook may not stop
// a service that strategy detects (it runs in one of them). legacy counts
// skipped entries from the old wizard format (see ParseHooks).
func CheckHooks(rawPre, rawPost string, composeCfg *compose.AppConfig, strategy Strategy) (pre, post []Hook, legacy int, err error) {
	pre, lp, err := ParseHooks(rawPre)
	if err != nil {
		return nil, nil, 0, fmt.Errorf("%s hooks: %w", PreHookPhase, err)
	}
	post, lq, err := ParseHooks(rawPost)
	if err != nil {
		return nil, nil, 0, fmt.Errorf("%s hooks: %w", PostHookPhase, err)
	}
	var services, strategyServices []string
	if composeCfg != nil {
		services = make([]string, 0, len(composeCfg.Services))
		for _, svc := range composeCfg.Services {
			services = append(services, svc.Name)
		}
		slices.Sort(services)
		if strategy != nil {
			for _, d := range strategy.Detect(composeCfg) {
				strategyServices = append(strategyServices, d.ServiceName)
			}
		}
	}
	if err := ValidateHooks(PreHookPhase, pre, services, strategyServices); err != nil {
		return nil, nil, 0, err
	}
	if err := ValidateHooks(PostHookPhase, post, services, nil); err != nil {
		return nil, nil, 0, err
	}
	return pre, post, lp + lq, nil
}

// HooksJSON is the canonical stored form of hooks.
func HooksJSON(hooks []Hook) string {
	if hooks == nil {
		hooks = []Hook{}
	}
	b, _ := json.Marshal(hooks)
	return string(b)
}
