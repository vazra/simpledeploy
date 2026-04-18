package docker

import (
	"context"
	"errors"
	"testing"

	"github.com/docker/docker/api/types/network"
)

func TestEnsureNetworkExists(t *testing.T) {
	m := NewMockClient()
	m.NetworkInspectFn = func(_ context.Context, id string) (network.Inspect, error) {
		return network.Inspect{Name: id}, nil
	}
	if err := EnsureNetwork(context.Background(), m, "simpledeploy-public"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if m.HasCall("NetworkCreate") {
		t.Error("should not call NetworkCreate when network exists")
	}
}

func TestEnsureNetworkCreates(t *testing.T) {
	m := NewMockClient()
	m.NetworkInspectFn = func(_ context.Context, id string) (network.Inspect, error) {
		return network.Inspect{}, errors.New("Error: No such network: " + id)
	}
	var gotOpts network.CreateOptions
	m.NetworkCreateFn = func(_ context.Context, name string, opts network.CreateOptions) (network.CreateResponse, error) {
		gotOpts = opts
		return network.CreateResponse{ID: name}, nil
	}
	if err := EnsureNetwork(context.Background(), m, "simpledeploy-public"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !m.HasCall("NetworkCreate:simpledeploy-public") {
		t.Error("expected NetworkCreate call for simpledeploy-public")
	}
	if gotOpts.Driver != "bridge" {
		t.Errorf("Driver = %q, want %q", gotOpts.Driver, "bridge")
	}
	if !gotOpts.Attachable {
		t.Error("expected Attachable=true")
	}
}

func TestEnsureNetworkInspectErrorPropagates(t *testing.T) {
	m := NewMockClient()
	m.NetworkInspectFn = func(_ context.Context, _ string) (network.Inspect, error) {
		return network.Inspect{}, errors.New("docker daemon unreachable")
	}
	if err := EnsureNetwork(context.Background(), m, "simpledeploy-public"); err == nil {
		t.Fatal("expected error, got nil")
	}
	if m.HasCall("NetworkCreate") {
		t.Error("should not call NetworkCreate on inspect failure")
	}
}

func TestEnsureNetworkCreateErrorPropagates(t *testing.T) {
	m := NewMockClient()
	m.NetworkInspectFn = func(_ context.Context, _ string) (network.Inspect, error) {
		return network.Inspect{}, errors.New("No such network: foo")
	}
	m.NetworkCreateFn = func(_ context.Context, _ string, _ network.CreateOptions) (network.CreateResponse, error) {
		return network.CreateResponse{}, errors.New("create failed")
	}
	if err := EnsureNetwork(context.Background(), m, "simpledeploy-public"); err == nil {
		t.Fatal("expected error, got nil")
	}
}
