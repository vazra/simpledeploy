package deployer

import (
	"context"
	"testing"

	"github.com/vazra/simpledeploy/internal/compose"
	"github.com/vazra/simpledeploy/internal/docker"
)

func singleServiceApp() *compose.AppConfig {
	return &compose.AppConfig{
		Name: "myapp",
		Services: []compose.ServiceConfig{
			{
				Name:  "web",
				Image: "nginx:latest",
				Ports: []compose.PortMapping{
					{Host: "8080", Container: "80", Protocol: "tcp"},
				},
				Environment: map[string]string{"ENV": "prod"},
				Volumes:     []compose.VolumeMount{{Source: "/data", Target: "/app/data"}},
				Restart:     "always",
			},
		},
	}
}

func TestDeployCreatesNetworkAndContainers(t *testing.T) {
	mock := docker.NewMockClient()
	d := New(mock)
	ctx := context.Background()

	app := singleServiceApp()
	if err := d.Deploy(ctx, app); err != nil {
		t.Fatalf("Deploy: %v", err)
	}

	if !mock.HasCall("NetworkCreate:simpledeploy-myapp") {
		t.Error("expected NetworkCreate:simpledeploy-myapp")
	}
	if !mock.HasCall("ImagePull:nginx:latest") {
		t.Error("expected ImagePull:nginx:latest")
	}
	if !mock.HasCall("ContainerCreate:simpledeploy-myapp-web") {
		t.Error("expected ContainerCreate:simpledeploy-myapp-web")
	}
	// ContainerStart is called with the ID returned from ContainerCreate
	ctrID := "simpledeploy-myapp-web-id"
	if !mock.HasCall("ContainerStart:" + ctrID) {
		t.Errorf("expected ContainerStart:%s", ctrID)
	}
}

func TestDeployMultipleServices(t *testing.T) {
	mock := docker.NewMockClient()
	d := New(mock)
	ctx := context.Background()

	app := &compose.AppConfig{
		Name: "multi",
		Services: []compose.ServiceConfig{
			{Name: "api", Image: "api:v1"},
			{Name: "db", Image: "postgres:15"},
		},
	}

	if err := d.Deploy(ctx, app); err != nil {
		t.Fatalf("Deploy: %v", err)
	}

	if !mock.HasCall("ContainerCreate:simpledeploy-multi-api") {
		t.Error("expected ContainerCreate:simpledeploy-multi-api")
	}
	if !mock.HasCall("ContainerCreate:simpledeploy-multi-db") {
		t.Error("expected ContainerCreate:simpledeploy-multi-db")
	}
}

func TestTeardownRemovesContainersAndNetwork(t *testing.T) {
	mock := docker.NewMockClient()
	d := New(mock)
	ctx := context.Background()

	// Deploy first so containers exist in the mock
	app := singleServiceApp()
	if err := d.Deploy(ctx, app); err != nil {
		t.Fatalf("Deploy: %v", err)
	}

	if err := d.Teardown(ctx, "myapp"); err != nil {
		t.Fatalf("Teardown: %v", err)
	}

	ctrID := "simpledeploy-myapp-web-id"
	if !mock.HasCall("ContainerStop:" + ctrID) {
		t.Errorf("expected ContainerStop:%s", ctrID)
	}
	if !mock.HasCall("ContainerRemove:" + ctrID) {
		t.Errorf("expected ContainerRemove:%s", ctrID)
	}
	if !mock.HasCall("NetworkRemove:simpledeploy-myapp") {
		t.Error("expected NetworkRemove:simpledeploy-myapp")
	}
}
