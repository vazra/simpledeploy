package api

import (
	"encoding/json"
	"net/http"
	"net/url"

	"github.com/docker/docker/api/types"
	"github.com/docker/docker/api/types/filters"
	"github.com/docker/docker/api/types/image"
	"github.com/docker/docker/api/types/network"
	"github.com/docker/docker/api/types/volume"
	dockerclient "github.com/docker/docker/client"
)

func (s *Server) dockerRaw() (*dockerclient.Client, bool) {
	if s.docker == nil {
		return nil, false
	}
	cli := s.docker.Raw()
	if cli == nil {
		return nil, false
	}
	return cli, true
}

func (s *Server) handleDockerDiskUsage(w http.ResponseWriter, r *http.Request) {
	cli, ok := s.dockerRaw()
	if !ok {
		http.Error(w, "docker not available", http.StatusServiceUnavailable)
		return
	}
	du, err := cli.DiskUsage(r.Context(), types.DiskUsageOptions{})
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(du)
}

func (s *Server) handleDockerPruneContainers(w http.ResponseWriter, r *http.Request) {
	cli, ok := s.dockerRaw()
	if !ok {
		http.Error(w, "docker not available", http.StatusServiceUnavailable)
		return
	}
	report, err := cli.ContainersPrune(r.Context(), filters.NewArgs())
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(report)
}

func (s *Server) handleDockerPruneImages(w http.ResponseWriter, r *http.Request) {
	cli, ok := s.dockerRaw()
	if !ok {
		http.Error(w, "docker not available", http.StatusServiceUnavailable)
		return
	}
	report, err := cli.ImagesPrune(r.Context(), filters.NewArgs())
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(report)
}

func (s *Server) handleDockerPruneVolumes(w http.ResponseWriter, r *http.Request) {
	cli, ok := s.dockerRaw()
	if !ok {
		http.Error(w, "docker not available", http.StatusServiceUnavailable)
		return
	}
	report, err := cli.VolumesPrune(r.Context(), filters.NewArgs())
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(report)
}

func (s *Server) handleDockerPruneBuildCache(w http.ResponseWriter, r *http.Request) {
	cli, ok := s.dockerRaw()
	if !ok {
		http.Error(w, "docker not available", http.StatusServiceUnavailable)
		return
	}
	report, err := cli.BuildCachePrune(r.Context(), types.BuildCachePruneOptions{})
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(report)
}

func (s *Server) handleDockerPruneAll(w http.ResponseWriter, r *http.Request) {
	cli, ok := s.dockerRaw()
	if !ok {
		http.Error(w, "docker not available", http.StatusServiceUnavailable)
		return
	}
	ctx := r.Context()
	empty := filters.NewArgs()

	containers, err := cli.ContainersPrune(ctx, empty)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	images, err := cli.ImagesPrune(ctx, empty)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	volumes, err := cli.VolumesPrune(ctx, empty)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	buildCache, err := cli.BuildCachePrune(ctx, types.BuildCachePruneOptions{})
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	totalReclaimed := containers.SpaceReclaimed + images.SpaceReclaimed +
		volumes.SpaceReclaimed + buildCache.SpaceReclaimed

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]any{
		"containers":      containers,
		"images":          images,
		"volumes":         volumes,
		"build_cache":     buildCache,
		"space_reclaimed": totalReclaimed,
	})
}

func (s *Server) handleDockerImages(w http.ResponseWriter, r *http.Request) {
	cli, ok := s.dockerRaw()
	if !ok {
		http.Error(w, "docker not available", http.StatusServiceUnavailable)
		return
	}
	imgs, err := cli.ImageList(r.Context(), image.ListOptions{All: true})
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(imgs)
}

func (s *Server) handleDockerImageRemove(w http.ResponseWriter, r *http.Request) {
	cli, ok := s.dockerRaw()
	if !ok {
		http.Error(w, "docker not available", http.StatusServiceUnavailable)
		return
	}
	id, err := url.PathUnescape(r.PathValue("id"))
	if err != nil {
		http.Error(w, "invalid image id", http.StatusBadRequest)
		return
	}
	dels, err := cli.ImageRemove(r.Context(), id, image.RemoveOptions{})
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(dels)
}

func (s *Server) handleDockerNetworks(w http.ResponseWriter, r *http.Request) {
	cli, ok := s.dockerRaw()
	if !ok {
		http.Error(w, "docker not available", http.StatusServiceUnavailable)
		return
	}
	nets, err := cli.NetworkList(r.Context(), network.ListOptions{})
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(nets)
}

func (s *Server) handleDockerVolumes(w http.ResponseWriter, r *http.Request) {
	cli, ok := s.dockerRaw()
	if !ok {
		http.Error(w, "docker not available", http.StatusServiceUnavailable)
		return
	}
	vols, err := cli.VolumeList(r.Context(), volume.ListOptions{})
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(vols)
}

func (s *Server) handleDockerNetworkRemove(w http.ResponseWriter, r *http.Request) {
	cli, ok := s.dockerRaw()
	if !ok {
		http.Error(w, "docker not available", http.StatusServiceUnavailable)
		return
	}
	id := r.PathValue("id")
	if err := cli.NetworkRemove(r.Context(), id); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
}

func (s *Server) handleDockerVolumeRemove(w http.ResponseWriter, r *http.Request) {
	cli, ok := s.dockerRaw()
	if !ok {
		http.Error(w, "docker not available", http.StatusServiceUnavailable)
		return
	}
	name := r.PathValue("name")
	if err := cli.VolumeRemove(r.Context(), name, false); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
}
