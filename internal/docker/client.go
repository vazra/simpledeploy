package docker

import (
	"context"
	"io"
	"strings"

	"github.com/moby/moby/api/types/container"
	"github.com/moby/moby/api/types/network"
	dockerclient "github.com/moby/moby/client"
)

type Client interface {
	Ping(ctx context.Context) error
	Close() error
	ContainerList(ctx context.Context, opts dockerclient.ContainerListOptions) ([]container.Summary, error)
	ContainerStats(ctx context.Context, containerID string) (io.ReadCloser, error)
	ContainerLogs(ctx context.Context, containerID string, opts dockerclient.ContainerLogsOptions) (io.ReadCloser, error)
	ContainerInspect(ctx context.Context, id string) (container.InspectResponse, error)
	NetworkInspect(ctx context.Context, id string) (network.Inspect, error)
	NetworkCreate(ctx context.Context, name string, opts dockerclient.NetworkCreateOptions) (network.CreateResponse, error)
	Raw() *dockerclient.Client
}

type DockerClient struct {
	cli *dockerclient.Client
}

func NewClient() (*DockerClient, error) {
	// API version negotiation is the default in moby/moby/client.
	cli, err := dockerclient.New(dockerclient.FromEnv)
	if err != nil {
		return nil, err
	}
	return &DockerClient{cli: cli}, nil
}

func (c *DockerClient) Ping(ctx context.Context) error {
	_, err := c.cli.Ping(ctx, dockerclient.PingOptions{})
	return err
}

func (c *DockerClient) Close() error {
	return c.cli.Close()
}

func (c *DockerClient) Raw() *dockerclient.Client {
	return c.cli
}

func (c *DockerClient) ContainerList(ctx context.Context, opts dockerclient.ContainerListOptions) ([]container.Summary, error) {
	res, err := c.cli.ContainerList(ctx, opts)
	return res.Items, err
}

func (c *DockerClient) ContainerStats(ctx context.Context, id string) (io.ReadCloser, error) {
	// Single sample incl. previous sample (PreCPUStats), matching the old
	// stream=false behavior so CPU deltas can be computed.
	res, err := c.cli.ContainerStats(ctx, id, dockerclient.ContainerStatsOptions{IncludePreviousSample: true})
	if err != nil {
		return nil, err
	}
	return res.Body, nil
}

func (c *DockerClient) ContainerLogs(ctx context.Context, containerID string, opts dockerclient.ContainerLogsOptions) (io.ReadCloser, error) {
	res, err := c.cli.ContainerLogs(ctx, containerID, opts)
	if err != nil {
		return nil, err
	}
	return res, nil
}

func (c *DockerClient) ContainerInspect(ctx context.Context, id string) (container.InspectResponse, error) {
	res, err := c.cli.ContainerInspect(ctx, id, dockerclient.ContainerInspectOptions{})
	return res.Container, err
}

func (c *DockerClient) NetworkInspect(ctx context.Context, id string) (network.Inspect, error) {
	res, err := c.cli.NetworkInspect(ctx, id, dockerclient.NetworkInspectOptions{})
	return res.Network, err
}

func (c *DockerClient) NetworkCreate(ctx context.Context, name string, opts dockerclient.NetworkCreateOptions) (network.CreateResponse, error) {
	res, err := c.cli.NetworkCreate(ctx, name, opts)
	return network.CreateResponse{ID: res.ID, Warning: strings.Join(res.Warning, "\n")}, err
}
