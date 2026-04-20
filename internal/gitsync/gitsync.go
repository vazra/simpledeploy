// Package gitsync treats {apps_dir} as a git working tree, committing local
// changes and pulling remote changes on a poll interval or webhook trigger.
//
// Only the following paths are tracked (via .gitignore whitelist):
//
//	**/docker-compose.yml
//	**/.env
//	**/simpledeploy.yml
//	_global.yml
//
// Conflict resolution: server (local) always wins. During a rebase, go-git's
// conflict resolution is limited, so we fall back to shelling out to git for
// the rebase+checkout-ours step. See conflict.go for details.
package gitsync

import (
	"context"
	"errors"
	"fmt"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/config"
	"github.com/go-git/go-git/v5/plumbing"
	"github.com/go-git/go-git/v5/plumbing/object"
	githttp "github.com/go-git/go-git/v5/plumbing/transport/http"
	"github.com/go-git/go-git/v5/plumbing/transport/ssh"

	"github.com/vazra/simpledeploy/internal/configsync"
	"github.com/vazra/simpledeploy/internal/store"
)

// gitignoreContent is written once on repo init. It whitelists only the files
// that simpledeploy manages; everything else is ignored.
const gitignoreContent = `# simpledeploy: config repo
# Only the following paths are tracked:
#   **/docker-compose.yml
#   **/.env
#   **/simpledeploy.yml
#   _global.yml
# Everything else is ignored.
*
!*/
!*/docker-compose.yml
!*/.env
!*/simpledeploy.yml
!_global.yml
!.gitignore
`

// Config controls the sync worker.
type Config struct {
	Enabled       bool
	Remote        string        // "git@github.com:owner/repo.git" or https URL
	Branch        string        // default "main"
	AppsDir       string        // working tree root
	AuthorName    string        // default "SimpleDeploy"
	AuthorEmail   string        // default "bot@simpledeploy.local"
	SSHKeyPath    string        // path to private key for git@ / ssh:// remotes
	HTTPSUsername string        // optional; defaults to "git" for github tokens
	HTTPSToken    string        // for https remotes
	PollInterval  time.Duration // default 60s. 0 disables polling.
	WebhookSecret string        // HMAC secret. Empty disables webhook endpoint.
}

func (c *Config) branch() string {
	if c.Branch == "" {
		return "main"
	}
	return c.Branch
}

func (c *Config) authorName() string {
	if c.AuthorName == "" {
		return "SimpleDeploy"
	}
	return c.AuthorName
}

func (c *Config) authorEmail() string {
	if c.AuthorEmail == "" {
		return "bot@simpledeploy.local"
	}
	return c.AuthorEmail
}

func (c *Config) pollInterval() time.Duration {
	if c.PollInterval == 0 {
		return 60 * time.Second
	}
	return c.PollInterval
}

// Reconciler is the callback invoked after a pull applies changes.
type Reconciler interface {
	ReconcileAfterSync(ctx context.Context, changedPaths []string) error
}

// ReconcilerFunc adapts a plain function to the Reconciler interface.
type ReconcilerFunc func(ctx context.Context, paths []string) error

func (f ReconcilerFunc) ReconcileAfterSync(ctx context.Context, paths []string) error {
	return f(ctx, paths)
}

// commitReq is an internal work item for the worker.
type commitReq struct {
	paths  []string
	reason string
}

// syncReq requests an immediate fetch+pull. The result is sent back on done.
type syncReq struct {
	ctx  context.Context
	done chan<- error
}

// Status is a snapshot of the Syncer state for the UI / status endpoint.
type Status struct {
	Enabled         bool
	Remote          string
	Branch          string
	HeadSHA         string
	LastSyncAt      time.Time
	LastSyncError   string
	PendingCommits  int
	DroppedRequests int64
	RecentConflicts []Conflict
}

// Conflict records a single server-wins conflict resolution.
type Conflict struct {
	Path        string
	RemoteSHA   string
	ResolvedAt  time.Time
	Description string
}

// Syncer coordinates git operations.
type Syncer struct {
	cfg Config
	st  *store.Store
	cs  *configsync.Syncer
	rec Reconciler

	repo *git.Repository // set after Start

	commitCh chan commitReq // buffered
	syncCh   chan syncReq   // buffered

	suppress atomic.Bool // true while import-from-pull is running

	mu              sync.Mutex
	headSHA         string
	lastSyncAt      time.Time
	lastSyncError   string
	recentConflicts []Conflict
	dropped         int64

	wg     sync.WaitGroup
	cancel context.CancelFunc
}

