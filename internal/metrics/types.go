package metrics

type MetricPoint struct {
	AppID       *int64
	ContainerID string
	CPUPct      float64
	MemBytes    int64
	MemLimit    int64
	NetRx       float64
	NetTx       float64
	DiskRead    float64
	DiskWrite   float64
	Ts          int64
	Tier        string
}

type RequestMetricPoint struct {
	AppID      int64
	Ts         int64
	Tier       string
	Count      int64
	ErrorCount int64
	AvgLatency float64
	MaxLatency float64
}

const (
	TierRaw = "raw"
	Tier1m  = "1m"
	Tier5m  = "5m"
	Tier1h  = "1h"
	Tier1d  = "1d"
)
