package backup

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/docker/docker/api/types/container"
)

// fakeLister returns every container regardless of filters, like a daemon
// that ignored them, so the executor's own label checks are exercised.
type fakeLister struct {
	containers []container.Summary
	opts       []container.ListOptions
}

func (f *fakeLister) ContainerList(_ context.Context, opts container.ListOptions) ([]container.Summary, error) {
	f.opts = append(f.opts, opts)
	return f.containers, nil
}

func ctr(id, project, service, state string, extra ...string) container.Summary {
	labels := map[string]string{}
	if project != "" {
		labels[composeProjectLabel] = project
	}
	if service != "" {
		labels[composeServiceLabel] = service
	}
	for i := 0; i+1 < len(extra); i += 2 {
		labels[extra[i]] = extra[i+1]
	}
	return container.Summary{ID: id, Names: []string{"/" + id}, Labels: labels, State: state}
}

func newTestExecutor(containers ...container.Summary) (*DockerHookExecutor, *fakeLister, *[][]string) {
	lister := &fakeLister{containers: containers}
	var calls [][]string
	e := NewDockerHookExecutor(lister)
	e.docker = func(_ context.Context, args ...string) ([]byte, error) {
		calls = append(calls, args)
		return []byte("done"), nil
	}
	return e, lister, &calls
}

func hostContainers() []container.Summary {
	return []container.Summary{
		ctr("own-db-1", testProject, "db", "running", composeNumberLabel, "1"),
		ctr("own-web-2", testProject, "web", "running", composeNumberLabel, "2"),
		ctr("own-web-1", testProject, "web", "exited", composeNumberLabel, "1"),
		ctr("own-web-run", testProject, "web", "running", composeOneoffLabel, "True"),
		ctr("other-db-1", "simpledeploy-other", "db", "running"),
		ctr("other-cache-1", "simpledeploy-other", "cache", "running"),
		ctr("simpledeploy", "", "", "running"),
		ctr("host-postgres", "", "", "running"),
	}
}

func TestDockerHookExecutorExecUsesOwnServiceContainer(t *testing.T) {
	e, lister, calls := newTestExecutor(hostContainers()...)
	out, err := e.ExecInService(context.Background(), testProject, "db", "pg_isready && echo ok")
	if err != nil {
		t.Fatalf("ExecInService: %v", err)
	}
	if out != "done" {
		t.Errorf("out = %q", out)
	}
	want := []string{"exec", "own-db-1", "sh", "-c", "pg_isready && echo ok"}
	if len(*calls) != 1 || strings.Join((*calls)[0], "|") != strings.Join(want, "|") {
		t.Fatalf("docker calls = %q, want [%q]", *calls, want)
	}
	f := lister.opts[0].Filters
	if !lister.opts[0].All || !f.ExactMatch("label", composeProjectLabel+"="+testProject) || !f.ExactMatch("label", composeServiceLabel+"=db") {
		t.Errorf("list options = %+v, want All with project and service label filters", lister.opts[0])
	}
}

func TestDockerHookExecutorRefusesOutsideProject(t *testing.T) {
	// Each name exists on the host, but not as a service of this app.
	for _, service := range []string{
		"cache",              // service of another app
		"other-db-1",         // another app's container name
		"simpledeploy",       // SimpleDeploy's own container
		"host-postgres",      // unrelated host container
		"own-db-1",           // own container name, not a service name
		"simpledeploy-other", // another project name
	} {
		t.Run(service, func(t *testing.T) {
			e, _, calls := newTestExecutor(hostContainers()...)
			ctx := context.Background()
			if _, err := e.ExecInService(ctx, testProject, service, "id"); err == nil || !strings.Contains(err.Error(), "no container found") {
				t.Errorf("exec err = %v", err)
			}
			if err := e.StopService(ctx, testProject, service); err == nil {
				t.Error("stop: expected error")
			}
			if err := e.StartService(ctx, testProject, service); err == nil {
				t.Error("start: expected error")
			}
			if len(*calls) != 0 {
				t.Fatalf("docker was called: %q", *calls)
			}
		})
	}
}

