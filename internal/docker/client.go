package docker

import (
	"context"
	"io"

	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/api/types/image"
	"github.com/docker/docker/api/types/network"
	dockerclient "github.com/docker/docker/client"
)

type Client interface {
	Ping(ctx context.Context) error
	Close() error
	NetworkCreate(ctx context.Context, name string, opts network.CreateOptions) (network.CreateResponse, error)
	NetworkRemove(ctx context.Context, name string) error
	ImagePull(ctx context.Context, ref string, opts image.PullOptions) (io.ReadCloser, error)
	ContainerCreate(ctx context.Context, config *container.Config, hostConfig *container.HostConfig, networkConfig *network.NetworkingConfig, name string) (container.CreateResponse, error)
	ContainerStart(ctx context.Context, id string, opts container.StartOptions) error
	ContainerStop(ctx context.Context, id string, opts container.StopOptions) error
	ContainerRemove(ctx context.Context, id string, opts container.RemoveOptions) error
	ContainerList(ctx context.Context, opts container.ListOptions) ([]container.Summary, error)
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

func (c *DockerClient) NetworkCreate(ctx context.Context, name string, opts network.CreateOptions) (network.CreateResponse, error) {
	return c.cli.NetworkCreate(ctx, name, opts)
}

func (c *DockerClient) NetworkRemove(ctx context.Context, name string) error {
	return c.cli.NetworkRemove(ctx, name)
}

func (c *DockerClient) ImagePull(ctx context.Context, ref string, opts image.PullOptions) (io.ReadCloser, error) {
	return c.cli.ImagePull(ctx, ref, opts)
}

func (c *DockerClient) ContainerCreate(ctx context.Context, config *container.Config, hostConfig *container.HostConfig, networkConfig *network.NetworkingConfig, name string) (container.CreateResponse, error) {
	return c.cli.ContainerCreate(ctx, config, hostConfig, networkConfig, nil, name)
}

func (c *DockerClient) ContainerStart(ctx context.Context, id string, opts container.StartOptions) error {
	return c.cli.ContainerStart(ctx, id, opts)
}

func (c *DockerClient) ContainerStop(ctx context.Context, id string, opts container.StopOptions) error {
	return c.cli.ContainerStop(ctx, id, opts)
}

func (c *DockerClient) ContainerRemove(ctx context.Context, id string, opts container.RemoveOptions) error {
	return c.cli.ContainerRemove(ctx, id, opts)
}

func (c *DockerClient) ContainerList(ctx context.Context, opts container.ListOptions) ([]container.Summary, error) {
	return c.cli.ContainerList(ctx, opts)
}
