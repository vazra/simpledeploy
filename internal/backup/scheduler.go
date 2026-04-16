package backup

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"strings"
	"sync"
	"time"

	"github.com/robfig/cron/v3"
	"github.com/vazra/simpledeploy/internal/store"
)

// BackupAlertFunc is a callback for backup-related alerts.
type BackupAlertFunc func(appName, strategy, message, eventType string)

// BackupStore is the subset of store.Store needed by Scheduler.
type BackupStore interface {
	ListBackupConfigs(appID *int64) ([]store.BackupConfig, error)
	GetBackupConfig(id int64) (*store.BackupConfig, error)
	CreateBackupRun(configID int64) (*store.BackupRun, error)
	UpdateBackupRunSuccess(id int64, sizeBytes int64, filePath, checksum string) error
	UpdateBackupRunFailed(id int64, errMsg string) error
	ListOldBackupRuns(configID int64, keepCount int) ([]store.BackupRun, error)
	ListOldBackupRunsByTime(configID int64, maxAgeDays int) ([]store.BackupRun, error)
	GetAppByID(id int64) (*store.App, error)
	GetBackupRun(id int64) (*store.BackupRun, error)
}

// Scheduler runs scheduled backups and handles restore requests.
type Scheduler struct {
	store      BackupStore
	strategies map[string]Strategy
	targets    map[string]func(configJSON string) (Target, error)
	cron       *cron.Cron
	entries    map[int64]cron.EntryID
	hookExec   ContainerExecutor
	alertFunc  BackupAlertFunc
	mu         sync.Mutex
}

// NewScheduler creates a new Scheduler backed by st.
func NewScheduler(st BackupStore, hookExec ContainerExecutor) *Scheduler {
	return &Scheduler{
		store:      st,
		strategies: make(map[string]Strategy),
		targets:    make(map[string]func(string) (Target, error)),
		cron:       cron.New(),
		entries:    make(map[int64]cron.EntryID),
		hookExec:   hookExec,
	}
}

// RegisterStrategy registers a named backup strategy.
func (s *Scheduler) RegisterStrategy(name string, strategy Strategy) {
	s.strategies[name] = strategy
}

// RegisterTargetFactory registers a factory function for the named target type.
func (s *Scheduler) RegisterTargetFactory(name string, factory func(string) (Target, error)) {
	s.targets[name] = factory
}

// SetAlertFunc sets the callback for backup alerts.
func (s *Scheduler) SetAlertFunc(fn BackupAlertFunc) {
	s.alertFunc = fn
}

// GetStrategy returns a registered strategy by name.
func (s *Scheduler) GetStrategy(name string) (Strategy, bool) {
	st, ok := s.strategies[name]
	return st, ok
}

// GetDetector returns a Detector with all registered strategies.
func (s *Scheduler) GetDetector() *Detector {
	d := NewDetector()
	for _, st := range s.strategies {
		d.Register(st)
	}
	return d
}

// ScheduleConfig adds or replaces a cron entry for the given config.
func (s *Scheduler) ScheduleConfig(configID int64, cronExpr string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	// Remove existing entry if present
	if entryID, ok := s.entries[configID]; ok {
		s.cron.Remove(entryID)
		delete(s.entries, configID)
	}

	cfgID := configID
	entryID, err := s.cron.AddFunc(cronExpr, func() {
		ctx := context.Background()
		if err := s.RunBackup(ctx, cfgID); err != nil {
			log.Printf("scheduled backup config=%d: %v", cfgID, err)
		}
	})
	if err != nil {
		return fmt.Errorf("add cron for config=%d: %w", configID, err)
	}

	s.entries[configID] = entryID
	return nil
}

// UnscheduleConfig removes the cron entry for the given config.
func (s *Scheduler) UnscheduleConfig(configID int64) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if entryID, ok := s.entries[configID]; ok {
		s.cron.Remove(entryID)
		delete(s.entries, configID)
	}
}

// Start loads all backup configs and schedules cron entries, then starts the cron runner.
func (s *Scheduler) Start() error {
	configs, err := s.store.ListBackupConfigs(nil)
	if err != nil {
		return fmt.Errorf("list backup configs: %w", err)
	}

	for _, cfg := range configs {
		if cfg.ScheduleCron == "" {
			continue
		}
		if err := s.ScheduleConfig(cfg.ID, cfg.ScheduleCron); err != nil {
			log.Printf("schedule config=%d cron=%q: %v", cfg.ID, cfg.ScheduleCron, err)
		}
	}

	s.cron.Start()
	return nil
}

