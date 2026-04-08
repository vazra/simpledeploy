package alerts

import "time"

type AlertEvent struct {
	AppName   string
	AppSlug   string
	Metric    string
	Value     float64
	Threshold float64
	Operator  string
	Status    string // "firing", "resolved"
	FiredAt   time.Time
}
