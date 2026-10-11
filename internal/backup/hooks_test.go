package backup

import (
	"context"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/vazra/simpledeploy/internal/compose"
)

type mockExecutor struct {
	calls  []mockCall
	failOn map[string]error // "method:service" -> error
	events *[]string        // optional shared log, to check ordering
}

type mockCall struct {
	method  string
	project string
	service string
	command string
}

func newMockExecutor() *mockExecutor {
	return &mockExecutor{failOn: make(map[string]error)}
}

func (m *mockExecutor) record(method, project, service, command string) error {
	m.calls = append(m.calls, mockCall{method, project, service, command})
	if m.events != nil {
		*m.events = append(*m.events, method+":"+service)
	}
	return m.failOn[method+":"+service]
}

func (m *mockExecutor) ExecInService(_ context.Context, project, service, command string) (string, error) {
	if err := m.record("exec", project, service, command); err != nil {
		return "", err
	}
	return "ok", nil
}

func (m *mockExecutor) StopService(_ context.Context, project, service string) error {
	return m.record("stop", project, service, "")
}

func (m *mockExecutor) StartService(_ context.Context, project, service string) error {
	return m.record("start", project, service, "")
}

const testProject = "simpledeploy-myapp"

func TestHookRunnerPreSequential(t *testing.T) {
	m := newMockExecutor()
	hr := NewHookRunner(m, testProject, 30*time.Second)

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
	if m.calls[0].method != "stop" || m.calls[0].service != "db" {
		t.Fatalf("first call: got %+v", m.calls[0])
	}
	if m.calls[1].method != "exec" || m.calls[1].service != "app" {
		t.Fatalf("second call: got %+v", m.calls[1])
	}
}

func TestHookRunnerPassesOwnProject(t *testing.T) {
	m := newMockExecutor()
	hr := NewHookRunner(m, testProject, 30*time.Second)
	hooks := []Hook{
		{Type: HookTypeStop, Service: "db"},
		{Type: HookTypeStart, Service: "db"},
		{Type: HookTypeFlushRedis, Service: "cache"},
		{Type: HookTypeExec, Service: "web", Command: "true"},
	}
	if err := hr.RunPre(context.Background(), hooks); err != nil {
		t.Fatalf("RunPre: %v", err)
	}
	for _, c := range m.calls {
		if c.project != testProject {
			t.Fatalf("call %+v used project %q, want %q", c, c.project, testProject)
		}
	}
}

func TestHookRunnerPreAbortsOnFailure(t *testing.T) {
	m := newMockExecutor()
	m.failOn["stop:db"] = fmt.Errorf("container not found")
	hr := NewHookRunner(m, testProject, 30*time.Second)

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
	hr := NewHookRunner(m, testProject, 30*time.Second)

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
			hr := NewHookRunner(m, testProject, 30*time.Second)
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
	hr := NewHookRunner(m, testProject, 30*time.Second)

	err := hr.RunPre(context.Background(), []Hook{{Type: HookTypeExec, Service: "s"}})
	if err == nil {
		t.Fatal("expected error for exec without command")
	}
}

func TestHookRunnerUnknownType(t *testing.T) {
	m := newMockExecutor()
	hr := NewHookRunner(m, testProject, 30*time.Second)

	err := hr.RunPre(context.Background(), []Hook{{Type: "bogus", Service: "s"}})
	if err == nil {
		t.Fatal("expected error for unknown hook type")
	}
}

func TestParseHooks(t *testing.T) {
	hooks, legacy, err := ParseHooks(`[{"type":"stop","service":" db "},{"type":"exec","service":"web","command":"echo hi","timeout":5}]`)
	if err != nil {
		t.Fatalf("ParseHooks: %v", err)
	}
	if legacy != 0 || len(hooks) != 2 {
		t.Fatalf("hooks=%+v legacy=%d", hooks, legacy)
	}
	if hooks[0] != (Hook{Type: "stop", Service: "db"}) {
		t.Errorf("hooks[0] = %+v", hooks[0])
	}
	if hooks[1] != (Hook{Type: "exec", Service: "web", Command: "echo hi", Timeout: 5}) {
		t.Errorf("hooks[1] = %+v", hooks[1])
	}

	for _, empty := range []string{"", "  ", "null", "[]"} {
		if h, l, err := ParseHooks(empty); err != nil || len(h) != 0 || l != 0 {
			t.Errorf("ParseHooks(%q) = %v, %d, %v", empty, h, l, err)
		}
	}

	if _, _, err := ParseHooks(`{"type":"stop"}`); err == nil {
		t.Error("non-list value should fail")
	}
	if _, _, err := ParseHooks(`[{"type":"stop","service":["db"]}]`); err == nil {
		t.Error("non-string service should fail")
	}
}

// The old wizard saved the detected-service object as the service; those
// hooks never ran and are skipped rather than failing every backup.
func TestParseHooksSkipsLegacyWizardEntries(t *testing.T) {
	raw := `[{"type":"stop_container","service":{"service_name":"db","container_name":"simpledeploy-myapp-db-1"}},` +
		`{"type":"exec","service":"web","command":"true"}]`
	hooks, legacy, err := ParseHooks(raw)
	if err != nil {
		t.Fatalf("ParseHooks: %v", err)
	}
	if legacy != 1 || len(hooks) != 1 || hooks[0].Service != "web" {
		t.Fatalf("hooks=%+v legacy=%d", hooks, legacy)
	}
}