// Stop stops the cron scheduler.
func (s *Scheduler) Stop() {
	s.cron.Stop()
}

// RunBackup executes a backup for the given config ID using Pipeline.
func (s *Scheduler) RunBackup(ctx context.Context, cfgID int64) error {
	cfg, err := s.store.GetBackupConfig(cfgID)
	if err != nil {
		return fmt.Errorf("get backup config: %w", err)
	}

	run, err := s.store.CreateBackupRun(cfgID)
	if err != nil {
		return fmt.Errorf("create backup run: %w", err)
	}

	strategy, ok := s.strategies[cfg.Strategy]
	if !ok {
		errMsg := fmt.Sprintf("unknown strategy: %s", cfg.Strategy)
		s.store.UpdateBackupRunFailed(run.ID, errMsg)
		return fmt.Errorf("%s", errMsg)
	}

	factory, ok := s.targets[cfg.Target]
	if !ok {
		errMsg := fmt.Sprintf("unknown target: %s", cfg.Target)
		s.store.UpdateBackupRunFailed(run.ID, errMsg)
		return fmt.Errorf("%s", errMsg)
	}

	target, err := factory(cfg.TargetConfigJSON)
	if err != nil {
		errMsg := fmt.Sprintf("create target: %v", err)
		s.store.UpdateBackupRunFailed(run.ID, errMsg)
		return fmt.Errorf("%s", errMsg)
	}

	app, err := s.store.GetAppByID(cfg.AppID)
	if err != nil {
		errMsg := fmt.Sprintf("get app: %v", err)
		s.store.UpdateBackupRunFailed(run.ID, errMsg)
		return fmt.Errorf("%s", errMsg)
	}

	// Parse hooks from config JSON
	preHooks := parseHooks(cfg.PreHooks)
	postHooks := parseHooks(cfg.PostHooks)
	paths := parsePaths(cfg.Paths)

	// Build hook runner
	var hookRunner *HookRunner
	if s.hookExec != nil && (len(preHooks) > 0 || len(postHooks) > 0) {
		hookRunner = NewHookRunner(s.hookExec, 30*time.Second)
	}

	// Build pipeline
	pipe := NewPipeline(strategy, target, hookRunner)

	opts := BackupOpts{
		ContainerName: app.Name,
		Paths:         paths,
	}

	result, err := pipe.RunBackup(ctx, opts, preHooks, postHooks)
	if err != nil {
		errMsg := err.Error()
		s.store.UpdateBackupRunFailed(run.ID, errMsg)
		s.sendAlert(app.Name, cfg.Strategy, fmt.Sprintf("backup failed: %s", errMsg), "backup_failed")
		return fmt.Errorf("pipeline: %w", err)
	}

	if err := s.store.UpdateBackupRunSuccess(run.ID, result.SizeBytes, result.FilePath, result.Checksum); err != nil {
		return fmt.Errorf("update run success: %w", err)
	}

	// Prune old runs based on retention mode
	s.pruneOldRuns(ctx, cfg, target)

	s.sendAlert(app.Name, cfg.Strategy, "backup completed successfully", "backup_success")
	return nil
}

// RunRestore restores data from the backup run identified by runID.
func (s *Scheduler) RunRestore(ctx context.Context, runID int64) error {
	run, err := s.store.GetBackupRun(runID)
	if err != nil {
		return fmt.Errorf("get backup run: %w", err)
	}

	cfg, err := s.store.GetBackupConfig(run.BackupConfigID)
	if err != nil {
		return fmt.Errorf("get backup config: %w", err)
	}

	strategy, ok := s.strategies[cfg.Strategy]
	if !ok {
		return fmt.Errorf("unknown strategy: %s", cfg.Strategy)
	}

	factory, ok := s.targets[cfg.Target]
	if !ok {
		return fmt.Errorf("unknown target: %s", cfg.Target)
	}

	target, err := factory(cfg.TargetConfigJSON)
	if err != nil {
		return fmt.Errorf("create target: %w", err)
	}

	app, err := s.store.GetAppByID(cfg.AppID)
	if err != nil {
		return fmt.Errorf("get app: %w", err)
	}

	preHooks := parseHooks(cfg.PreHooks)
	postHooks := parseHooks(cfg.PostHooks)

	var hookRunner *HookRunner
	if s.hookExec != nil && (len(preHooks) > 0 || len(postHooks) > 0) {
		hookRunner = NewHookRunner(s.hookExec, 30*time.Second)
	}

	pipe := NewPipeline(strategy, target, hookRunner)

	opts := RestoreOpts{
		ContainerName: app.Name,
		Paths:         parsePaths(cfg.Paths),
	}

	return pipe.RunRestore(ctx, opts, run.FilePath, run.Checksum, preHooks, postHooks)
}

