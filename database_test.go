package main

import (
	"os"
	"testing"
	"time"

	_ "github.com/mattn/go-sqlite3"
)

func TestDAO_FindToot(t *testing.T) {
	dbPath := "test_mastosync.db"
	defer os.Remove(dbPath)

	err := CreateDB(dbPath)
	if err != nil {
		t.Fatalf("Failed to create DB: %v", err)
	}

	dao, err := OpenDB(dbPath)
	if err != nil {
		t.Fatalf("Failed to open DB: %v", err)
	}
	defer dao.db.Close()

	rssguid := "test-guid"
	mastid := "test-mast-id"
	now := time.Now().Truncate(time.Second) // SQLite precision might vary, truncate for comparison

	// Test RecordSync
	err = dao.RecordSync(rssguid, mastid, now)
	if err != nil {
		t.Fatalf("Failed to record sync: %v", err)
	}

	// Test FindToot (existing)
	toot, err := dao.FindToot(rssguid)
	if err != nil {
		t.Fatalf("FindToot failed: %v", err)
	}
	if toot == nil {
		t.Fatal("FindToot returned nil for existing record")
	}
	if toot.MastID != mastid {
		t.Errorf("Expected MastID %s, got %s", mastid, toot.MastID)
	}

	// Test FindToot (non-existing)
	toot, err = dao.FindToot("non-existing")
	if err != nil {
		t.Fatalf("FindToot failed for non-existing: %v", err)
	}
	if toot != nil {
		t.Errorf("Expected nil for non-existing record, got %v", toot)
	}
}
