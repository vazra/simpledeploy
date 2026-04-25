package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"
)

// AuditEntry represents a single row in the audit_log table.
type AuditEntry struct {
	ID               int64
	AppID            *int64
	AppSlug          string
	ActorUserID      *int64
	ActorName        string
	ActorSource      string
	IP               string
	Category         string
	Action           string
	Target           string
	Summary          string
	BeforeJSON       []byte
	AfterJSON        []byte
	Error            string
	ComposeVersionID *int64
	// SyncEligible is input-only: set it before calling RecordAudit to request
	// sync tracking (stored as sync_status='pending'). It is NOT a stored column
	// and is never populated on read; inspect SyncStatus directly instead.
	SyncEligible     bool
	SyncStatus       *string
	SyncCommitSHA    string
	SyncError        string
	CreatedAt        time.Time
}

// ActivityFilter controls which rows ListActivity returns.
type ActivityFilter struct {
	AppID         *int64
	AppSlug       string
	Categories    []string
	Before        int64
	Limit         int
	AllowedAppIDs []int64 // nil = unrestricted; empty slice = only system (app_id IS NULL) rows
	ActorUserID   *int64  // when set with AllowedAppIDs, restricts the app_id IS NULL branch to own auth events
}

// nullableInt returns nil for nil pointer, else the value as any for SQL binding.
func nullableInt(v *int64) any {
	if v == nil {
		return nil
	}
	return *v
}

// nullableString returns nil for empty string, else the string.
func nullableString(v string) any {
	if v == "" {
		return nil
	}
	return v
}

// nullableBytes returns nil for nil/empty slice, else the bytes.
func nullableBytes(v []byte) any {
	if len(v) == 0 {
		return nil
	}
	return v
}

// auditScanner is satisfied by both *sql.Row and *sql.Rows.
type auditScanner interface {
	Scan(dest ...any) error
}

// scanAudit scans a 19-column audit_log row.
// Caller must SELECT columns in exactly this order:
// id, app_id, app_slug, actor_user_id, actor_name, actor_source, ip,
// category, action, target, summary, before_json, after_json, error,
// compose_version_id, sync_status, sync_commit_sha, sync_error, created_at
func scanAudit(s auditScanner) (AuditEntry, error) {
	var e AuditEntry
	var (
		appID            sql.NullInt64
		appSlug          sql.NullString
		actorUserID      sql.NullInt64
		actorName        sql.NullString
		ip               sql.NullString
		target           sql.NullString
		beforeJSON       sql.NullString
		afterJSON        sql.NullString
		errStr           sql.NullString
		composeVersionID sql.NullInt64
		syncStatus       sql.NullString
		syncCommitSHA    sql.NullString
		syncError        sql.NullString
		createdAt        sql.NullString
	)
	if err := s.Scan(
		&e.ID,
		&appID,
		&appSlug,
		&actorUserID,
		&actorName,
		&e.ActorSource,
		&ip,
		&e.Category,
		&e.Action,
		&target,
		&e.Summary,
		&beforeJSON,
		&afterJSON,
		&errStr,
		&composeVersionID,
		&syncStatus,
		&syncCommitSHA,
		&syncError,
		&createdAt,
	); err != nil {
		return e, err
	}
	if appID.Valid {
		v := appID.Int64
		e.AppID = &v
	}
	if appSlug.Valid {
		e.AppSlug = appSlug.String
	}
	if actorUserID.Valid {
		v := actorUserID.Int64
		e.ActorUserID = &v
	}
	if actorName.Valid {
		e.ActorName = actorName.String
	}
	if ip.Valid {
		e.IP = ip.String
	}
	if target.Valid {
		e.Target = target.String
	}
	if beforeJSON.Valid {
		e.BeforeJSON = []byte(beforeJSON.String)
	}
	if afterJSON.Valid {
		e.AfterJSON = []byte(afterJSON.String)
	}
	if errStr.Valid {
		e.Error = errStr.String
	}
	if composeVersionID.Valid {
		v := composeVersionID.Int64
		e.ComposeVersionID = &v
	}
	if syncStatus.Valid {
		v := syncStatus.String
		e.SyncStatus = &v
	}
	if syncCommitSHA.Valid {
		e.SyncCommitSHA = syncCommitSHA.String
	}
	if syncError.Valid {
		e.SyncError = syncError.String
	}
	if createdAt.Valid {
		t, err := time.Parse("2006-01-02 15:04:05", createdAt.String)
		if err != nil {
			// try RFC3339 fallback
			t, err = time.Parse(time.RFC3339, createdAt.String)
			if err == nil {
				e.CreatedAt = t
			}
		} else {
			e.CreatedAt = t
		}
	}
	return e, nil
}

