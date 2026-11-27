package api

import (
	"encoding/json"
	"net/http"
	"net/url"
	"os"

	dockerclient "github.com/moby/moby/client"
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

func (s *Server) handleDockerInfo(w http.ResponseWriter, r *http.Request) {
	cli, ok := s.dockerRaw()
	if !ok {
		http.Error(w, "docker not available", http.StatusServiceUnavailable)
		return
	}
	res, err := cli.Info(r.Context(), dockerclient.InfoOptions{})
	if err != nil {
		httpError(w, err, http.StatusInternalServerError)
		return
	}
	info := res.Info
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]any{
		"server_version":     info.ServerVersion,
		"os":                 info.OperatingSystem,
		"arch":               info.Architecture,
		"kernel":             info.KernelVersion,
		"cpus":               info.NCPU,
		"memory":             info.MemTotal,
		"containers":         info.Containers,
		"containers_running": info.ContainersRunning,
		"containers_paused":  info.ContainersPaused,
		"containers_stopped": info.ContainersStopped,
		"images":             info.Images,
		"storage_driver":     info.Driver,
		"docker_root_dir":    info.DockerRootDir,
		"logging_driver":     info.LoggingDriver,
		"logging_options":    readDaemonLogOpts(),
	})
}

func readDaemonLogOpts() map[string]string {
	data, err := os.ReadFile("/etc/docker/daemon.json")
	if err != nil {
		return nil
	}
	var cfg struct {
		LogOpts map[string]string `json:"log-opts"`
	}
	if json.Unmarshal(data, &cfg) != nil {
		return nil
	}
	return cfg.LogOpts
}

func (s *Server) handleDockerDiskUsage(w http.ResponseWriter, r *http.Request) {
	cli, ok := s.dockerRaw()
	if !ok {
		http.Error(w, "docker not available", http.StatusServiceUnavailable)
		return
	}
	du, err := cli.DiskUsage(r.Context(), dockerclient.DiskUsageOptions{
		Containers: true, Images: true, BuildCache: true, Volumes: true, Verbose: true,
	})
	if err != nil {
		httpError(w, err, http.StatusInternalServerError)
		return
	}
	// Keep the legacy /system/df response shape the UI expects.
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]any{
		"LayersSize": du.Images.TotalSize,
		"Images":     du.Images.Items,
		"Containers": du.Containers.Items,
		"Volumes":    du.Volumes.Items,
		"BuildCache": du.BuildCache.Items,
	})
}

func (s *Server) handleDockerPruneContainers(w http.ResponseWriter, r *http.Request) {
	cli, ok := s.dockerRaw()
	if !ok {
		http.Error(w, "docker not available", http.StatusServiceUnavailable)
		return
	}
	res, err := cli.ContainerPrune(r.Context(), dockerclient.ContainerPruneOptions{})
	if err != nil {
		httpError(w, err, http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(res.Report)
}

func (s *Server) handleDockerPruneImages(w http.ResponseWriter, r *http.Request) {
	cli, ok := s.dockerRaw()
	if !ok {
		http.Error(w, "docker not available", http.StatusServiceUnavailable)
		return
	}
	res, err := cli.ImagePrune(r.Context(), dockerclient.ImagePruneOptions{Filters: make(dockerclient.Filters).Add("dangling", "false")})
	if err != nil {
		httpError(w, err, http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(res.Report)
}

func (s *Server) handleDockerPruneVolumes(w http.ResponseWriter, r *http.Request) {
	cli, ok := s.dockerRaw()
	if !ok {
		http.Error(w, "docker not available", http.StatusServiceUnavailable)
		return
	}
	res, err := cli.VolumePrune(r.Context(), dockerclient.VolumePruneOptions{})
	if err != nil {
		httpError(w, err, http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(res.Report)
}

func (s *Server) handleDockerPruneBuildCache(w http.ResponseWriter, r *http.Request) {
	cli, ok := s.dockerRaw()
	if !ok {
		http.Error(w, "docker not available", http.StatusServiceUnavailable)
		return
	}
	res, err := cli.BuildCachePrune(r.Context(), dockerclient.BuildCachePruneOptions{})
	if err != nil {
		httpError(w, err, http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(res.Report)
}

func (s *Server) handleDockerPruneAll(w http.ResponseWriter, r *http.Request) {
	cli, ok := s.dockerRaw()
	if !ok {
		http.Error(w, "docker not available", http.StatusServiceUnavailable)
		return
	}
	ctx := r.Context()
	containersRes, err := cli.ContainerPrune(ctx, dockerclient.ContainerPruneOptions{})
	if err != nil {
		httpError(w, err, http.StatusInternalServerError)
		return
	}
	imagesRes, err := cli.ImagePrune(ctx, dockerclient.ImagePruneOptions{Filters: make(dockerclient.Filters).Add("dangling", "false")})
	if err != nil {
		httpError(w, err, http.StatusInternalServerError)
		return
	}
	volumesRes, err := cli.VolumePrune(ctx, dockerclient.VolumePruneOptions{})
	if err != nil {
		httpError(w, err, http.StatusInternalServerError)
		return
	}
	buildCacheRes, err := cli.BuildCachePrune(ctx, dockerclient.BuildCachePruneOptions{})
	if err != nil {
		httpError(w, err, http.StatusInternalServerError)
		return
	}
	containers, images := containersRes.Report, imagesRes.Report
	volumes, buildCache := volumesRes.Report, buildCacheRes.Report

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
	res, err := cli.ImageList(r.Context(), dockerclient.ImageListOptions{})
	if err != nil {
		httpError(w, err, http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(res.Items)
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
	res, err := cli.ImageRemove(r.Context(), id, dockerclient.ImageRemoveOptions{})
	if err != nil {
		httpError(w, err, http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(res.Items)
}

func (s *Server) handleDockerNetworks(w http.ResponseWriter, r *http.Request) {
	cli, ok := s.dockerRaw()
	if !ok {
		http.Error(w, "docker not available", http.StatusServiceUnavailable)
		return
	}
	res, err := cli.NetworkList(r.Context(), dockerclient.NetworkListOptions{})
	if err != nil {
		httpError(w, err, http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(res.Items)
}

func (s *Server) handleDockerVolumes(w http.ResponseWriter, r *http.Request) {
	cli, ok := s.dockerRaw()
	if !ok {
		http.Error(w, "docker not available", http.StatusServiceUnavailable)
		return
	}
	res, err := cli.VolumeList(r.Context(), dockerclient.VolumeListOptions{})
	if err != nil {
		httpError(w, err, http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]any{"Volumes": res.Items, "Warnings": res.Warnings})
}

func (s *Server) handleDockerNetworkRemove(w http.ResponseWriter, r *http.Request) {
	cli, ok := s.dockerRaw()
	if !ok {
		http.Error(w, "docker not available", http.StatusServiceUnavailable)
		return
	}
	id := r.PathValue("id")
	if _, err := cli.NetworkRemove(r.Context(), id, dockerclient.NetworkRemoveOptions{}); err != nil {
		httpError(w, err, http.StatusInternalServerError)
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
	if _, err := cli.VolumeRemove(r.Context(), name, dockerclient.VolumeRemoveOptions{}); err != nil {
		httpError(w, err, http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
}
