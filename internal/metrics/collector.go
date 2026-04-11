package metrics

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"strings"
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

type StatusSyncer interface {
	ListApps() ([]StatusApp, error)
	UpdateAppStatus(slug, status string) error
}

type StatusApp struct {
	Slug   string
	Status string
}

// containerCounters holds cumulative Docker counters for rate computation.
type containerCounters struct {
	netRx     uint64
	netTx     uint64
	diskRead  uint64
	diskWrite uint64
	ts        time.Time
}

// Collector gathers system and container metrics.
type Collector struct {
	docker       docker.Client
	appLookup    AppLookup
	statusSyncer StatusSyncer
	out          chan<- MetricPoint
	prevCounters map[string]*containerCounters // key: container ID
}

// NewCollector creates a new Collector.
// appLookup may be nil; if so, AppID will always be nil.
func NewCollector(dc docker.Client, appLookup AppLookup, out chan<- MetricPoint) *Collector {
	return &Collector{docker: dc, appLookup: appLookup, out: out, prevCounters: make(map[string]*containerCounters)}
}

func (c *Collector) SetStatusSyncer(ss StatusSyncer) {
	c.statusSyncer = ss
}

func (c *Collector) syncStatus(ctx context.Context) {
	if c.statusSyncer == nil {
		return
	}
	apps, err := c.statusSyncer.ListApps()
	if err != nil {
		log.Printf("metrics: syncStatus ListApps: %v", err)
		return
	}

	f := filters.NewArgs(filters.Arg("label", "com.docker.compose.project"))
	containers, err := c.docker.ContainerList(ctx, container.ListOptions{Filters: f})
	if err != nil {
		log.Printf("metrics: syncStatus ContainerList: %v", err)
		return
	}

	running := make(map[string]bool)
	for _, ctr := range containers {
		if project, ok := ctr.Labels["com.docker.compose.project"]; ok {
			slug := strings.TrimPrefix(project, "simpledeploy-")
			running[slug] = true
		}
	}

	for _, app := range apps {
		if app.Status == "stopped" {
			continue
		}
		if running[app.Slug] && app.Status != "running" {
			c.statusSyncer.UpdateAppStatus(app.Slug, "running")
		} else if !running[app.Slug] && app.Status == "running" {
			c.statusSyncer.UpdateAppStatus(app.Slug, "error")
		}
	}
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
		Ts:          time.Now().Unix(),
	}, nil
}

// CollectContainers collects metrics for all simpledeploy containers.
func (c *Collector) CollectContainers(ctx context.Context) ([]MetricPoint, error) {
	f := filters.NewArgs(filters.Arg("label", "com.docker.compose.project"))
	containers, err := c.docker.ContainerList(ctx, container.ListOptions{Filters: f})
	if err != nil {
		return nil, fmt.Errorf("ContainerList: %w", err)
	}

	seen := make(map[string]bool, len(containers))
	var points []MetricPoint
	for _, ctr := range containers {
		seen[ctr.ID] = true
		pt, err := c.collectContainer(ctx, ctr)
		if err != nil {
			log.Printf("metrics: skip container %s: %v", ctr.ID[:12], err)
			continue
		}
		points = append(points, pt)
	}
	// Clean stale entries from prevCounters
	for id := range c.prevCounters {
		if !seen[id] {
			delete(c.prevCounters, id)
		}
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

	// Network: sum cumulative byte counters across interfaces
	var curNetRx, curNetTx uint64
	for _, ns := range stats.Networks {
		curNetRx += ns.RxBytes
		curNetTx += ns.TxBytes
	}

	// Block IO: sum cumulative byte counters
	var curDiskRead, curDiskWrite uint64
	for _, entry := range stats.BlkioStats.IoServiceBytesRecursive {
		switch entry.Op {
		case "Read":
			curDiskRead += entry.Value
		case "Write":
			curDiskWrite += entry.Value
		}
	}

	// Compute rates from deltas
	now := time.Now()
	var netRxRate, netTxRate, diskReadRate, diskWriteRate float64
	if prev, ok := c.prevCounters[ctr.ID]; ok {
		elapsed := now.Sub(prev.ts).Seconds()
		if elapsed > 0 {
			// Handle counter resets (current < previous): output 0
			if curNetRx >= prev.netRx {
				netRxRate = float64(curNetRx-prev.netRx) / elapsed
			}
			if curNetTx >= prev.netTx {
				netTxRate = float64(curNetTx-prev.netTx) / elapsed
			}
			if curDiskRead >= prev.diskRead {
				diskReadRate = float64(curDiskRead-prev.diskRead) / elapsed
			}
			if curDiskWrite >= prev.diskWrite {
				diskWriteRate = float64(curDiskWrite-prev.diskWrite) / elapsed
			}
		}
	}
	// Store current counters for next cycle
	c.prevCounters[ctr.ID] = &containerCounters{
		netRx:     curNetRx,
		netTx:     curNetTx,
		diskRead:  curDiskRead,
		diskWrite: curDiskWrite,
		ts:        now,
	}

	// Map container to app via compose project label
	var appID *int64
	if project, ok := ctr.Labels["com.docker.compose.project"]; ok && project != "" && c.appLookup != nil {
		slug := strings.TrimPrefix(project, "simpledeploy-")
		if id, err := c.appLookup(slug); err == nil {
			appID = &id
		}
	}

	// Use compose service name for container_id (e.g. "web", "redis")
	containerID := ctr.ID
	if svc, ok := ctr.Labels["com.docker.compose.service"]; ok && svc != "" {
		containerID = svc
	}

	return MetricPoint{
		AppID:       appID,
		ContainerID: containerID,
		CPUPct:      cpuPct,
		MemBytes:    memBytes,
		MemLimit:    memLimit,
		NetRx:       netRxRate,
		NetTx:       netTxRate,
		DiskRead:    diskReadRate,
		DiskWrite:   diskWriteRate,
		Tier:        TierRaw,
		Ts:          now.Unix(),
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
			c.syncStatus(ctx)
		}
	}
}
