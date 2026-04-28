package store

import (
	"testing"
)

func TestCreateAndGetUser(t *testing.T) {
	s := newTestStore(t)

	u, err := s.CreateUser("alice", "hash123", "manage", "", "")
	if err != nil {
		t.Fatalf("CreateUser: %v", err)
	}
	if u.ID == 0 {
		t.Fatal("expected ID to be set")
	}
	if u.Username != "alice" {
		t.Errorf("Username = %q, want alice", u.Username)
	}
	if u.Role != "manage" {
		t.Errorf("Role = %q, want manage", u.Role)
	}
	if u.PasswordHash != "hash123" {
		t.Errorf("PasswordHash = %q, want hash123", u.PasswordHash)
	}

	got, err := s.GetUserByUsername("alice")
	if err != nil {
		t.Fatalf("GetUserByUsername: %v", err)
	}
	if got.ID != u.ID {
		t.Errorf("ID = %d, want %d", got.ID, u.ID)
	}
	if got.Username != u.Username {
		t.Errorf("Username = %q, want %q", got.Username, u.Username)
	}
	if got.Role != u.Role {
		t.Errorf("Role = %q, want %q", got.Role, u.Role)
	}
}

func TestCreateUserDuplicate(t *testing.T) {
	s := newTestStore(t)

	if _, err := s.CreateUser("bob", "h1", "viewer", "", ""); err != nil {
		t.Fatalf("CreateUser first: %v", err)
	}
	if _, err := s.CreateUser("bob", "h2", "viewer", "", ""); err == nil {
		t.Fatal("expected error for duplicate username, got nil")
	}
}

func TestListUsers(t *testing.T) {
	s := newTestStore(t)

	if _, err := s.CreateUser("zara", "h1", "viewer", "", ""); err != nil {
		t.Fatalf("CreateUser zara: %v", err)
	}
	if _, err := s.CreateUser("adam", "h2", "manage", "", ""); err != nil {
		t.Fatalf("CreateUser adam: %v", err)
	}

	users, err := s.ListUsers()
	if err != nil {
		t.Fatalf("ListUsers: %v", err)
	}
	if len(users) != 2 {
		t.Fatalf("len(users) = %d, want 2", len(users))
	}
	if users[0].Username != "adam" {
		t.Errorf("users[0].Username = %q, want adam (ordered by username)", users[0].Username)
	}
	// ListUsers excludes password_hash
	if users[0].PasswordHash != "" {
		t.Errorf("expected PasswordHash to be empty in ListUsers, got %q", users[0].PasswordHash)
	}
}

func TestDeleteUser(t *testing.T) {
	s := newTestStore(t)

	u, err := s.CreateUser("carol", "h1", "viewer", "", "")
	if err != nil {
		t.Fatalf("CreateUser: %v", err)
	}
	if err := s.DeleteUser(u.ID); err != nil {
		t.Fatalf("DeleteUser: %v", err)
	}
	if _, err := s.GetUserByID(u.ID); err == nil {
		t.Fatal("expected error after delete, got nil")
	}
}

func TestUserCount(t *testing.T) {
	s := newTestStore(t)

	count, err := s.UserCount()
	if err != nil {
		t.Fatalf("UserCount: %v", err)
	}
	if count != 0 {
		t.Errorf("UserCount = %d, want 0", count)
	}

	if _, err := s.CreateUser("dave", "h1", "viewer", "", ""); err != nil {
		t.Fatalf("CreateUser: %v", err)
	}

	count, err = s.UserCount()
	if err != nil {
		t.Fatalf("UserCount: %v", err)
	}
	if count != 1 {
		t.Errorf("UserCount = %d, want 1", count)
	}
}

func TestCreateAndGetAPIKey(t *testing.T) {
	s := newTestStore(t)

	u, err := s.CreateUser("eve", "h1", "viewer", "", "")
	if err != nil {
		t.Fatalf("CreateUser: %v", err)
	}

	k, err := s.CreateAPIKey(u.ID, "hashABC", "my-key")
	if err != nil {
		t.Fatalf("CreateAPIKey: %v", err)
	}
	if k.ID == 0 {
		t.Fatal("expected key ID to be set")
	}
	if k.KeyHash != "hashABC" {
		t.Errorf("KeyHash = %q, want hashABC", k.KeyHash)
	}

	gotKey, gotUser, err := s.GetAPIKeyByHash("hashABC")
	if err != nil {
		t.Fatalf("GetAPIKeyByHash: %v", err)
	}
	if gotKey.ID != k.ID {
		t.Errorf("key ID = %d, want %d", gotKey.ID, k.ID)
	}
	if gotUser.ID != u.ID {
		t.Errorf("user ID = %d, want %d", gotUser.ID, u.ID)
	}
	if gotUser.Username != "eve" {
		t.Errorf("user Username = %q, want eve", gotUser.Username)
	}
}

