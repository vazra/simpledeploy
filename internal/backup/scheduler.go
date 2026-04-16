package backup

import (
	"context"
	"fmt"
	"log"

	"github.com/robfig/cron/v3"
	"github.com/vazra/simpledeploy/internal/store"
)

// BackupStore is the subset of store.Store needed by Scheduler.
type BackupStore interface {
	ListBackupConfigs(appID *int64) ([]store.BackupConfig, error)
	GetBackupConfig(id int64) (*store.BackupConfig, error)
	CreateBackupRun(configID int64) (*store.BackupRun, error)
	UpdateBackupRunSuccess(id int64, sizeBytes int64, filePath string) error
	UpdateBackupRunFailed(id int64, errMsg string) error
	ListOldBackupRuns(configID int64, keepCount int) ([]store.BackupRun, error)
	GetAppByID(id int64) (*store.App, error)
	GetBackupRun(id int64) (*store.BackupRun, error)
}

// Scheduler runs scheduled backups and handles restore requests.
type Scheduler struct {
	store      BackupStore
	strategies map[string]Strategy
	targets    map[string]func(configJSON string) (Target, error)
	cron       *cron.Cron
}

// NewScheduler creates a new Scheduler backed by st.
func NewScheduler(st BackupStore) *Scheduler {
	return &Scheduler{
		store:      st,
		strategies: make(map[string]Strategy),
		targets:    make(map[string]func(string) (Target, error)),
		cron:       cron.New(),
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

// Start loads all backup configs and schedules cron entries, then starts the cron runner.
func (s *Scheduler) Start() error {
	configs, err := s.store.ListBackupConfigs(nil)
	if err != nil {
		return fmt.Errorf("list backup configs: %w", err)
	}

	for _, cfg := range configs {
		cfgID := cfg.ID
		_, err := s.cron.AddFunc(cfg.ScheduleCron, func() {
			ctx := context.Background()
			if err := s.RunBackup(ctx, cfgID); err != nil {
				log.Printf("scheduled backup config=%d: %v", cfgID, err)
			}
		})
		if err != nil {
			log.Printf("add cron for config=%d schedule=%q: %v", cfg.ID, cfg.ScheduleCron, err)
		}
	}

	s.cron.Start()
	return nil
}

// Stop stops the cron scheduler.
func (s *Scheduler) Stop() {
	s.cron.Stop()
}

// RunBackup executes a backup for the given config ID.
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

	result, err := strategy.Backup(ctx, BackupOpts{ContainerName: app.Name})
	if err != nil {
		errMsg := fmt.Sprintf("backup: %v", err)
		s.store.UpdateBackupRunFailed(run.ID, errMsg)
		return fmt.Errorf("%s", errMsg)
	}
	defer result.Reader.Close()

	path, size, err := target.Upload(ctx, result.Filename, result.Reader)
	if err != nil {
		errMsg := fmt.Sprintf("upload: %v", err)
		s.store.UpdateBackupRunFailed(run.ID, errMsg)
		return fmt.Errorf("%s", errMsg)
	}

	if err := s.store.UpdateBackupRunSuccess(run.ID, size, path); err != nil {
		return fmt.Errorf("update run success: %w", err)
	}

	// prune old runs beyond retention
	if cfg.RetentionCount > 0 {
		oldRuns, err := s.store.ListOldBackupRuns(cfgID, cfg.RetentionCount)
		if err != nil {
			log.Printf("list old backup runs config=%d: %v", cfgID, err)
		} else {
			for _, old := range oldRuns {
				if delErr := target.Delete(ctx, old.FilePath); delErr != nil {
					log.Printf("delete old backup file=%s: %v", old.FilePath, delErr)
				}
			}
		}
	}

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

	data, err := target.Download(ctx, run.FilePath)
	if err != nil {
		return fmt.Errorf("download: %w", err)
	}
	defer data.Close()

	if err := strategy.Restore(ctx, RestoreOpts{ContainerName: app.Name, Reader: data}); err != nil {
		return fmt.Errorf("restore: %w", err)
	}

	return nil
}
