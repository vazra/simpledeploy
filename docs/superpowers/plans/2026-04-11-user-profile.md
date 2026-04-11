# User Profile Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Let users view/edit their profile (display name, email) and change their password.

**Architecture:** New migration adds display_name/email to users table. Three new API endpoints (GET/PUT /api/me, PUT /api/me/password). New Profile.svelte page linked from sidebar. All existing queries updated for new columns.

**Tech Stack:** Go, SQLite, Svelte 5 (runes), svelte-spa-router

---

### Task 1: Database Migration

**Files:**
- Create: `internal/store/migrations/012_user_profile.sql`

- [ ] **Step 1: Create migration file**

```sql
ALTER TABLE users ADD COLUMN display_name TEXT NOT NULL DEFAULT '';
ALTER TABLE users ADD COLUMN email TEXT NOT NULL DEFAULT '';
```

- [ ] **Step 2: Commit**

```bash
git add internal/store/migrations/012_user_profile.sql
git commit -m "feat(store): add display_name and email columns to users"
```

---

### Task 2: Update Store Layer

**Files:**
- Modify: `internal/store/users.go`
- Test: `internal/store/users_test.go` (if exists, or create)

- [ ] **Step 1: Update User struct**

In `internal/store/users.go`, add `DisplayName` and `Email` fields to `User`:

```go
type User struct {
	ID           int64
	Username     string
	PasswordHash string
	Role         string
	DisplayName  string
	Email        string
	CreatedAt    time.Time
}
```

- [ ] **Step 2: Update all SELECT queries to include new columns**

Every query that scans into a User must now include `display_name, email`. Update these methods:

**CreateUser** - update INSERT RETURNING and Scan:
```go
func (s *Store) CreateUser(username, passwordHash, role string) (*User, error) {
	var u User
	err := s.db.QueryRow(`
		INSERT INTO users (username, password_hash, role)
		VALUES (?, ?, ?)
		RETURNING id, username, password_hash, role, display_name, email, created_at
	`, username, passwordHash, role).Scan(
		&u.ID, &u.Username, &u.PasswordHash, &u.Role, &u.DisplayName, &u.Email, &u.CreatedAt,
	)
	if err != nil {
		return nil, fmt.Errorf("create user: %w", err)
	}
	return &u, nil
}
```

**GetUserByUsername** - update SELECT and Scan:
```go
func (s *Store) GetUserByUsername(username string) (*User, error) {
	var u User
	err := s.db.QueryRow(`
		SELECT id, username, password_hash, role, display_name, email, created_at
		FROM users WHERE username = ?
	`, username).Scan(&u.ID, &u.Username, &u.PasswordHash, &u.Role, &u.DisplayName, &u.Email, &u.CreatedAt)
	if err == sql.ErrNoRows {
		return nil, fmt.Errorf("user %q not found", username)
	}
	if err != nil {
		return nil, fmt.Errorf("get user by username: %w", err)
	}
	return &u, nil
}
```

**GetUserByID** - update SELECT and Scan:
```go
func (s *Store) GetUserByID(id int64) (*User, error) {
	var u User
	err := s.db.QueryRow(`
		SELECT id, username, password_hash, role, display_name, email, created_at
		FROM users WHERE id = ?
	`, id).Scan(&u.ID, &u.Username, &u.PasswordHash, &u.Role, &u.DisplayName, &u.Email, &u.CreatedAt)
	if err == sql.ErrNoRows {
		return nil, fmt.Errorf("user %d not found", id)
	}
	if err != nil {
		return nil, fmt.Errorf("get user by id: %w", err)
	}
	return &u, nil
}
```