const (
	commitChanSize = 32
	syncChanSize   = 8
	maxConflicts   = 20

	// suppressTail is how long to keep suppress=true after an import completes.
	// The configsync debouncer fires up to 500ms after the last ScheduleAppWrite
	// call; keeping suppress active for 2x that window ensures the debounced
	// WriteAppSidecar -> callHook -> EnqueueCommit path is also suppressed,
	// preventing a spurious bot commit after every remote pull.
	suppressTail = 1200 * time.Millisecond
)

// New validates config and constructs a Syncer. Does not touch disk or network.
func New(cfg Config, st *store.Store, cs *configsync.Syncer, rec Reconciler) (*Syncer, error) {
	if !cfg.Enabled {
		return &Syncer{cfg: cfg}, nil
	}
	if cfg.AppsDir == "" {
		return nil, errors.New("gitsync: AppsDir required")
	}
	if cfg.Remote == "" {
		return nil, errors.New("gitsync: Remote required")
	}
	return &Syncer{
		cfg:      cfg,
		st:       st,
		cs:       cs,
		rec:      rec,
		commitCh: make(chan commitReq, commitChanSize),
		syncCh:   make(chan syncReq, syncChanSize),
	}, nil
}

// Start initializes the repo if needed and starts worker + poll loops.
func (g *Syncer) Start(ctx context.Context) error {
	if !g.cfg.Enabled {
		return nil
	}

	if err := g.initRepo(); err != nil {
		g.setError(err.Error())
		return err
	}

	wctx, cancel := context.WithCancel(ctx)
	g.cancel = cancel

	g.wg.Add(1)
	go g.worker(wctx)

	if g.cfg.pollInterval() > 0 {
		g.wg.Add(1)
		go g.pollLoop(wctx)
	}

	return nil
}

// Stop flushes pending commits with a short deadline and joins goroutines.
func (g *Syncer) Stop() error {
	if !g.cfg.Enabled || g.cancel == nil {
		return nil
	}

	// Signal the worker to stop accepting new work.
	g.cancel()
	g.wg.Wait()
	return nil
}

// EnqueueCommit marks the working tree dirty and requests a commit-and-push.
// Non-blocking; drops if the channel is full. Coalesces naturally via buffered channel.
func (g *Syncer) EnqueueCommit(paths []string, reason string) {
	if !g.cfg.Enabled {
		return
	}
	if g.suppress.Load() {
		return
	}
	select {
	case g.commitCh <- commitReq{paths: paths, reason: reason}:
	default:
		atomic.AddInt64(&g.dropped, 1)
		log.Printf("[gitsync] commit channel full, dropping request: %s", reason)
	}
}

// SyncNow triggers an immediate fetch+pull+apply.
func (g *Syncer) SyncNow(ctx context.Context) error {
	if !g.cfg.Enabled {
		return nil
	}
	done := make(chan error, 1)
	select {
	case g.syncCh <- syncReq{ctx: ctx, done: done}:
	default:
		atomic.AddInt64(&g.dropped, 1)
		return nil
	}
	select {
	case err := <-done:
		return err
	case <-ctx.Done():
		return ctx.Err()
	}
}

// Status returns a snapshot.
func (g *Syncer) Status() Status {
	if !g.cfg.Enabled {
		return Status{Enabled: false, Remote: g.cfg.Remote, Branch: g.cfg.branch()}
	}
	g.mu.Lock()
	defer g.mu.Unlock()
	conflicts := make([]Conflict, len(g.recentConflicts))
	copy(conflicts, g.recentConflicts)
	return Status{
		Enabled:         true,
		Remote:          g.cfg.Remote,
		Branch:          g.cfg.branch(),
		HeadSHA:         g.headSHA,
		LastSyncAt:      g.lastSyncAt,
		LastSyncError:   g.lastSyncError,
		PendingCommits:  len(g.commitCh),
		DroppedRequests: atomic.LoadInt64(&g.dropped),
		RecentConflicts: conflicts,
	}
}

// WebhookHandler returns an http.Handler or nil if disabled.
func (g *Syncer) WebhookHandler() http.Handler {
	if !g.cfg.Enabled || g.cfg.WebhookSecret == "" {
		return nil
	}
	return newWebhookHandler(g)
}

// ---- internal ----

