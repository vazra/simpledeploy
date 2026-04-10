package store

import (
	"path/filepath"
	"testing"
)

func openTestDB(t *testing.T) *Store {
	t.Helper()
	dir := t.TempDir()
	db, err := Open(filepath.Join(dir, "test.db"))
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	t.Cleanup(func() { db.Close() })
	return db
}

func TestCreateAndListRegistries(t *testing.T) {
	db := openTestDB(t)

	reg, err := db.CreateRegistry("ghcr", "ghcr.io", "enc-user", "enc-pass")
	if err != nil {
		t.Fatalf("CreateRegistry: %v", err)
	}
	if reg.ID == "" {
		t.Fatal("expected non-empty ID")
	}
	if reg.Name != "ghcr" {
		t.Errorf("Name = %q, want ghcr", reg.Name)
	}

	regs, err := db.ListRegistries()
	if err != nil {
		t.Fatalf("ListRegistries: %v", err)
	}
	if len(regs) != 1 {
		t.Fatalf("got %d registries, want 1", len(regs))
	}
	if regs[0].URL != "ghcr.io" {
		t.Errorf("URL = %q, want ghcr.io", regs[0].URL)
	}
}

func TestGetRegistryByName(t *testing.T) {
	db := openTestDB(t)
	db.CreateRegistry("ecr", "123.dkr.ecr.us-east-1.amazonaws.com", "u", "p")

	reg, err := db.GetRegistryByName("ecr")
	if err != nil {
		t.Fatalf("GetRegistryByName: %v", err)
	}
	if reg.URL != "123.dkr.ecr.us-east-1.amazonaws.com" {
		t.Errorf("URL = %q", reg.URL)
	}
}

func TestUpdateRegistry(t *testing.T) {
	db := openTestDB(t)
	reg, _ := db.CreateRegistry("test", "old.io", "u", "p")

	err := db.UpdateRegistry(reg.ID, "test2", "new.io", "u2", "p2")
	if err != nil {
		t.Fatalf("UpdateRegistry: %v", err)
	}

	updated, _ := db.GetRegistry(reg.ID)
	if updated.Name != "test2" || updated.URL != "new.io" {
		t.Errorf("got name=%q url=%q", updated.Name, updated.URL)
	}
}

func TestDeleteRegistry(t *testing.T) {
	db := openTestDB(t)
	reg, _ := db.CreateRegistry("del", "del.io", "u", "p")

	if err := db.DeleteRegistry(reg.ID); err != nil {
		t.Fatalf("DeleteRegistry: %v", err)
	}
	regs, _ := db.ListRegistries()
	if len(regs) != 0 {
		t.Errorf("got %d registries after delete", len(regs))
	}
}

func TestCreateRegistryDuplicateName(t *testing.T) {
	db := openTestDB(t)
	db.CreateRegistry("dup", "a.io", "u", "p")
	_, err := db.CreateRegistry("dup", "b.io", "u", "p")
	if err == nil {
		t.Fatal("expected error for duplicate name")
	}
}