const auditInsertSQL = `
INSERT INTO audit_log
  (app_id, app_slug, actor_user_id, actor_name, actor_source, ip, category, action, target, summary, before_json, after_json, error, compose_version_id, sync_status)
VALUES
  (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
RETURNING id`

// RecordAudit inserts a new audit_log row and returns its id.
func (s *Store) RecordAudit(ctx context.Context, e AuditEntry) (int64, error) {
	return s.recordAuditTx(ctx, nil, e)
}

// recordAuditTx inserts into audit_log within an optional transaction.
// When tx is nil it uses s.db directly.
func (s *Store) recordAuditTx(ctx context.Context, tx *sql.Tx, e AuditEntry) (int64, error) {
	var syncStatus any
	if e.SyncEligible {
		syncStatus = "pending"
	}

	args := []any{
		nullableInt(e.AppID),
		nullableString(e.AppSlug),
		nullableInt(e.ActorUserID),
		nullableString(e.ActorName),
		e.ActorSource,
		nullableString(e.IP),
		e.Category,
		e.Action,
		nullableString(e.Target),
		e.Summary,
		nullableBytes(e.BeforeJSON),
		nullableBytes(e.AfterJSON),
		nullableString(e.Error),
		nullableInt(e.ComposeVersionID),
		syncStatus,
	}

	var row *sql.Row
	if tx != nil {
		row = tx.QueryRowContext(ctx, auditInsertSQL, args...)
	} else {
		row = s.db.QueryRowContext(ctx, auditInsertSQL, args...)
	}

	var id int64
	if err := row.Scan(&id); err != nil {
		return 0, fmt.Errorf("insert audit_log: %w", err)
	}
	return id, nil
}

const auditSelectCols = `
SELECT id, app_id, app_slug, actor_user_id, actor_name, actor_source, ip,
       category, action, target, summary, before_json, after_json, error,
       compose_version_id, sync_status, sync_commit_sha, sync_error, created_at`

// GetActivity returns a single audit_log row by id, including before/after JSON.
func (s *Store) GetActivity(ctx context.Context, id int64) (AuditEntry, error) {
	row := s.db.QueryRowContext(ctx, auditSelectCols+`
FROM audit_log WHERE id = ?`, id)
	e, err := scanAudit(row)
	if errors.Is(err, sql.ErrNoRows) {
		return e, fmt.Errorf("audit entry %d not found", id)
	}
	if err != nil {
		return e, fmt.Errorf("get audit entry: %w", err)
	}
	return e, nil
}

// ListActivity returns audit_log rows matching the filter, ordered by id DESC.
// before_json and after_json are always NULL in list results (use GetActivity for full row).
// Returns entries and a cursor (nextBefore) for the next page; 0 means no more pages.
func (s *Store) ListActivity(ctx context.Context, f ActivityFilter) (entries []AuditEntry, nextBefore int64, err error) {
	limit := f.Limit
	if limit <= 0 {
		limit = 50
	}
	if limit > 200 {
		limit = 200
	}

	var conds []string
	var args []any

	if f.Before > 0 {
		conds = append(conds, "id < ?")
		args = append(args, f.Before)
	}
	if f.AppID != nil {
		conds = append(conds, "app_id = ?")
		args = append(args, *f.AppID)
	}
	if f.AppSlug != "" {
		conds = append(conds, "app_slug = ?")
		args = append(args, f.AppSlug)
	}
	if len(f.Categories) > 0 {
		placeholders := strings.Repeat("?,", len(f.Categories))
		placeholders = placeholders[:len(placeholders)-1]
		conds = append(conds, "category IN ("+placeholders+")")
		for _, c := range f.Categories {
			args = append(args, c)
		}
	}
	if f.AllowedAppIDs != nil {
		if f.ActorUserID != nil {
			// Non-admin scope: only own auth events for app_id IS NULL rows.
			if len(f.AllowedAppIDs) == 0 {
				conds = append(conds, "(app_id IS NULL AND category = 'auth' AND actor_user_id = ?)")
				args = append(args, *f.ActorUserID)
			} else {
				placeholders := strings.Repeat("?,", len(f.AllowedAppIDs))
				placeholders = placeholders[:len(placeholders)-1]
				conds = append(conds, "(app_id IN ("+placeholders+") OR (app_id IS NULL AND category = 'auth' AND actor_user_id = ?))")
				for _, id := range f.AllowedAppIDs {
					args = append(args, id)
				}
				args = append(args, *f.ActorUserID)
			}
		} else {
			// Admin/legacy scope: app_id IS NULL rows unrestricted.
			if len(f.AllowedAppIDs) == 0 {
				conds = append(conds, "app_id IS NULL")
			} else {
				placeholders := strings.Repeat("?,", len(f.AllowedAppIDs))
				placeholders = placeholders[:len(placeholders)-1]
				conds = append(conds, "(app_id IS NULL OR app_id IN ("+placeholders+"))")
				for _, id := range f.AllowedAppIDs {
					args = append(args, id)
				}
			}
		}
	}

	where := ""
	if len(conds) > 0 {
		where = " WHERE " + strings.Join(conds, " AND ")
	}

	// SELECT NULL, NULL in place of before_json/after_json to allow scanAudit reuse.
	q := `SELECT id, app_id, app_slug, actor_user_id, actor_name, actor_source, ip,
       category, action, target, summary, NULL, NULL, error,
       compose_version_id, sync_status, sync_commit_sha, sync_error, created_at
FROM audit_log` + where + ` ORDER BY id DESC LIMIT ?`
	args = append(args, limit+1)

	rows, err := s.db.QueryContext(ctx, q, args...)
	if err != nil {
		return nil, 0, fmt.Errorf("list audit_log: %w", err)
	}
	defer rows.Close()

	for rows.Next() {
		e, err := scanAudit(rows)
		if err != nil {
			return nil, 0, fmt.Errorf("scan audit entry: %w", err)
		}
		entries = append(entries, e)
	}
	if err := rows.Err(); err != nil {
		return nil, 0, fmt.Errorf("iterate audit_log: %w", err)
	}

	if len(entries) > limit {
		nextBefore = entries[limit-1].ID
		entries = entries[:limit]
	}
	return entries, nextBefore, nil
}