func (g *Syncer) worker(ctx context.Context) {
	defer g.wg.Done()
	// On shutdown, drain commit channel with a short deadline.
	defer func() {
		deadline := time.Now().Add(5 * time.Second)
		for {
			select {
			case req := <-g.commitCh:
				if time.Now().After(deadline) {
					return
				}
				g.doCommit(context.Background(), req)
			default:
				return
			}
		}
	}()

	for {
		select {
		case <-ctx.Done():
			return
		case req := <-g.commitCh:
			g.doCommit(ctx, req)
		case req := <-g.syncCh:
			err := g.doPull(req.ctx)
			req.done <- err
		}
	}
}

func (g *Syncer) pollLoop(ctx context.Context) {
	defer g.wg.Done()
	ticker := time.NewTicker(g.cfg.pollInterval())
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			done := make(chan error, 1)
			select {
			case g.syncCh <- syncReq{ctx: ctx, done: done}:
				<-done // wait so we don't pile up
			default:
				// worker busy; skip this tick
			}
		}
	}
}

// initRepo ensures the appsDir is a git repo with the correct remote.
func (g *Syncer) initRepo() error {
	gitDir := filepath.Join(g.cfg.AppsDir, ".git")
	if _, err := os.Stat(gitDir); os.IsNotExist(err) {
		return g.initFresh()
	}
	// Existing repo: validate remote.
	repo, err := git.PlainOpen(g.cfg.AppsDir)
	if err != nil {
		return fmt.Errorf("gitsync: open existing repo: %w", err)
	}
	remotes, err := repo.Remotes()
	if err != nil {
		return fmt.Errorf("gitsync: list remotes: %w", err)
	}
	for _, r := range remotes {
		if r.Config().Name == "origin" {
			urls := r.Config().URLs
			if len(urls) > 0 && urls[0] != g.cfg.Remote {
				return fmt.Errorf(
					"gitsync: existing repo remote URL %q does not match cfg.Remote %q; "+
						"update the config or remove .git to re-initialize",
					urls[0], g.cfg.Remote,
				)
			}
		}
	}
	g.repo = repo
	g.updateHeadSHA()
	return nil
}

func (g *Syncer) initFresh() error {
	if err := os.MkdirAll(g.cfg.AppsDir, 0700); err != nil {
		return fmt.Errorf("gitsync: mkdir appsDir: %w", err)
	}
	// Use `git init -b <branch>` via shell to ensure the default branch name
	// matches cfg.Branch. go-git's PlainInit always creates "master".
	if out, err := gitExec(g.cfg.AppsDir, "init", "-b", g.cfg.branch()); err != nil {
		return fmt.Errorf("gitsync: git init: %w\n%s", err, out)
	}
	repo, err := git.PlainOpen(g.cfg.AppsDir)
	if err != nil {
		return fmt.Errorf("gitsync: open after init: %w", err)
	}

	// Write .gitignore.
	if err := os.WriteFile(filepath.Join(g.cfg.AppsDir, ".gitignore"), []byte(gitignoreContent), 0600); err != nil {
		return fmt.Errorf("gitsync: write .gitignore: %w", err)
	}

	// Set repo-level user config so commits don't fail on machines without global git config.
	repoCfg, err := repo.Config()
	if err != nil {
		return fmt.Errorf("gitsync: repo config: %w", err)
	}
	repoCfg.Author.Name = g.cfg.authorName()
	repoCfg.Author.Email = g.cfg.authorEmail()
	if err := repo.SetConfig(repoCfg); err != nil {
		return fmt.Errorf("gitsync: set repo config: %w", err)
	}

	wt, err := repo.Worktree()
	if err != nil {
		return fmt.Errorf("gitsync: worktree: %w", err)
	}

	// Stage allowed files.
	if err := g.stageAllowed(wt); err != nil {
		return fmt.Errorf("gitsync: stage: %w", err)
	}

	// Initial commit.
	now := time.Now()
	sig := &object.Signature{Name: g.cfg.authorName(), Email: g.cfg.authorEmail(), When: now}
	_, err = wt.Commit("chore(simpledeploy): initial sync commit", &git.CommitOptions{
		Author:    sig,
		Committer: sig,
	})
	if err != nil {
		return fmt.Errorf("gitsync: initial commit: %w", err)
	}
	g.repo = repo
	g.updateHeadSHA()

	// Ensure git user config is set for shell fallback operations (push --continue, etc.).
	_, _ = gitExec(g.cfg.AppsDir, "config", "user.name", g.cfg.authorName())
	_, _ = gitExec(g.cfg.AppsDir, "config", "user.email", g.cfg.authorEmail())

	// Add remote.
	if g.cfg.Remote != "" {
		_, err = repo.CreateRemote(&config.RemoteConfig{
			Name: "origin",
			URLs: []string{g.cfg.Remote},
		})
		if err != nil {
			return fmt.Errorf("gitsync: create remote: %w", err)
		}

		// Check if remote has commits on the branch; refuse to push if so.
		refs, lsErr := repo.References()
		_ = refs
		remoteHasCommits, lsErr := g.remoteHasBranch()
		if lsErr != nil {
			// Can't reach remote - that's ok; leave for operator to handle.
			log.Printf("[gitsync] warning: could not ls-remote: %v", lsErr)
		} else if remoteHasCommits {
			msg := fmt.Sprintf(
				"gitsync: remote %q already has commits on branch %q; "+
					"run with --adopt-local to force-push local state, "+
					"or --adopt-remote to clone-and-import remote state",
				g.cfg.Remote, g.cfg.branch(),
			)
			g.setError(msg)
			return errors.New(msg)
		}

		// Push initial commit via shell fallback for reliability (go-git push to
		// a brand-new bare repo via file:// can struggle with ref tracking).
		if out, pushErr := gitExec(g.cfg.AppsDir, "push", "-u", "origin", g.cfg.branch()); pushErr != nil {
			log.Printf("[gitsync] warning: initial push failed: %v\n%s", pushErr, out)
		}
	}

	return nil
}

