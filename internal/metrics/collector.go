package metrics

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"time"

	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/api/types/filters"
	dockercontainer "github.com/docker/docker/api/types/container"
	"github.com/shirou/gopsutil/v4/cpu"
	"github.com/shirou/gopsutil/v4/mem"
	"github.com/vazra/simpledeploy/internal/docker"
)

// AppLookup resolves an app slug to its database ID.
// Return (0, err) when the app is not found.
type AppLookup func(slug string) (int64, error)

// Collector gathers system and container metrics.
type Collector struct {
	docker    docker.Client
	appLookup AppLookup
	out       chan<- MetricPoint
}

// NewCollector creates a new Collector.
// appLookup may be nil; if so, AppID will always be nil.
func NewCollector(dc docker.Client, appLookup AppLookup, out chan<- MetricPoint) *Collector {
	return &Collector{docker: dc, appLookup: appLookup, out: out}
}

// CollectSystem collects host-level CPU, memory metrics.
func (c *Collector) CollectSystem() (MetricPoint, error) {
	pcts, err := cpu.Percent(0, false)
	if err != nil {
		return MetricPoint{}, fmt.Errorf("cpu.Percent: %w", err)
	}
	var cpuPct float64
	if len(pcts) > 0 {
		cpuPct = pcts[0]
	}

	vm, err := mem.VirtualMemory()
	if err != nil {
		return MetricPoint{}, fmt.Errorf("mem.VirtualMemory: %w", err)
	}

	return MetricPoint{
		AppID:       nil,
		ContainerID: "",
		CPUPct:      cpuPct,
		MemBytes:    int64(vm.Used),
		MemLimit:    int64(vm.Total),
		DiskRead:    0,
		DiskWrite:   0,
		Tier:        TierRaw,
		Timestamp:   time.Now(),
	}, nil
}

// CollectContainers collects metrics for all simpledeploy containers.
func (c *Collector) CollectContainers(ctx context.Context) ([]MetricPoint, error) {
	f := filters.NewArgs(filters.Arg("label", "simpledeploy.project"))
	containers, err := c.docker.ContainerList(ctx, container.ListOptions{Filters: f})
	if err != nil {
		return nil, fmt.Errorf("ContainerList: %w", err)
	}

	var points []MetricPoint
	for _, ctr := range containers {
		pt, err := c.collectContainer(ctx, ctr)
		if err != nil {
			log.Printf("metrics: skip container %s: %v", ctr.ID[:12], err)
			continue
		}
		points = append(points, pt)
	}
	return points, nil
}

func (c *Collector) collectContainer(ctx context.Context, ctr dockercontainer.Summary) (MetricPoint, error) {
	resp, err := c.docker.ContainerStats(ctx, ctr.ID)
	if err != nil {
		return MetricPoint{}, fmt.Errorf("ContainerStats: %w", err)
	}
	defer resp.Body.Close()

	var stats dockercontainer.StatsResponse
	if err := json.NewDecoder(resp.Body).Decode(&stats); err != nil {
		return MetricPoint{}, fmt.Errorf("decode stats: %w", err)
	}

	// CPU
	cpuDelta := float64(stats.CPUStats.CPUUsage.TotalUsage) - float64(stats.PreCPUStats.CPUUsage.TotalUsage)
	sysDelta := float64(stats.CPUStats.SystemUsage) - float64(stats.PreCPUStats.SystemUsage)
	numCPUs := float64(stats.CPUStats.OnlineCPUs)
	if numCPUs == 0 {
		numCPUs = 1
	}
	var cpuPct float64
	if sysDelta > 0 {
		cpuPct = (cpuDelta / sysDelta) * numCPUs * 100
	}

	// Memory
	memBytes := int64(stats.MemoryStats.Usage)
	memLimit := int64(stats.MemoryStats.Limit)

	// Network
	var netRx, netTx int64
	for _, ns := range stats.Networks {
		netRx += int64(ns.RxBytes)
		netTx += int64(ns.TxBytes)
	}

	// Block IO
	var diskRead, diskWrite int64
	for _, entry := range stats.BlkioStats.IoServiceBytesRecursive {
		switch entry.Op {
		case "Read":
			diskRead += int64(entry.Value)
		case "Write":
			diskWrite += int64(entry.Value)
		}
	}

	// Map container to app via simpledeploy.project label
	var appID *int64
	if slug, ok := ctr.Labels["simpledeploy.project"]; ok && slug != "" && c.appLookup != nil {
		if id, err := c.appLookup(slug); err == nil {
			appID = &id
		}
	}

	return MetricPoint{
		AppID:       appID,
		ContainerID: ctr.ID,
		CPUPct:      cpuPct,
		MemBytes:    memBytes,
		MemLimit:    memLimit,
		NetRx:       netRx,
		NetTx:       netTx,
		DiskRead:    diskRead,
		DiskWrite:   diskWrite,
		Tier:        TierRaw,
		Timestamp:   time.Now(),
	}, nil
}

// Run collects metrics on every interval tick until ctx is cancelled.
func (c *Collector) Run(ctx context.Context, interval time.Duration) {
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			if pt, err := c.CollectSystem(); err == nil {
				select {
				case c.out <- pt:
				case <-ctx.Done():
					return
				}
			} else {
				log.Printf("metrics: CollectSystem: %v", err)
			}

			pts, err := c.CollectContainers(ctx)
			if err != nil {
				log.Printf("metrics: CollectContainers: %v", err)
				continue
			}
			for _, pt := range pts {
				select {
				case c.out <- pt:
				case <-ctx.Done():
					return
				}
			}
		}
	}
}
