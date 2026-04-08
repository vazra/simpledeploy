package metrics

import "time"

type MetricPoint struct {
	AppID       *int64
	ContainerID string
	CPUPct      float64
	MemBytes    int64
	MemLimit    int64
	NetRx       int64
	NetTx       int64
	DiskRead    int64
	DiskWrite   int64
	Timestamp   time.Time
	Tier        string
}

const (
	TierRaw = "raw"
	Tier1m  = "1m"
	Tier5m  = "5m"
	Tier1h  = "1h"
)