// MarkSyncSynced sets sync_status='synced' and records the commit SHA for the given ids.
func (s *Store) MarkSyncSynced(ctx context.Context, ids []int64, commitSHA string) error {
	if len(ids) == 0 {
		return nil
	}
	placeholders := strings.Repeat("?,", len(ids))
	placeholders = placeholders[:len(placeholders)-1]
	args := []any{commitSHA}
	for _, id := range ids {
		args = append(args, id)
	}
	_, err := s.db.ExecContext(ctx,
		`UPDATE audit_log SET sync_status='synced', sync_commit_sha=?, sync_error=NULL WHERE id IN (`+placeholders+`)`,
		args...,
	)
	if err != nil {
		return fmt.Errorf("mark sync synced: %w", err)
	}
	return nil
}

// MarkSyncFailed sets sync_status='failed' and records the error message for the given ids.
func (s *Store) MarkSyncFailed(ctx context.Context, ids []int64, errMsg string) error {
	if len(ids) == 0 {
		return nil
	}
	placeholders := strings.Repeat("?,", len(ids))
	placeholders = placeholders[:len(placeholders)-1]
	args := []any{errMsg}
	for _, id := range ids {
		args = append(args, id)
	}
	_, err := s.db.ExecContext(ctx,
		`UPDATE audit_log SET sync_status='failed', sync_error=? WHERE id IN (`+placeholders+`)`,
		args...,
	)
	if err != nil {
		return fmt.Errorf("mark sync failed: %w", err)
	}
	return nil
}

// PendingSyncAuditIDs returns ids of audit_log rows with sync_status='pending', ordered by id.
func (s *Store) PendingSyncAuditIDs(ctx context.Context) ([]int64, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT id FROM audit_log WHERE sync_status='pending' ORDER BY id`)
	if err != nil {
		return nil, fmt.Errorf("query pending sync ids: %w", err)
	}
	defer rows.Close()

	var ids []int64
	for rows.Next() {
		var id int64
		if err := rows.Scan(&id); err != nil {
			return nil, fmt.Errorf("scan pending sync id: %w", err)
		}
		ids = append(ids, id)
	}
	return ids, rows.Err()
}

// PruneAudit deletes audit_log rows with created_at before olderThan.
// Returns the number of rows deleted.
func (s *Store) PruneAudit(ctx context.Context, olderThan time.Time) (int64, error) {
	res, err := s.db.ExecContext(ctx,
		`DELETE FROM audit_log WHERE created_at < ?`,
		olderThan.UTC().Format(time.RFC3339),
	)
	if err != nil {
		return 0, fmt.Errorf("prune audit_log: %w", err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return 0, fmt.Errorf("prune audit_log rows affected: %w", err)
	}
	return n, nil
}

// PurgeAudit deletes all rows from audit_log.
func (s *Store) PurgeAudit(ctx context.Context) error {
	_, err := s.db.ExecContext(ctx, `DELETE FROM audit_log`)
	if err != nil {
		return fmt.Errorf("purge audit_log: %w", err)
	}
	return nil
}
