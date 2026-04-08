package deployer

import (
	"context"
	"fmt"
	"io"

	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/api/types/filters"
	"github.com/docker/docker/api/types/image"
	"github.com/docker/docker/api/types/network"
	"github.com/docker/go-connections/nat"
	"github.com/vazra/simpledeploy/internal/compose"
	"github.com/vazra/simpledeploy/internal/docker"
)

const projectLabel = "simpledeploy.project"

type Deployer struct {
	docker docker.Client
}

func New(docker docker.Client) *Deployer {
	return &Deployer{docker: docker}
}

func (d *Deployer) Deploy(ctx context.Context, app *compose.AppConfig) error {
	netName := fmt.Sprintf("simpledeploy-%s", app.Name)
	_, err := d.docker.NetworkCreate(ctx, netName, network.CreateOptions{
		Labels: map[string]string{
			projectLabel: app.Name,
		},
	})
	if err != nil {
		return fmt.Errorf("create network: %w", err)
	}

	for _, svc := range app.Services {
		if err := d.deployService(ctx, app.Name, netName, svc); err != nil {
			return fmt.Errorf("deploy service %s: %w", svc.Name, err)
		}
	}
	return nil
}

func (d *Deployer) deployService(ctx context.Context, appName, netName string, svc compose.ServiceConfig) error {
	rc, err := d.docker.ImagePull(ctx, svc.Image, image.PullOptions{})
	if err != nil {
		return fmt.Errorf("pull image %s: %w", svc.Image, err)
	}
	_, _ = io.Copy(io.Discard, rc)
	rc.Close()

	env := make([]string, 0, len(svc.Environment))
	for k, v := range svc.Environment {
		env = append(env, fmt.Sprintf("%s=%s", k, v))
	}

	labels := map[string]string{
		projectLabel:            appName,
		"simpledeploy.service": svc.Name,
	}
	for k, v := range svc.Labels {
		labels[k] = v
	}

	exposedPorts := nat.PortSet{}
	portBindings := nat.PortMap{}
	for _, p := range svc.Ports {
		proto := p.Protocol
		if proto == "" {
			proto = "tcp"
		}
		ctrPort, err := nat.NewPort(proto, p.Container)
		if err != nil {
			return fmt.Errorf("parse container port %s: %w", p.Container, err)
		}
		exposedPorts[ctrPort] = struct{}{}
		portBindings[ctrPort] = []nat.PortBinding{
			{HostPort: p.Host},
		}
	}

	containerConfig := &container.Config{
		Image:        svc.Image,
		Env:          env,
		Labels:       labels,
		ExposedPorts: exposedPorts,
	}

	volBinds := make([]string, 0, len(svc.Volumes))
	for _, v := range svc.Volumes {
		volBinds = append(volBinds, fmt.Sprintf("%s:%s", v.Source, v.Target))
	}

	hostConfig := &container.HostConfig{
		PortBindings: portBindings,
		RestartPolicy: container.RestartPolicy{
			Name: restartPolicyName(svc.Restart),
		},
		Binds: volBinds,
	}

	netConfig := &network.NetworkingConfig{
		EndpointsConfig: map[string]*network.EndpointSettings{
			netName: {},
		},
	}

	ctrName := fmt.Sprintf("simpledeploy-%s-%s", appName, svc.Name)
	resp, err := d.docker.ContainerCreate(ctx, containerConfig, hostConfig, netConfig, ctrName)
	if err != nil {
		return fmt.Errorf("create container %s: %w", ctrName, err)
	}

	if err := d.docker.ContainerStart(ctx, resp.ID, container.StartOptions{}); err != nil {
		return fmt.Errorf("start container %s: %w", ctrName, err)
	}

	return nil
}

func (d *Deployer) Teardown(ctx context.Context, projectName string) error {
	ctrs, err := d.docker.ContainerList(ctx, container.ListOptions{
		All: true,
		Filters: filters.NewArgs(
			filters.Arg("label", fmt.Sprintf("%s=%s", projectLabel, projectName)),
		),
	})
	if err != nil {
		return fmt.Errorf("list containers: %w", err)
	}

	for _, ctr := range ctrs {
		if err := d.docker.ContainerStop(ctx, ctr.ID, container.StopOptions{}); err != nil {
			return fmt.Errorf("stop container %s: %w", ctr.ID, err)
		}
		if err := d.docker.ContainerRemove(ctx, ctr.ID, container.RemoveOptions{}); err != nil {
			return fmt.Errorf("remove container %s: %w", ctr.ID, err)
		}
	}

	netName := fmt.Sprintf("simpledeploy-%s", projectName)
	if err := d.docker.NetworkRemove(ctx, netName); err != nil {
		return fmt.Errorf("remove network %s: %w", netName, err)
	}

	return nil
}

func restartPolicyName(restart string) container.RestartPolicyMode {
	switch restart {
	case "always":
		return container.RestartPolicyAlways
	case "unless-stopped":
		return container.RestartPolicyUnlessStopped
	case "on-failure":
		return container.RestartPolicyOnFailure
	default:
		return container.RestartPolicyDisabled
	}
}