func TestListAPIKeysByUser(t *testing.T) {
	s := newTestStore(t)

	u, err := s.CreateUser("frank", "h1", "viewer", "", "")
	if err != nil {
		t.Fatalf("CreateUser: %v", err)
	}

	if _, err := s.CreateAPIKey(u.ID, "hash1", "key-one"); err != nil {
		t.Fatalf("CreateAPIKey 1: %v", err)
	}
	if _, err := s.CreateAPIKey(u.ID, "hash2", "key-two"); err != nil {
		t.Fatalf("CreateAPIKey 2: %v", err)
	}

	keys, err := s.ListAPIKeysByUser(u.ID)
	if err != nil {
		t.Fatalf("ListAPIKeysByUser: %v", err)
	}
	if len(keys) != 2 {
		t.Fatalf("len(keys) = %d, want 2", len(keys))
	}
}

func TestDeleteAPIKey(t *testing.T) {
	s := newTestStore(t)

	u, err := s.CreateUser("grace", "h1", "viewer", "", "")
	if err != nil {
		t.Fatalf("CreateUser: %v", err)
	}

	k, err := s.CreateAPIKey(u.ID, "hashDEL", "doomed-key")
	if err != nil {
		t.Fatalf("CreateAPIKey: %v", err)
	}
	if err := s.DeleteAPIKey(k.ID, u.ID); err != nil {
		t.Fatalf("DeleteAPIKey: %v", err)
	}
	if _, _, err := s.GetAPIKeyByHash("hashDEL"); err == nil {
		t.Fatal("expected error after delete, got nil")
	}
}

func TestGrantAndCheckAppAccess(t *testing.T) {
	s := newTestStore(t)

	app := &App{
		Name:        "test-app",
		Slug:        "test-app",
		ComposePath: "/apps/test-app/docker-compose.yml",
		Status:      "stopped",
	}
	if err := s.UpsertApp(app, nil); err != nil {
		t.Fatalf("UpsertApp: %v", err)
	}

	u, err := s.CreateUser("henry", "h1", "viewer", "", "")
	if err != nil {
		t.Fatalf("CreateUser: %v", err)
	}

	ok, err := s.HasAppAccess(u.ID, "test-app")
	if err != nil {
		t.Fatalf("HasAppAccess before grant: %v", err)
	}
	if ok {
		t.Fatal("expected no access before grant")
	}

	if err := s.GrantAppAccess(u.ID, app.ID); err != nil {
		t.Fatalf("GrantAppAccess: %v", err)
	}

	ok, err = s.HasAppAccess(u.ID, "test-app")
	if err != nil {
		t.Fatalf("HasAppAccess after grant: %v", err)
	}
	if !ok {
		t.Fatal("expected access after grant")
	}

	if err := s.RevokeAppAccess(u.ID, app.ID); err != nil {
		t.Fatalf("RevokeAppAccess: %v", err)
	}

	ok, err = s.HasAppAccess(u.ID, "test-app")
	if err != nil {
		t.Fatalf("HasAppAccess after revoke: %v", err)
	}
	if ok {
		t.Fatal("expected no access after revoke")
	}
}

func TestSuperAdminBypassesAppAccess(t *testing.T) {
	s := newTestStore(t)

	app := &App{
		Name:        "secret-app",
		Slug:        "secret-app",
		ComposePath: "/apps/secret-app/docker-compose.yml",
		Status:      "stopped",
	}
	if err := s.UpsertApp(app, nil); err != nil {
		t.Fatalf("UpsertApp: %v", err)
	}

	u, err := s.CreateUser("superuser", "h1", "super_admin", "", "")
	if err != nil {
		t.Fatalf("CreateUser: %v", err)
	}

	ok, err := s.HasAppAccess(u.ID, "secret-app")
	if err != nil {
		t.Fatalf("HasAppAccess: %v", err)
	}
	if !ok {
		t.Fatal("expected super_admin to have access without explicit grant")
	}
}