func (g *Syncer) remoteHasBranch() (bool, error) {
	auth, err := g.buildAuth()
	if err != nil {
		return false, err
	}
	rem, err := g.repo.Remote("origin")
	if err != nil {
		return false, err
	}
	refs, err := rem.List(&git.ListOptions{Auth: auth})
	if err != nil {
		return false, err
	}
	branchRef := plumbing.NewRemoteReferenceName("origin", g.cfg.branch())
	targetRef := "refs/heads/" + g.cfg.branch()
	for _, ref := range refs {
		if ref.Name() == branchRef || ref.Name().String() == targetRef {
			return true, nil
		}
	}
	return false, nil
}

// stageAllowed stages all tracked files (docker-compose.yml, .env, simpledeploy.yml, _global.yml, .gitignore).
func (g *Syncer) stageAllowed(wt *git.Worktree) error {
	// Walk the appsDir and add whitelisted files.
	return filepath.Walk(g.cfg.AppsDir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return nil
		}
		if info.IsDir() {
			base := filepath.Base(path)
			// Skip .git and hidden dirs except root.
			if base == ".git" {
				return filepath.SkipDir
			}
			return nil
		}
		rel, _ := filepath.Rel(g.cfg.AppsDir, path)
		if isAllowedPath(rel) {
			_, addErr := wt.Add(rel)
			return addErr
		}
		return nil
	})
}

func isAllowedPath(rel string) bool {
	base := filepath.Base(rel)
	depth := len(strings.Split(rel, string(os.PathSeparator)))
	switch base {
	case "docker-compose.yml", ".env", "simpledeploy.yml":
		return depth == 2 // exactly <slug>/filename
	case "_global.yml", ".gitignore":
		return depth == 1
	}
	return false
}

// doCommit stages allowed paths and commits if there are changes, then pushes.
func (g *Syncer) doCommit(ctx context.Context, req commitReq) {
	if g.repo == nil {
		return
	}
	wt, err := g.repo.Worktree()
	if err != nil {
		log.Printf("[gitsync] worktree: %v", err)
		return
	}

	if len(req.paths) == 0 {
		if err := g.stageAllowed(wt); err != nil {
			log.Printf("[gitsync] stage all: %v", err)
			return
		}
	} else {
		for _, p := range req.paths {
			rel, err := filepath.Rel(g.cfg.AppsDir, p)
			if err != nil {
				rel = p
			}
			if isAllowedPath(rel) {
				if _, err := wt.Add(rel); err != nil {
					log.Printf("[gitsync] add %s: %v", rel, err)
				}
			}
		}
	}

	st, err := wt.Status()
	if err != nil {
		log.Printf("[gitsync] status: %v", err)
		return
	}
	if st.IsClean() {
		return
	}

	msg := buildCommitMessage("chore(simpledeploy): sync config", req.reason)
	now := time.Now()
	sig := &object.Signature{Name: g.cfg.authorName(), Email: g.cfg.authorEmail(), When: now}
	_, err = wt.Commit(msg, &git.CommitOptions{
		Author:    sig,
		Committer: sig,
	})
	if err != nil {
		log.Printf("[gitsync] commit: %v", err)
		return
	}
	g.updateHeadSHA()

	if err := g.doPushWithRetry(); err != nil {
		log.Printf("[gitsync] push: %v", err)
		g.setError(err.Error())
	} else {
		g.clearError()
	}
}

