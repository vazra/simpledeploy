package backup

import (
	"context"
	"fmt"
	"strings"
	"testing"
	"time"
)

type mockExecutor struct {
	calls  []mockCall
	failOn map[string]error // "type:service" -> error
}

type mockCall struct {
	method    string
	container string
	command   string
}

func newMockExecutor() *mockExecutor {
	return &mockExecutor{failOn: make(map[string]error)}
}

func (m *mockExecutor) ExecInContainer(_ context.Context, container, command string) (string, error) {
	m.calls = append(m.calls, mockCall{"exec", container, command})
	if err, ok := m.failOn["exec:"+container]; ok {
		return "", err
	}
	return "ok", nil
}

func (m *mockExecutor) StopContainer(_ context.Context, container string) error {
	m.calls = append(m.calls, mockCall{"stop", container, ""})
	if err, ok := m.failOn["stop:"+container]; ok {
		return err
	}
	return nil
}

func (m *mockExecutor) StartContainer(_ context.Context, container string) error {
	m.calls = append(m.calls, mockCall{"start", container, ""})
	if err, ok := m.failOn["start:"+container]; ok {
		return err
	}
	return nil
}

func TestHookRunnerPreSequential(t *testing.T) {
	m := newMockExecutor()
	hr := NewHookRunner(m, 30*time.Second)

	hooks := []Hook{
		{Type: HookTypeStop, Service: "db"},
		{Type: HookTypeExec, Service: "app", Command: "echo prepare"},
	}

	err := hr.RunPre(context.Background(), hooks)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(m.calls) != 2 {
		t.Fatalf("expected 2 calls, got %d", len(m.calls))
	}
	if m.calls[0].method != "stop" || m.calls[0].container != "db" {
		t.Fatalf("first call: got %+v", m.calls[0])
	}
	if m.calls[1].method != "exec" || m.calls[1].container != "app" {
		t.Fatalf("second call: got %+v", m.calls[1])
	}
}

func TestHookRunnerPreAbortsOnFailure(t *testing.T) {
	m := newMockExecutor()
	m.failOn["stop:db"] = fmt.Errorf("container not found")
	hr := NewHookRunner(m, 30*time.Second)

	hooks := []Hook{
		{Type: HookTypeStop, Service: "db"},
		{Type: HookTypeStart, Service: "db"},
	}

	err := hr.RunPre(context.Background(), hooks)
	if err == nil {
		t.Fatal("expected error")
	}
	if len(m.calls) != 1 {
		t.Fatalf("expected 1 call (abort after failure), got %d", len(m.calls))
	}
}

func TestHookRunnerPostContinuesOnFailure(t *testing.T) {
	m := newMockExecutor()
	m.failOn["start:db"] = fmt.Errorf("start failed")
	hr := NewHookRunner(m, 30*time.Second)

	hooks := []Hook{
		{Type: HookTypeStart, Service: "db"},
		{Type: HookTypeStart, Service: "app"},
	}

	warnings := hr.RunPost(context.Background(), hooks)
	if len(warnings) != 1 {
		t.Fatalf("expected 1 warning, got %d: %v", len(warnings), warnings)
	}
	if !strings.Contains(warnings[0], "start failed") {
		t.Fatalf("warning should contain error: %s", warnings[0])
	}
	if len(m.calls) != 2 {
		t.Fatalf("expected 2 calls (continue on failure), got %d", len(m.calls))
	}
}

func TestHookRunnerDispatchesCorrectCommands(t *testing.T) {
	tests := []struct {
		hook       Hook
		wantMethod string
		wantCmd    string
	}{
		{Hook{Type: HookTypeStop, Service: "s"}, "stop", ""},
		{Hook{Type: HookTypeStart, Service: "s"}, "start", ""},
		{Hook{Type: HookTypeFlushRedis, Service: "redis"}, "exec", "redis-cli BGSAVE"},
		{Hook{Type: HookTypeFlushMySQL, Service: "mysql"}, "exec", "mysql -u root -e 'FLUSH TABLES WITH READ LOCK; SYSTEM sleep 0; UNLOCK TABLES;'"},
		{Hook{Type: HookTypeExec, Service: "s", Command: "pg_dump"}, "exec", "pg_dump"},
	}

	for _, tt := range tests {
		t.Run(tt.hook.Type, func(t *testing.T) {
			m := newMockExecutor()
			hr := NewHookRunner(m, 30*time.Second)
			err := hr.RunPre(context.Background(), []Hook{tt.hook})
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if len(m.calls) != 1 {
				t.Fatalf("expected 1 call, got %d", len(m.calls))
			}
			if m.calls[0].method != tt.wantMethod {
				t.Fatalf("method: got %s, want %s", m.calls[0].method, tt.wantMethod)
			}
			if tt.wantCmd != "" && m.calls[0].command != tt.wantCmd {
				t.Fatalf("command: got %q, want %q", m.calls[0].command, tt.wantCmd)
			}
		})
	}
}

func TestHookRunnerExecRequiresCommand(t *testing.T) {
	m := newMockExecutor()
	hr := NewHookRunner(m, 30*time.Second)

	err := hr.RunPre(context.Background(), []Hook{{Type: HookTypeExec, Service: "s"}})
	if err == nil {
		t.Fatal("expected error for exec without command")
	}
}

func TestHookRunnerUnknownType(t *testing.T) {
	m := newMockExecutor()
	hr := NewHookRunner(m, 30*time.Second)

	err := hr.RunPre(context.Background(), []Hook{{Type: "bogus", Service: "s"}})
	if err == nil {
		t.Fatal("expected error for unknown hook type")
	}
}