func TestDockerHookExecutorSameServiceOtherProject(t *testing.T) {
	// "db" exists in both apps; only this app's container may be touched.
	e, _, calls := newTestExecutor(hostContainers()...)
	if err := e.StopService(context.Background(), testProject, "db"); err != nil {
		t.Fatalf("StopService: %v", err)
	}
	if len(*calls) != 1 || (*calls)[0][1] != "own-db-1" {
		t.Fatalf("docker calls = %q, want only own-db-1", *calls)
	}
	// And another app's project cannot be named through this app's runner.
	hr := NewHookRunner(e, testProject, defaultHookTimeout)
	*calls = nil
	if err := hr.RunPre(context.Background(), []Hook{{Type: HookTypeStop, Service: "cache"}}); err == nil {
		t.Fatal("RunPre: expected error for another app's service")
	}
	if len(*calls) != 0 {
		t.Fatalf("docker was called: %q", *calls)
	}
}

func TestDockerHookExecutorEmptyNames(t *testing.T) {
	e, _, calls := newTestExecutor(hostContainers()...)
	if err := e.StopService(context.Background(), "", "db"); err == nil {
		t.Error("empty project: expected error")
	}
	if err := e.StopService(context.Background(), testProject, ""); err == nil {
		t.Error("empty service: expected error")
	}
	if len(*calls) != 0 {
		t.Fatalf("docker was called: %q", *calls)
	}
}

func TestDockerHookExecutorStopStartAllReplicas(t *testing.T) {
	e, _, calls := newTestExecutor(hostContainers()...)
	ctx := context.Background()
	if err := e.StopService(ctx, testProject, "web"); err != nil {
		t.Fatalf("StopService: %v", err)
	}
	if err := e.StartService(ctx, testProject, "web"); err != nil {
		t.Fatalf("StartService: %v", err)
	}
	// Replica order, one-off (compose run) container left out.
	want := "stop own-web-1|stop own-web-2|start own-web-1|start own-web-2"
	var got []string
	for _, c := range *calls {
		got = append(got, strings.Join(c, " "))
	}
	if strings.Join(got, "|") != want {
		t.Fatalf("docker calls = %q, want %q", got, want)
	}
}

func TestDockerHookExecutorExecNeedsRunningContainer(t *testing.T) {
	e, _, calls := newTestExecutor(hostContainers()...)
	// web-1 is exited, web-2 runs: exec goes to web-2.
	if _, err := e.ExecInService(context.Background(), testProject, "web", "true"); err != nil {
		t.Fatalf("ExecInService: %v", err)
	}
	if (*calls)[0][1] != "own-web-2" {
		t.Fatalf("exec went to %q, want own-web-2", (*calls)[0][1])
	}

	e, _, calls = newTestExecutor(ctr("own-db-1", testProject, "db", "exited"))
	if _, err := e.ExecInService(context.Background(), testProject, "db", "true"); err == nil || !strings.Contains(err.Error(), "not running") {
		t.Fatalf("err = %v, want not running", err)
	}
	if len(*calls) != 0 {
		t.Fatalf("docker was called: %q", *calls)
	}
}

func TestDockerHookExecutorReportsCommandFailure(t *testing.T) {
	e, _, _ := newTestExecutor(hostContainers()...)
	e.docker = func(context.Context, ...string) ([]byte, error) {
		return []byte("permission denied"), errors.New("exit status 1")
	}
	_, err := e.ExecInService(context.Background(), testProject, "db", "false")
	if err == nil || !strings.Contains(err.Error(), "permission denied") {
		t.Fatalf("err = %v", err)
	}
	if err := e.StopService(context.Background(), testProject, "db"); err == nil || !strings.Contains(err.Error(), "stop db") {
		t.Fatalf("stop err = %v", err)
	}
}