// doPull fetches, rebases with server-wins conflict resolution, then reconciles.
func (g *Syncer) doPull(ctx context.Context) error {
	if g.repo == nil {
		return errors.New("gitsync: repo not initialized")
	}

	prevSHA := g.headSHA

	// Fetch.
	auth, err := g.buildAuth()
	if err != nil {
		g.setError(err.Error())
		return err
	}
	fetchErr := g.repo.Fetch(&git.FetchOptions{
		RemoteName: "origin",
		Auth:       auth,
		Force:      false,
	})
	if fetchErr != nil && fetchErr != git.NoErrAlreadyUpToDate {
		g.setError(fetchErr.Error())
		return fmt.Errorf("gitsync: fetch: %w", fetchErr)
	}
	if fetchErr == git.NoErrAlreadyUpToDate {
		g.setLastSync(nil)
		return nil
	}

	// Rebase via shell fallback (go-git rebase with conflict resolution is limited).
	conflicts, newSHA, pullErr := rebaseServerWins(g.cfg.AppsDir, g.cfg.branch())
	if pullErr != nil {
		g.setError(pullErr.Error())
		return pullErr
	}

	// Re-open repo to pick up new HEAD.
	repo, err := git.PlainOpen(g.cfg.AppsDir)
	if err != nil {
		return fmt.Errorf("gitsync: re-open after rebase: %w", err)
	}
	g.repo = repo
	g.mu.Lock()
	g.headSHA = newSHA
	g.mu.Unlock()

	// Record conflicts.
	for _, c := range conflicts {
		c.ResolvedAt = time.Now()
		g.recordConflict(c)
		if g.st != nil {
			_ = g.st.InsertConflictAlert(c.Path, c.RemoteSHA, c.Description)
		}
	}

	g.setLastSync(nil)

	if newSHA == prevSHA {
		return nil
	}

	// Compute changed paths between prevSHA and newSHA.
	changedPaths, err := g.diffPaths(prevSHA, newSHA)
	if err != nil {
		log.Printf("[gitsync] diff paths: %v", err)
	}

	// Import sidecar changes; suppress new commits during this window.
	// Keep suppress active for suppressTail after import so that the
	// configsync debouncer (500ms) cannot fire a WriteAppSidecar ->
	// callHook -> EnqueueCommit sequence that would produce a spurious
	// bot commit rebounding the pulled change.
	g.suppress.Store(true)
	defer func() {
		time.AfterFunc(suppressTail, func() { g.suppress.Store(false) })
	}()

	if g.cs == nil {
		goto afterImport
	}
	for _, p := range changedPaths {
		switch {
		case strings.HasSuffix(p, "/simpledeploy.yml"):
			slug := strings.TrimSuffix(strings.TrimPrefix(p, "/"), "/simpledeploy.yml")
			if idx := strings.LastIndex(slug, "/"); idx >= 0 {
				slug = slug[idx+1:]
			}
			sidecar, readErr := g.cs.ReadAppSidecar(slug)
			if readErr == nil && sidecar != nil {
				if importErr := g.cs.ImportAppSidecar(sidecar); importErr != nil {
					log.Printf("[gitsync] import app sidecar %s: %v", slug, importErr)
				}
			}
		case p == "_global.yml":
			global, readErr := g.cs.ReadRedactedGlobal()
			if readErr == nil && global != nil {
				if importErr := g.cs.ImportRedactedGlobal(global); importErr != nil {
					log.Printf("[gitsync] import redacted global: %v", importErr)
				}
			}
		}
	}

afterImport:
	if g.rec != nil {
		if recErr := g.rec.ReconcileAfterSync(ctx, changedPaths); recErr != nil {
			log.Printf("[gitsync] reconcile after sync: %v", recErr)
		}
	}

	return nil
}