// CheckMissed iterates configs and alerts if last run is overdue (>2x cron interval).
func (s *Scheduler) CheckMissed() {
	configs, err := s.store.ListBackupConfigs(nil)
	if err != nil {
		log.Printf("check missed: list configs: %v", err)
		return
	}

	for _, cfg := range configs {
		if cfg.ScheduleCron == "" {
			continue
		}

		// Get latest run to find last run time
		runs, err := s.store.ListOldBackupRuns(cfg.ID, 0)
		if err != nil {
			continue
		}

		var lastRun *time.Time
		if len(runs) > 0 {
			lastRun = &runs[0].StartedAt
		}

		if isMissedBackup(cfg.ScheduleCron, lastRun) {
			app, err := s.store.GetAppByID(cfg.AppID)
			appName := fmt.Sprintf("app-%d", cfg.AppID)
			if err == nil {
				appName = app.Name
			}
			s.sendAlert(appName, cfg.Strategy, "missed scheduled backup (overdue >2x interval)", "backup_missed")
		}
	}
}

func (s *Scheduler) sendAlert(appName, strategy, message, eventType string) {
	if s.alertFunc != nil {
		s.alertFunc(appName, strategy, message, eventType)
	}
}

func (s *Scheduler) pruneOldRuns(ctx context.Context, cfg *store.BackupConfig, target Target) {
	var oldRuns []store.BackupRun
	var err error

	switch cfg.RetentionMode {
	case "count":
		if cfg.RetentionCount > 0 {
			oldRuns, err = s.store.ListOldBackupRuns(cfg.ID, cfg.RetentionCount)
		}
	case "time":
		if cfg.RetentionDays != nil && *cfg.RetentionDays > 0 {
			oldRuns, err = s.store.ListOldBackupRunsByTime(cfg.ID, *cfg.RetentionDays)
		}
	default:
		// fallback: use count if set
		if cfg.RetentionCount > 0 {
			oldRuns, err = s.store.ListOldBackupRuns(cfg.ID, cfg.RetentionCount)
		}
	}

	if err != nil {
		log.Printf("list old backup runs config=%d: %v", cfg.ID, err)
		return
	}

	for _, old := range oldRuns {
		if delErr := target.Delete(ctx, old.FilePath); delErr != nil {
			log.Printf("delete old backup file=%s: %v", old.FilePath, delErr)
		}
	}
}

// isMissedBackup returns true if time since lastRun exceeds 2x the cron interval.
func isMissedBackup(cronExpr string, lastRun *time.Time) bool {
	if lastRun == nil {
		return false // no previous run, nothing to compare
	}

	parser := cron.NewParser(cron.Minute | cron.Hour | cron.Dom | cron.Month | cron.Dow)
	sched, err := parser.Parse(cronExpr)
	if err != nil {
		return false
	}

	// Calculate interval from two consecutive firings
	ref := time.Now().Add(-24 * time.Hour) // reference point
	first := sched.Next(ref)
	second := sched.Next(first)
	interval := second.Sub(first)

	elapsed := time.Since(*lastRun)
	return elapsed > 2*interval
}

func parseHooks(jsonStr string) []Hook {
	jsonStr = strings.TrimSpace(jsonStr)
	if jsonStr == "" || jsonStr == "null" {
		return nil
	}
	var hooks []Hook
	if err := json.Unmarshal([]byte(jsonStr), &hooks); err != nil {
		log.Printf("parse hooks JSON: %v", err)
		return nil
	}
	return hooks
}

func parsePaths(jsonStr string) []string {
	jsonStr = strings.TrimSpace(jsonStr)
	if jsonStr == "" || jsonStr == "null" {
		return nil
	}
	var paths []string
	if err := json.Unmarshal([]byte(jsonStr), &paths); err != nil {
		// Try as comma-separated fallback
		return strings.Split(jsonStr, ",")
	}
	return paths
}
