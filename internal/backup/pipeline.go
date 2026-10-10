package backup

import (
	"context"
	"fmt"
)

// PipelineResult holds the outcome of a backup pipeline run.
type PipelineResult struct {
	FilePath  string
	SizeBytes int64
	Checksum  string
}

// Pipeline orchestrates backup and restore flows through strategy, target, and hooks.
type Pipeline struct {
	strategy Strategy
	target   Target
	hooks    *HookRunner
}

// NewPipeline creates a Pipeline with the given strategy, target, and optional hook runner.
func NewPipeline(strategy Strategy, target Target, hooks *HookRunner) *Pipeline {
	return &Pipeline{strategy: strategy, target: target, hooks: hooks}
}

// RunBackup: pre-hooks -> strategy.Backup -> tee through checksum -> target.Upload -> post-hooks
func (p *Pipeline) RunBackup(ctx context.Context, opts BackupOpts, preHooks, postHooks []Hook) (*PipelineResult, error) {
	// 1. Run pre-hooks (abort on failure)
	if p.hooks != nil && len(preHooks) > 0 {
		if err := p.hooks.RunPre(ctx, preHooks); err != nil {
			return nil, fmt.Errorf("pre-hook: %w", err)
		}
	}

	// 2. Run backup strategy
	backupResult, err := p.strategy.Backup(ctx, opts)
	if err != nil {
		p.runPostHooks(ctx, postHooks)
		return nil, fmt.Errorf("backup: %w", err)
	}
	defer backupResult.Reader.Close()

	// 3. Tee through SHA-256 checksum
	cw := NewChecksumWriter()
	teedReader := cw.TeeReader(backupResult.Reader)

	// 4. Upload to target
	filePath, size, err := p.target.Upload(ctx, backupResult.Filename, teedReader)
	if err != nil {
		p.runPostHooks(ctx, postHooks)
		return nil, fmt.Errorf("upload: %w", err)
	}

	// 5. Post-hooks (best effort)
	p.runPostHooks(ctx, postHooks)

	return &PipelineResult{FilePath: filePath, SizeBytes: size, Checksum: cw.Sum()}, nil
}

// RunRestore: download -> verify checksum -> pre-hooks -> strategy.Restore -> post-hooks
func (p *Pipeline) RunRestore(ctx context.Context, opts RestoreOpts, filePath, expectedChecksum string, preHooks, postHooks []Hook) error {
	reader, err := p.target.Download(ctx, filePath)
	if err != nil {
		return fmt.Errorf("download: %w", err)
	}
	defer reader.Close()

	if expectedChecksum != "" {
		// Stage the download on disk while hashing so a large backup is
		// never held in memory, then restore from the verified file.
		// Strategies that validate archives check this file in place.
		spool, err := newSpoolFile()
		if err != nil {
			return err
		}
		defer spool.Close()
		limit, capped := restoreCap(opts.MaxDecompressedBytes)
		cw := NewChecksumWriter()
		if err := spool.fill(cw.TeeReader(reader), limit, capped); err != nil {
			return fmt.Errorf("download: %w", err)
		}
		if actual := cw.Sum(); actual != expectedChecksum {
			return fmt.Errorf("checksum mismatch: expected %s, got %s", expectedChecksum, actual)
		}
		reader.Close()
		opts.Reader = spool
	} else {
		opts.Reader = reader
	}

	if p.hooks != nil && len(preHooks) > 0 {
		if err := p.hooks.RunPre(ctx, preHooks); err != nil {
			return fmt.Errorf("pre-hook: %w", err)
		}
	}

	if err := p.strategy.Restore(ctx, opts); err != nil {
		p.runPostHooks(ctx, postHooks)
		return fmt.Errorf("restore: %w", err)
	}

	p.runPostHooks(ctx, postHooks)
	return nil
}

func (p *Pipeline) runPostHooks(ctx context.Context, hooks []Hook) {
	if p.hooks != nil && len(hooks) > 0 {
		p.hooks.RunPost(ctx, hooks)
	}
}
