package docker

import (
	"context"
	"io"

	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/api/types/network"
	dockerclient "github.com/docker/docker/client"
)

type Client interface {
	Ping(ctx context.Context) error
	Close() error
	ContainerList(ctx context.Context, opts container.ListOptions) ([]container.Summary, error)
	ContainerStats(ctx context.Context, containerID string) (container.StatsResponseReader, error)
	ContainerLogs(ctx context.Context, containerID string, opts container.LogsOptions) (io.ReadCloser, error)
	ContainerInspect(ctx context.Context, id string) (container.InspectResponse, error)
	NetworkInspect(ctx context.Context, id string) (network.Inspect, error)
	NetworkCreate(ctx context.Context, name string, opts network.CreateOptions) (network.CreateResponse, error)
	Raw() *dockerclient.Client
}

type DockerClient struct {
	cli *dockerclient.Client
}

func NewClient() (*DockerClient, error) {
	cli, err := dockerclient.NewClientWithOpts(
		dockerclient.FromEnv,
		dockerclient.WithAPIVersionNegotiation(),
	)
	if err != nil {
		return nil, err
	}
	return &DockerClient{cli: cli}, nil
}

func (c *DockerClient) Ping(ctx context.Context) error {
	_, err := c.cli.Ping(ctx)
	return err
}

func (c *DockerClient) Close() error {
	return c.cli.Close()
}

func (c *DockerClient) Raw() *dockerclient.Client {
	return c.cli
}

func (c *DockerClient) ContainerList(ctx context.Context, opts container.ListOptions) ([]container.Summary, error) {
	return c.cli.ContainerList(ctx, opts)
}

func (c *DockerClient) ContainerStats(ctx context.Context, id string) (container.StatsResponseReader, error) {
	return c.cli.ContainerStats(ctx, id, false)
}

func (c *DockerClient) ContainerLogs(ctx context.Context, containerID string, opts container.LogsOptions) (io.ReadCloser, error) {
	return c.cli.ContainerLogs(ctx, containerID, opts)
}

func (c *DockerClient) ContainerInspect(ctx context.Context, id string) (container.InspectResponse, error) {
	return c.cli.ContainerInspect(ctx, id)
}

func (c *DockerClient) NetworkInspect(ctx context.Context, id string) (network.Inspect, error) {
	return c.cli.NetworkInspect(ctx, id, network.InspectOptions{})
}

func (c *DockerClient) NetworkCreate(ctx context.Context, name string, opts network.CreateOptions) (network.CreateResponse, error) {
	return c.cli.NetworkCreate(ctx, name, opts)
}
