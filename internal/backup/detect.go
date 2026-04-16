package backup

import "github.com/vazra/simpledeploy/internal/compose"

// DetectionResult summarizes what a single strategy found for an app.
type DetectionResult struct {
	StrategyType string            `json:"strategy_type"`
	Label        string            `json:"label"`
	Services     []DetectedService `json:"services"`
	Available    bool              `json:"available"`
	Description  string            `json:"description"`
}

// Detector runs all registered strategies against a compose config.
type Detector struct {
	strategies []Strategy
}

func NewDetector() *Detector { return &Detector{} }

func (d *Detector) Register(s Strategy) { d.strategies = append(d.strategies, s) }

func (d *Detector) DetectAll(cfg *compose.AppConfig) []DetectionResult {
	var results []DetectionResult
	for _, s := range d.strategies {
		services := s.Detect(cfg)
		result := DetectionResult{
			StrategyType: s.Type(),
			Label:        strategyDisplayLabel(s.Type()),
			Services:     services,
			Available:    len(services) > 0,
			Description:  strategyDescription(s.Type(), len(services) > 0),
		}
		results = append(results, result)
	}
	return results
}

func strategyDisplayLabel(t string) string {
	switch t {
	case "postgres":
		return "PostgreSQL"
	case "mysql":
		return "MySQL / MariaDB"
	case "mongo":
		return "MongoDB"
	case "redis":
		return "Redis"
	case "sqlite":
		return "SQLite"
	case "volume":
		return "Volume Snapshot"
	default:
		return t
	}
}

func strategyDescription(t string, available bool) string {
	if !available {
		return strategyUnavailableDesc(t)
	}
	switch t {
	case "postgres":
		return "Backs up via pg_dump inside the container"
	case "mysql":
		return "Backs up via mysqldump inside the container"
	case "mongo":
		return "Backs up via mongodump inside the container"
	case "redis":
		return "Triggers BGSAVE and copies the RDB file"
	case "sqlite":
		return "Uses .backup command for consistent snapshots"
	case "volume":
		return "Archives mounted volumes via tar"
	default:
		return ""
	}
}

func strategyUnavailableDesc(t string) string {
	switch t {
	case "postgres":
		return "No PostgreSQL service detected"
	case "mysql":
		return "No MySQL/MariaDB service detected"
	case "mongo":
		return "No MongoDB service detected"
	case "redis":
		return "No Redis service detected"
	case "sqlite":
		return "No SQLite service detected (requires label)"
	case "volume":
		return "No mounted volumes detected"
	default:
		return "Not detected"
	}
}