func (g *Syncer) diffPaths(fromSHA, toSHA string) ([]string, error) {
	if fromSHA == "" || fromSHA == toSHA {
		return nil, nil
	}

	fromHash := plumbing.NewHash(fromSHA)
	toHash := plumbing.NewHash(toSHA)

	fromCommit, err := g.repo.CommitObject(fromHash)
	if err != nil {
		return nil, fmt.Errorf("commit %s: %w", fromSHA, err)
	}
	toCommit, err := g.repo.CommitObject(toHash)
	if err != nil {
		return nil, fmt.Errorf("commit %s: %w", toSHA, err)
	}

	fromTree, err := fromCommit.Tree()
	if err != nil {
		return nil, err
	}
	toTree, err := toCommit.Tree()
	if err != nil {
		return nil, err
	}

	changes, err := fromTree.Diff(toTree)
	if err != nil {
		return nil, err
	}

	var paths []string
	for _, c := range changes {
		if c.To.Name != "" {
			paths = append(paths, c.To.Name)
		} else if c.From.Name != "" {
			paths = append(paths, c.From.Name)
		}
	}
	return paths, nil
}

func (g *Syncer) doPush() error {
	auth, err := g.buildAuth()
	if err != nil {
		return err
	}
	pushErr := g.repo.Push(&git.PushOptions{
		RemoteName: "origin",
		Auth:       auth,
		RefSpecs: []config.RefSpec{
			config.RefSpec(fmt.Sprintf("refs/heads/%s:refs/heads/%s", g.cfg.branch(), g.cfg.branch())),
		},
	})
	if pushErr == git.NoErrAlreadyUpToDate {
		return nil
	}
	return pushErr
}

func (g *Syncer) doPushWithRetry() error {
	err := g.doPush()
	if err == nil {
		return nil
	}
	// Non-fast-forward: fetch + retry once.
	auth, _ := g.buildAuth()
	_ = g.repo.Fetch(&git.FetchOptions{RemoteName: "origin", Auth: auth, Force: true})
	return g.doPush()
}

func (g *Syncer) buildAuth() (interface {
	// go-git auth is an interface; returning any is simpler.
	String() string
	Name() string
}, error) {
	remote := g.cfg.Remote
	if strings.HasPrefix(remote, "git@") || strings.HasPrefix(remote, "ssh://") {
		if g.cfg.SSHKeyPath == "" {
			return nil, errors.New("gitsync: SSHKeyPath required for SSH remote")
		}
		pubkeys, err := ssh.NewPublicKeysFromFile("git", g.cfg.SSHKeyPath, "")
		if err != nil {
			return nil, fmt.Errorf("gitsync: load SSH key: %w", err)
		}
		return pubkeys, nil
	}
	// HTTPS (or file://)
	if g.cfg.HTTPSToken != "" {
		username := g.cfg.HTTPSUsername
		if username == "" {
			username = "git"
		}
		return &githttp.BasicAuth{Username: username, Password: g.cfg.HTTPSToken}, nil
	}
	return &githttp.BasicAuth{}, nil
}

func (g *Syncer) updateHeadSHA() {
	if g.repo == nil {
		return
	}
	ref, err := g.repo.Head()
	if err != nil {
		return
	}
	g.mu.Lock()
	g.headSHA = ref.Hash().String()
	g.mu.Unlock()
}

func (g *Syncer) setError(msg string) {
	g.mu.Lock()
	g.lastSyncError = msg
	g.mu.Unlock()
}

func (g *Syncer) clearError() {
	g.mu.Lock()
	g.lastSyncError = ""
	g.mu.Unlock()
}

func (g *Syncer) setLastSync(err error) {
	g.mu.Lock()
	g.lastSyncAt = time.Now()
	if err != nil {
		g.lastSyncError = err.Error()
	} else {
		g.lastSyncError = ""
	}
	g.mu.Unlock()
}

func (g *Syncer) recordConflict(c Conflict) {
	g.mu.Lock()
	defer g.mu.Unlock()
	g.recentConflicts = append(g.recentConflicts, c)
	if len(g.recentConflicts) > maxConflicts {
		g.recentConflicts = g.recentConflicts[len(g.recentConflicts)-maxConflicts:]
	}
}

func buildCommitMessage(subject, reason string) string {
	if reason == "" {
		return subject + "\n\nSource: simpledeploy-sync\n"
	}
	return subject + "\n\nSource: simpledeploy-sync\nReason: " + reason + "\n"
}