**ListUsers** - update SELECT and Scan (note: this one excludes password_hash):
```go
func (s *Store) ListUsers() ([]User, error) {
	rows, err := s.db.Query(`
		SELECT id, username, role, display_name, email, created_at
		FROM users ORDER BY username
	`)
	if err != nil {
		return nil, fmt.Errorf("list users: %w", err)
	}
	defer rows.Close()

	var users []User
	for rows.Next() {
		var u User
		if err := rows.Scan(&u.ID, &u.Username, &u.Role, &u.DisplayName, &u.Email, &u.CreatedAt); err != nil {
			return nil, fmt.Errorf("scan user: %w", err)
		}
		users = append(users, u)
	}
	return users, rows.Err()
}
```

**GetAPIKeyByHash** - update the user portion of the JOIN query. The user fields in the SELECT and Scan must include display_name and email:
```go
func (s *Store) GetAPIKeyByHash(hash string) (*APIKeyRecord, *User, error) {
	var k APIKeyRecord
	var u User
	var expiresAt sql.NullTime
	err := s.db.QueryRow(`
		SELECT
			ak.id, ak.user_id, ak.key_hash, ak.name, ak.created_at, ak.expires_at,
			u.id, u.username, u.password_hash, u.role, u.display_name, u.email, u.created_at
		FROM api_keys ak
		JOIN users u ON u.id = ak.user_id
		WHERE ak.key_hash = ?
	`, hash).Scan(
		&k.ID, &k.UserID, &k.KeyHash, &k.Name, &k.CreatedAt, &expiresAt,
		&u.ID, &u.Username, &u.PasswordHash, &u.Role, &u.DisplayName, &u.Email, &u.CreatedAt,
	)
	if err == sql.ErrNoRows {
		return nil, nil, fmt.Errorf("api key not found")
	}
	if err != nil {
		return nil, nil, fmt.Errorf("get api key by hash: %w", err)
	}
	if expiresAt.Valid {
		t := expiresAt.Time
		k.ExpiresAt = &t
	}
	return &k, &u, nil
}
```

- [ ] **Step 3: Add UpdateProfile method**

```go
// UpdateProfile updates the user's display name and email.
func (s *Store) UpdateProfile(id int64, displayName, email string) error {
	res, err := s.db.Exec(`UPDATE users SET display_name = ?, email = ? WHERE id = ?`, displayName, email, id)
	if err != nil {
		return fmt.Errorf("update profile: %w", err)
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return fmt.Errorf("user %d not found", id)
	}
	return nil
}
```

- [ ] **Step 4: Add UpdatePassword method**

```go
// UpdatePassword updates the user's password hash.
func (s *Store) UpdatePassword(id int64, newHash string) error {
	res, err := s.db.Exec(`UPDATE users SET password_hash = ? WHERE id = ?`, newHash, id)
	if err != nil {
		return fmt.Errorf("update password: %w", err)
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return fmt.Errorf("user %d not found", id)
	}
	return nil
}
```

- [ ] **Step 5: Run tests**

```bash
go test ./internal/store/ -v
```

Expected: all existing store tests pass (new columns have defaults so nothing breaks).

- [ ] **Step 6: Commit**

```bash
git add internal/store/users.go
git commit -m "feat(store): add profile fields, UpdateProfile, UpdatePassword"
```

---

### Task 3: API Endpoints

**Files:**
- Modify: `internal/api/users.go` (add handlers + update userResponse)
- Modify: `internal/api/server.go` (register routes)

- [ ] **Step 1: Update userResponse and toUserResponse**

In `internal/api/users.go`, update the response struct to include new fields:

```go
type userResponse struct {
	ID          int64  `json:"id"`
	Username    string `json:"username"`
	Role        string `json:"role"`
	DisplayName string `json:"display_name"`
	Email       string `json:"email"`
}

func toUserResponse(u *store.User) userResponse {
	return userResponse{ID: u.ID, Username: u.Username, Role: u.Role, DisplayName: u.DisplayName, Email: u.Email}
}
```

- [ ] **Step 2: Add handleGetMe handler**

In `internal/api/users.go`:

```go
func (s *Server) handleGetMe(w http.ResponseWriter, r *http.Request) {
	authUser := GetAuthUser(r)
	if authUser == nil {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}
	user, err := s.store.GetUserByID(authUser.ID)
	if err != nil {
		httpError(w, err, http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(toUserResponse(user))
}
```

- [ ] **Step 3: Add handleUpdateMe handler**

```go
func (s *Server) handleUpdateMe(w http.ResponseWriter, r *http.Request) {
	authUser := GetAuthUser(r)
	if authUser == nil {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}
	var body struct {
		DisplayName string `json:"display_name"`
		Email       string `json:"email"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}
	if err := s.store.UpdateProfile(authUser.ID, body.DisplayName, body.Email); err != nil {
		httpError(w, err, http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
}
```

- [ ] **Step 4: Add handleChangePassword handler**

```go
func (s *Server) handleChangePassword(w http.ResponseWriter, r *http.Request) {
	authUser := GetAuthUser(r)
	if authUser == nil {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}
	var body struct {
		CurrentPassword string `json:"current_password"`
		NewPassword     string `json:"new_password"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}
	if body.NewPassword == "" {
		http.Error(w, "new password required", http.StatusBadRequest)
		return
	}
	user, err := s.store.GetUserByID(authUser.ID)
	if err != nil {
		httpError(w, err, http.StatusInternalServerError)
		return
	}
	if !auth.CheckPassword(user.PasswordHash, body.CurrentPassword) {
		http.Error(w, "current password is incorrect", http.StatusBadRequest)
		return
	}
	hash, err := auth.HashPassword(body.NewPassword)
	if err != nil {
		httpError(w, err, http.StatusInternalServerError)
		return
	}
	if err := s.store.UpdatePassword(authUser.ID, hash); err != nil {
		httpError(w, err, http.StatusInternalServerError)
		return
	}
	if s.audit != nil {
		s.audit.Log(audit.Event{Type: "password_changed", Username: authUser.Username, Success: true})
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
}
```

- [ ] **Step 5: Register routes in server.go**

In `internal/api/server.go`, add after the existing user routes (around line 142):

```go
s.mux.Handle("GET /api/me", s.authMiddleware(http.HandlerFunc(s.handleGetMe)))
s.mux.Handle("PUT /api/me", s.authMiddleware(http.HandlerFunc(s.handleUpdateMe)))
s.mux.Handle("PUT /api/me/password", s.authMiddleware(http.HandlerFunc(s.handleChangePassword)))
```

- [ ] **Step 6: Update handleListUsers to include new fields in response**

The existing `handleListUsers` builds its response manually. Update it to use `toUserResponse`:

```go
func (s *Server) handleListUsers(w http.ResponseWriter, r *http.Request) {
	if requireRole(w, r, "super_admin") == nil {
		return
	}
	users, err := s.store.ListUsers()
	if err != nil {
		httpError(w, err, http.StatusInternalServerError)
		return
	}
	resp := make([]userResponse, len(users))
	for i, u := range users {
		resp[i] = toUserResponse(&u)
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(resp)
}
```

- [ ] **Step 7: Run tests**

```bash
go test ./internal/api/ -v
```

Expected: all existing API tests pass.

- [ ] **Step 8: Commit**

```bash
git add internal/api/users.go internal/api/server.go
git commit -m "feat(api): add GET/PUT /api/me and PUT /api/me/password"
```

---

### Task 4: API Tests

**Files:**
- Modify: `internal/api/users_test.go`

- [ ] **Step 1: Write test for GET /api/me**

Add to `internal/api/users_test.go`:

```go
func TestGetMe(t *testing.T) {
	srv, _, cookie := setupUserTestServer(t)

	req := authedRequest(t, http.MethodGet, "/api/me", nil, cookie)
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("GET /api/me status = %d, want 200", w.Code)
	}
	var resp map[string]any
	json.NewDecoder(w.Body).Decode(&resp)
	if resp["username"] != "admin" {
		t.Errorf("username = %v, want admin", resp["username"])
	}
	if resp["role"] != "super_admin" {
		t.Errorf("role = %v, want super_admin", resp["role"])
	}
}
```

- [ ] **Step 2: Write test for PUT /api/me**

```go
func TestUpdateMe(t *testing.T) {
	srv, st, cookie := setupUserTestServer(t)

	req := authedRequest(t, http.MethodPut, "/api/me", map[string]string{
		"display_name": "Admin User",
		"email":        "admin@example.com",
	}, cookie)
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("PUT /api/me status = %d, want 200", w.Code)
	}

	// Verify in DB
	user, err := st.GetUserByUsername("admin")
	if err != nil {
		t.Fatalf("get user: %v", err)
	}
	if user.DisplayName != "Admin User" {
		t.Errorf("display_name = %q, want %q", user.DisplayName, "Admin User")
	}
	if user.Email != "admin@example.com" {
		t.Errorf("email = %q, want %q", user.Email, "admin@example.com")
	}
}
```

- [ ] **Step 3: Write test for PUT /api/me/password**

```go
func TestChangePassword(t *testing.T) {
	srv, st, cookie := setupUserTestServer(t)

	// Change password
	req := authedRequest(t, http.MethodPut, "/api/me/password", map[string]string{
		"current_password": "password123",
		"new_password":     "newpass456",
	}, cookie)
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("PUT /api/me/password status = %d, want 200", w.Code)
	}

	// Verify new password works
	user, _ := st.GetUserByUsername("admin")
	if !auth.CheckPassword(user.PasswordHash, "newpass456") {
		t.Error("new password should be valid")
	}
}

func TestChangePasswordWrongCurrent(t *testing.T) {
	srv, _, cookie := setupUserTestServer(t)

	req := authedRequest(t, http.MethodPut, "/api/me/password", map[string]string{
		"current_password": "wrongpassword",
		"new_password":     "newpass456",
	}, cookie)
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", w.Code)
	}
}
```

- [ ] **Step 4: Run tests**

```bash
go test ./internal/api/ -run "TestGetMe|TestUpdateMe|TestChangePassword" -v
```

Expected: all 4 tests pass.

- [ ] **Step 5: Commit**

```bash
git add internal/api/users_test.go
git commit -m "test(api): add profile endpoint tests"
```

---

### Task 5: Frontend - API Client + Route

**Files:**
- Modify: `ui/src/lib/api.js`
- Modify: `ui/src/App.svelte`
- Create: `ui/src/routes/Profile.svelte`

- [ ] **Step 1: Add API methods**

In `ui/src/lib/api.js`, add to the `api` export object in the Users section:

```js
  // Profile
  getProfile: () => request('GET', '/me'),
  updateProfile: (data) => requestWithToast('PUT', '/me', data, 'Profile updated'),
  changePassword: (data) => requestWithToast('PUT', '/me/password', data, 'Password changed'),
```

- [ ] **Step 2: Add route in App.svelte**

Import Profile and add the route:

```js
import Profile from './routes/Profile.svelte'
```

Add to routes object:
```js
'/profile': Profile,
```

- [ ] **Step 3: Create Profile.svelte**

Create `ui/src/routes/Profile.svelte`:

```svelte
<script>
  import { onMount } from 'svelte'
  import Layout from '../components/Layout.svelte'
  import Button from '../components/Button.svelte'
  import Skeleton from '../components/Skeleton.svelte'
  import { api } from '../lib/api.js'

  let loading = $state(true)
  let saving = $state(false)
  let changingPw = $state(false)

  let displayName = $state('')
  let email = $state('')
  let username = $state('')
  let role = $state('')
  let createdAt = $state('')

  let currentPw = $state('')
  let newPw = $state('')
  let confirmPw = $state('')
  let pwError = $state('')

  onMount(loadProfile)

  async function loadProfile() {
    loading = true
    const res = await api.getProfile()
    if (res.data) {
      displayName = res.data.display_name || ''
      email = res.data.email || ''
      username = res.data.username
      role = res.data.role
      createdAt = new Date(res.data.created_at).toLocaleDateString()
    }
    loading = false
  }

  async function saveProfile() {
    saving = true
    await api.updateProfile({ display_name: displayName, email })
    saving = false
  }

  async function changePassword() {
    pwError = ''
    if (newPw !== confirmPw) {
      pwError = 'Passwords do not match'
      return
    }
    if (!newPw) {
      pwError = 'New password required'
      return
    }
    changingPw = true
    const res = await api.changePassword({ current_password: currentPw, new_password: newPw })
    changingPw = false
    if (!res.error) {
      currentPw = ''
      newPw = ''
      confirmPw = ''
    }
  }
</script>

<Layout title="Profile">
  {#if loading}
    <div class="space-y-4 max-w-lg">
      <Skeleton class="h-10 w-full" />
      <Skeleton class="h-10 w-full" />
      <Skeleton class="h-10 w-full" />
    </div>
  {:else}
    <div class="max-w-lg space-y-8">
      <!-- Account Info (read-only) -->
      <section>
        <h2 class="text-sm font-medium text-text-secondary mb-3">Account</h2>
        <div class="bg-surface-2 rounded-lg p-4 space-y-2 text-sm">
          <div class="flex justify-between"><span class="text-text-secondary">Username</span><span class="text-text-primary">{username}</span></div>
          <div class="flex justify-between"><span class="text-text-secondary">Role</span><span class="text-text-primary">{role}</span></div>
          <div class="flex justify-between"><span class="text-text-secondary">Created</span><span class="text-text-primary">{createdAt}</span></div>
        </div>
      </section>

      <!-- Profile -->
      <section>
        <h2 class="text-sm font-medium text-text-secondary mb-3">Profile</h2>
        <div class="space-y-3">
          <div>
            <label for="displayName" class="block text-xs text-text-secondary mb-1">Display Name</label>
            <input id="displayName" type="text" bind:value={displayName}
              class="w-full px-3 py-2 bg-surface-2 border border-border rounded-lg text-sm text-text-primary focus:outline-none focus:ring-1 focus:ring-accent" />
          </div>
          <div>
            <label for="email" class="block text-xs text-text-secondary mb-1">Email</label>
            <input id="email" type="email" bind:value={email}
              class="w-full px-3 py-2 bg-surface-2 border border-border rounded-lg text-sm text-text-primary focus:outline-none focus:ring-1 focus:ring-accent" />
          </div>
          <Button onclick={saveProfile} disabled={saving}>{saving ? 'Saving...' : 'Save Profile'}</Button>
        </div>
      </section>

      <!-- Password -->
      <section>
        <h2 class="text-sm font-medium text-text-secondary mb-3">Change Password</h2>
        <div class="space-y-3">
          <div>
            <label for="currentPw" class="block text-xs text-text-secondary mb-1">Current Password</label>
            <input id="currentPw" type="password" bind:value={currentPw}
              class="w-full px-3 py-2 bg-surface-2 border border-border rounded-lg text-sm text-text-primary focus:outline-none focus:ring-1 focus:ring-accent" />
          </div>
          <div>
            <label for="newPw" class="block text-xs text-text-secondary mb-1">New Password</label>
            <input id="newPw" type="password" bind:value={newPw}
              class="w-full px-3 py-2 bg-surface-2 border border-border rounded-lg text-sm text-text-primary focus:outline-none focus:ring-1 focus:ring-accent" />
          </div>
          <div>
            <label for="confirmPw" class="block text-xs text-text-secondary mb-1">Confirm Password</label>
            <input id="confirmPw" type="password" bind:value={confirmPw}
              class="w-full px-3 py-2 bg-surface-2 border border-border rounded-lg text-sm text-text-primary focus:outline-none focus:ring-1 focus:ring-accent" />
          </div>
          {#if pwError}
            <p class="text-xs text-danger">{pwError}</p>
          {/if}
          <Button onclick={changePassword} disabled={changingPw}>{changingPw ? 'Changing...' : 'Change Password'}</Button>
        </div>
      </section>
    </div>
  {/if}
</Layout>
```

- [ ] **Step 4: Commit**

```bash
git add ui/src/lib/api.js ui/src/App.svelte ui/src/routes/Profile.svelte
git commit -m "feat(ui): add profile page with edit and password change"
```

---

### Task 6: Sidebar Profile Link

**Files:**
- Modify: `ui/src/components/Sidebar.svelte`

- [ ] **Step 1: Add profile link to sidebar bottom**

In `ui/src/components/Sidebar.svelte`, replace the logout section (the `<div>` at the bottom with ThemeToggle and logout button) with a version that includes a profile link:

Replace the bottom `<div class="flex flex-col gap-1 p-3 border-t border-border/30">` section:

```svelte
  <div class="flex flex-col gap-1 p-3 border-t border-border/30">
    <a
      href="#/profile"
      class="flex items-center gap-2.5 px-3 py-2 rounded-lg text-sm transition-colors
        {isActive('/profile') ? 'bg-surface-3/50 text-text-primary font-medium' : 'text-text-secondary hover:text-text-primary hover:bg-surface-3/30'}"
      title={forceExpanded || $sidebarExpanded ? '' : 'Profile'}
    >
      <svg class="w-5 h-5 shrink-0" fill="none" viewBox="0 0 24 24" stroke="currentColor" stroke-width="1.5">
        <path stroke-linecap="round" stroke-linejoin="round" d="M15.75 6a3.75 3.75 0 11-7.5 0 3.75 3.75 0 017.5 0zM4.501 20.118a7.5 7.5 0 0114.998 0A17.933 17.933 0 0112 21.75c-2.676 0-5.216-.584-7.499-1.632z" />
      </svg>
      {#if forceExpanded || $sidebarExpanded}
        <span class="whitespace-nowrap">Profile</span>
      {/if}
    </a>
    <div class="flex items-center {$sidebarExpanded ? 'justify-between' : 'justify-center'}">
      <ThemeToggle />
      {#if forceExpanded || $sidebarExpanded}
        <button
          onclick={logout}
          class="text-xs text-text-muted hover:text-danger transition-colors"
        >
          Logout
        </button>
      {/if}
    </div>
    <button
      onclick={toggle}
      class="flex items-center justify-center w-full py-2 rounded-lg text-text-secondary hover:text-text-primary hover:bg-surface-3/50 transition-colors"
      title={$sidebarExpanded ? 'Collapse sidebar' : 'Expand sidebar'}
      aria-label="Toggle sidebar"
    >
      <svg class="w-4 h-4 transition-transform {$sidebarExpanded ? '' : 'rotate-180'}" fill="none" viewBox="0 0 24 24" stroke="currentColor" stroke-width="2">
        <path stroke-linecap="round" stroke-linejoin="round" d="M18.75 19.5l-7.5-7.5 7.5-7.5m-6 15L5.25 12l7.5-7.5" />
      </svg>
    </button>
  </div>
```

- [ ] **Step 2: Commit**

```bash
git add ui/src/components/Sidebar.svelte
git commit -m "feat(ui): add profile link to sidebar"
```

---

### Task 7: Build and Verify

- [ ] **Step 1: Run all Go tests**

```bash
go test ./...
```

Expected: all tests pass.

- [ ] **Step 2: Build UI**

```bash
cd ui && npm run build && cd ..
```

Expected: build succeeds with no errors.

- [ ] **Step 3: Build Go binary**

```bash
make build
```

Expected: binary compiles successfully.