func TestValidateHooks(t *testing.T) {
	services := []string{"cache", "db", "web"}
	long := strings.Repeat("x", MaxHookCommandLen+1)
	tooMany := make([]Hook, MaxHooks+1)
	for i := range tooMany {
		tooMany[i] = Hook{Type: HookTypeStart, Service: "web"}
	}

	cases := []struct {
		name    string
		phase   string
		hooks   []Hook
		wantErr string
	}{
		{"valid", PreHookPhase, []Hook{{Type: HookTypeStop, Service: "web"}, {Type: HookTypeExec, Service: "cache", Command: "redis-cli SAVE", Timeout: 60}}, ""},
		{"unknown type", PreHookPhase, []Hook{{Type: "stop_container", Service: "web"}}, "unknown type"},
		{"empty service", PreHookPhase, []Hook{{Type: HookTypeStop}}, "choose the service"},
		{"other app service", PreHookPhase, []Hook{{Type: HookTypeStop, Service: "postgres"}}, "not a service of this app"},
		{"raw container name", PreHookPhase, []Hook{{Type: HookTypeExec, Service: "simpledeploy-other-db-1", Command: "id"}}, "not a service of this app"},
		{"simpledeploy itself", PostHookPhase, []Hook{{Type: HookTypeStop, Service: "simpledeploy"}}, "not a service of this app"},
		{"exec without command", PreHookPhase, []Hook{{Type: HookTypeExec, Service: "web"}}, "enter the command"},
		{"command too long", PreHookPhase, []Hook{{Type: HookTypeExec, Service: "web", Command: long}}, "longer than"},
		{"nul in command", PreHookPhase, []Hook{{Type: HookTypeExec, Service: "web", Command: "a\x00b"}}, "invalid character"},
		{"negative timeout", PreHookPhase, []Hook{{Type: HookTypeStop, Service: "web", Timeout: -1}}, "timeout"},
		{"huge timeout", PreHookPhase, []Hook{{Type: HookTypeStop, Service: "web", Timeout: MaxHookTimeout + 1}}, "timeout"},
		{"too many", PostHookPhase, tooMany, "at most"},
		{"stop strategy service", PreHookPhase, []Hook{{Type: HookTypeStop, Service: "db"}}, "cannot be stopped"},
		{"start strategy service after", PostHookPhase, []Hook{{Type: HookTypeStart, Service: "db"}}, ""},
		{"exec in strategy service", PreHookPhase, []Hook{{Type: HookTypeExec, Service: "db", Command: "sync"}}, ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := ValidateHooks(tc.phase, tc.hooks, services, []string{"db"})
			if tc.wantErr == "" {
				if err != nil {
					t.Fatalf("unexpected error: %v", err)
				}
				return
			}
			if err == nil || !strings.Contains(err.Error(), tc.wantErr) {
				t.Fatalf("err = %v, want containing %q", err, tc.wantErr)
			}
			if !strings.HasPrefix(err.Error(), tc.phase) {
				t.Errorf("err %q should name the phase %q", err, tc.phase)
			}
		})
	}

	// Without a services list (compose unreadable) names are not checked;
	// the executor's project scoping still applies.
	if err := ValidateHooks(PreHookPhase, []Hook{{Type: HookTypeStop, Service: "anything"}}, nil, nil); err != nil {
		t.Errorf("nil services: %v", err)
	}
}

func TestCheckHooksUsesComposeServices(t *testing.T) {
	cfg := &compose.AppConfig{Name: testProject, Services: []compose.ServiceConfig{
		{Name: "web", Image: "nginx"},
		{Name: "db", Image: "postgres:16"},
	}}
	pg := NewPostgresStrategy()

	pre, post, legacy, err := CheckHooks(`[{"type":"stop","service":"web"}]`, `[{"type":"start","service":"web"}]`, cfg, pg)
	if err != nil || len(pre) != 1 || len(post) != 1 || legacy != 0 {
		t.Fatalf("CheckHooks = %v %v %d %v", pre, post, legacy, err)
	}
	if _, _, _, err := CheckHooks(`[{"type":"stop","service":"redis"}]`, "", cfg, pg); err == nil || !strings.Contains(err.Error(), "Services in this app: db, web") {
		t.Fatalf("unknown service err = %v", err)
	}
	// pg_dump runs inside db, so it cannot be stopped first.
	if _, _, _, err := CheckHooks(`[{"type":"stop","service":"db"}]`, "", cfg, pg); err == nil || !strings.Contains(err.Error(), "cannot be stopped") {
		t.Fatalf("stop strategy service err = %v", err)
	}
	if _, _, _, err := CheckHooks("", `not json`, cfg, pg); err == nil || !strings.HasPrefix(err.Error(), PostHookPhase) {
		t.Fatalf("bad JSON err = %v", err)
	}
}

func TestHooksJSON(t *testing.T) {
	if got := HooksJSON(nil); got != "[]" {
		t.Errorf("HooksJSON(nil) = %q", got)
	}
	got := HooksJSON([]Hook{{Type: "stop", Service: "web"}, {Type: "exec", Service: "db", Command: "sync", Timeout: 9}})
	want := `[{"type":"stop","service":"web"},{"type":"exec","service":"db","command":"sync","timeout":9}]`
	if got != want {
		t.Errorf("HooksJSON = %s, want %s", got, want)
	}
}
