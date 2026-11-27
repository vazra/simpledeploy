package proxy

import (
	"context"
	dockerclient "github.com/moby/moby/client"

	"github.com/moby/moby/api/types/container"
)

// dockerInspector is the narrow subset of docker.Client needed by DockerResolver.
type dockerInspector interface {
	ContainerList(ctx context.Context, opts dockerclient.ContainerListOptions) ([]container.Summary, error)
	ContainerInspect(ctx context.Context, id string) (container.InspectResponse, error)
}

// DockerResolver resolves <service>:<port> upstreams to <container-ip>:<port>
// via Docker API, using compose project/service labels.
type DockerResolver struct {
	Client dockerInspector
}

// ContainerIP returns the IP of the first running container matching the given
// compose project and service, on the given docker network. Returns "" (no
// error) when the container exists but is not yet attached to the network, or
// when no container matches. Returns a non-nil error only on Docker failures.
func (r *DockerResolver) ContainerIP(project, service, netName string) (string, error) {
	if r == nil || r.Client == nil {
		return "", nil
	}
	ctx := context.Background()
	f := make(dockerclient.Filters)
	f.Add("label", "com.docker.compose.project="+project)
	f.Add("label", "com.docker.compose.service="+service)
	f.Add("status", "running")
	list, err := r.Client.ContainerList(ctx, dockerclient.ContainerListOptions{Filters: f, All: false})
	if err != nil {
		return "", err
	}
	if len(list) == 0 {
		return "", nil
	}
	insp, err := r.Client.ContainerInspect(ctx, list[0].ID)
	if err != nil {
		return "", err
	}
	if insp.NetworkSettings == nil {
		return "", nil
	}
	net, ok := insp.NetworkSettings.Networks[netName]
	if !ok || net == nil {
		return "", nil
	}
	if !net.IPAddress.IsValid() {
		return "", nil
	}
	return net.IPAddress.String(), nil
}
