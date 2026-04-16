package alerts

import (
	"fmt"
	"time"
)

type AlertEvent struct {
	AppName   string
	AppSlug   string
	Metric    string
	MetricDisplay string // human-friendly metric name
	Value     float64
	ValueDisplay string // human-friendly value (e.g. "26.3 GB")
	Threshold float64
	ThresholdDisplay string // human-friendly threshold
	Operator  string
	Status    string // "firing", "resolved"
	FiredAt   time.Time
}

var metricDisplayNames = map[string]string{
	"cpu_pct":   "CPU",
	"mem_pct":   "Memory",
	"mem_bytes": "Memory",
}

func formatBytes(b float64) string {
	switch {
	case b >= 1<<30:
		return fmt.Sprintf("%.1f GB", b/(1<<30))
	case b >= 1<<20:
		return fmt.Sprintf("%.1f MB", b/(1<<20))
	case b >= 1<<10:
		return fmt.Sprintf("%.1f KB", b/(1<<10))
	default:
		return fmt.Sprintf("%.0f B", b)
	}
}

func formatMetricValue(metric string, value float64) string {
	switch metric {
	case "mem_bytes":
		return formatBytes(value)
	case "cpu_pct", "mem_pct":
		return fmt.Sprintf("%.1f%%", value)
	default:
		return fmt.Sprintf("%.1f", value)
	}
}

// BackupAlertEvent represents a backup-related alert.
type BackupAlertEvent struct {
	AppName   string
	Strategy  string
	Message   string
	EventType string // "backup_failed" or "backup_missed"
	FiredAt   time.Time
}

func (b BackupAlertEvent) ToAlertEvent() AlertEvent {
	metricDisplay := "Backup Failed"
	if b.EventType == "backup_missed" {
		metricDisplay = "Backup Missed"
	}
	return AlertEvent{
		AppName:      b.AppName,
		Metric:       b.EventType,
		MetricDisplay: metricDisplay,
		ValueDisplay: b.Message,
		Status:       "firing",
		FiredAt:      b.FiredAt,
	}
}

func EnrichEvent(event *AlertEvent) {
	if name, ok := metricDisplayNames[event.Metric]; ok {
		event.MetricDisplay = name
	} else {
		event.MetricDisplay = event.Metric
	}
	event.ValueDisplay = formatMetricValue(event.Metric, event.Value)
	event.ThresholdDisplay = formatMetricValue(event.Metric, event.Threshold)
	if event.AppName == "" {
		event.AppName = "All Apps"
	}
}
