package api

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestDockerEndpoints_NoAuth(t *testing.T) {
	srv := NewServer(0, nil, nil, nil)

	endpoints := []struct {
		method string
		path   string
	}{
		{"GET", "/api/docker/disk-usage"},
		{"POST", "/api/docker/prune/containers"},
		{"POST", "/api/docker/prune/images"},
		{"POST", "/api/docker/prune/volumes"},
		{"POST", "/api/docker/prune/build-cache"},
		{"POST", "/api/docker/prune/all"},
		{"GET", "/api/docker/images"},
		{"DELETE", "/api/docker/images/sha256:abc123"},
		{"GET", "/api/docker/networks"},
		{"GET", "/api/docker/volumes"},
		{"DELETE", "/api/docker/networks/net1"},
		{"DELETE", "/api/docker/volumes/vol1"},
	}

	for _, ep := range endpoints {
		t.Run(ep.method+" "+ep.path, func(t *testing.T) {
			req := httptest.NewRequest(ep.method, ep.path, nil)
			w := httptest.NewRecorder()
			srv.Handler().ServeHTTP(w, req)
			if w.Code != http.StatusUnauthorized {
				t.Fatalf("status = %d, want %d", w.Code, http.StatusUnauthorized)
			}
		})
	}
}

func TestDockerEndpoints_NoDockerClient(t *testing.T) {
	// Server with no docker client returns 503
	srv := NewServer(0, nil, nil, nil)

	endpoints := []struct {
		method string
		path   string
	}{
		{"GET", "/api/docker/disk-usage"},
		{"POST", "/api/docker/prune/containers"},
		{"POST", "/api/docker/prune/images"},
		{"POST", "/api/docker/prune/volumes"},
		{"POST", "/api/docker/prune/build-cache"},
		{"POST", "/api/docker/prune/all"},
		{"GET", "/api/docker/images"},
		{"DELETE", "/api/docker/images/sha256:abc123"},
		{"GET", "/api/docker/networks"},
		{"GET", "/api/docker/volumes"},
		{"DELETE", "/api/docker/networks/net1"},
		{"DELETE", "/api/docker/volumes/vol1"},
	}

	for _, ep := range endpoints {
		t.Run(ep.method+" "+ep.path, func(t *testing.T) {
			req := httptest.NewRequest(ep.method, ep.path, nil)
			// Bypass auth by calling handler directly
			w := httptest.NewRecorder()
			switch ep.path {
			case "/api/docker/disk-usage":
				srv.handleDockerDiskUsage(w, req)
			case "/api/docker/prune/containers":
				srv.handleDockerPruneContainers(w, req)
			case "/api/docker/prune/images":
				srv.handleDockerPruneImages(w, req)
			case "/api/docker/prune/volumes":
				srv.handleDockerPruneVolumes(w, req)
			case "/api/docker/prune/build-cache":
				srv.handleDockerPruneBuildCache(w, req)
			case "/api/docker/prune/all":
				srv.handleDockerPruneAll(w, req)
			case "/api/docker/images":
				srv.handleDockerImages(w, req)
			case "/api/docker/networks":
				srv.handleDockerNetworks(w, req)
			case "/api/docker/volumes":
				srv.handleDockerVolumes(w, req)
			default:
				// DELETE endpoints
				switch ep.path {
				case "/api/docker/images/sha256:abc123":
					srv.handleDockerImageRemove(w, req)
				case "/api/docker/networks/net1":
					srv.handleDockerNetworkRemove(w, req)
				case "/api/docker/volumes/vol1":
					srv.handleDockerVolumeRemove(w, req)
				}
			}
			if w.Code != http.StatusServiceUnavailable {
				t.Fatalf("status = %d, want %d", w.Code, http.StatusServiceUnavailable)
			}
		})
	}
}
