package proxy

import (
	"context"
	"errors"
	"testing"

	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/api/types/network"
)

type fakeInspector struct {
	listResult    []container.Summary
	listErr       error
	inspectResult container.InspectResponse
	inspectErr    error
	listCalls     int
	inspectCalls  int
}

func (f *fakeInspector) ContainerList(_ context.Context, _ container.ListOptions) ([]container.Summary, error) {
	f.listCalls++
	return f.listResult, f.listErr
}

func (f *fakeInspector) ContainerInspect(_ context.Context, _ string) (container.InspectResponse, error) {
	f.inspectCalls++
	return f.inspectResult, f.inspectErr
}

func inspectOn(netName, ip string) container.InspectResponse {
	return container.InspectResponse{
		ContainerJSONBase: &container.ContainerJSONBase{ID: "c1"},
		NetworkSettings: &container.NetworkSettings{
			Networks: map[string]*network.EndpointSettings{
				netName: {IPAddress: ip},
			},
		},
	}
}

func TestDockerResolver_ContainerIP_FoundAndAttached(t *testing.T) {
	f := &fakeInspector{
		listResult:    []container.Summary{{ID: "c1"}},
		inspectResult: inspectOn("simpledeploy-public", "10.0.0.5"),
	}
	r := &DockerResolver{Client: f}
	ip, err := r.ContainerIP("simpledeploy-app", "web", "simpledeploy-public")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if ip != "10.0.0.5" {
		t.Errorf("ip = %q, want 10.0.0.5", ip)
	}
	if f.listCalls != 1 || f.inspectCalls != 1 {
		t.Errorf("expected 1 list + 1 inspect call, got %d + %d", f.listCalls, f.inspectCalls)
	}
}

func TestDockerResolver_ContainerIP_NoMatchingContainer(t *testing.T) {
	f := &fakeInspector{listResult: nil}
	r := &DockerResolver{Client: f}
	ip, err := r.ContainerIP("p", "s", "simpledeploy-public")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if ip != "" {
		t.Errorf("ip = %q, want empty", ip)
	}
	if f.inspectCalls != 0 {
		t.Errorf("inspect should not be called when list is empty, got %d", f.inspectCalls)
	}
}

func TestDockerResolver_ContainerIP_NotYetOnNetwork(t *testing.T) {
	f := &fakeInspector{
		listResult:    []container.Summary{{ID: "c1"}},
		inspectResult: inspectOn("other-net", "10.0.0.5"),
	}
	r := &DockerResolver{Client: f}
	ip, err := r.ContainerIP("p", "s", "simpledeploy-public")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if ip != "" {
		t.Errorf("ip = %q, want empty (not attached yet)", ip)
	}
}

func TestDockerResolver_ContainerIP_ListError(t *testing.T) {
	want := errors.New("docker down")
	f := &fakeInspector{listErr: want}
	r := &DockerResolver{Client: f}
	ip, err := r.ContainerIP("p", "s", "simpledeploy-public")
	if err == nil || !errors.Is(err, want) {
		t.Errorf("err = %v, want %v", err, want)
	}
	if ip != "" {
		t.Errorf("ip = %q, want empty on error", ip)
	}
}

func TestDockerResolver_ContainerIP_InspectError(t *testing.T) {
	want := errors.New("boom")
	f := &fakeInspector{
		listResult: []container.Summary{{ID: "c1"}},
		inspectErr: want,
	}
	r := &DockerResolver{Client: f}
	ip, err := r.ContainerIP("p", "s", "simpledeploy-public")
	if err == nil || !errors.Is(err, want) {
		t.Errorf("err = %v, want %v", err, want)
	}
	if ip != "" {
		t.Errorf("ip = %q, want empty on error", ip)
	}
}

func TestDockerResolver_ContainerIP_NilClient(t *testing.T) {
	r := &DockerResolver{Client: nil}
	ip, err := r.ContainerIP("p", "s", "simpledeploy-public")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if ip != "" {
		t.Errorf("ip = %q, want empty", ip)
	}
}

func TestDockerResolver_ContainerIP_NilReceiver(t *testing.T) {
	var r *DockerResolver
	ip, err := r.ContainerIP("p", "s", "simpledeploy-public")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if ip != "" {
		t.Errorf("ip = %q, want empty", ip)
	}
}
