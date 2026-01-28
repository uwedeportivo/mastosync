package main

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"text/template"

	_ "github.com/mattn/go-sqlite3"
	"github.com/mmcdole/gofeed"
)

type MockPoster struct {
	postedItems []*gofeed.Item
}

func (m *MockPoster) Post(item *gofeed.Item, tmpl *template.Template) (string, error) {
	m.postedItems = append(m.postedItems, item)
	return fmt.Sprintf("mock-id-%d", len(m.postedItems)), nil
}

func TestSyncer_SyncFeed(t *testing.T) {
	// Setup mock feed server
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/xml")
		fmt.Fprintln(w, `<?xml version="1.0" encoding="utf-8"?>
<rss version="2.0">
  <channel>
    <title>Test Feed</title>
    <item>
      <title>Item 1</title>
      <link>http://example.com/1</link>
      <guid>guid-1</guid>
    </item>
    <item>
      <title>Item 2</title>
      <link>http://example.com/2</link>
      <guid>guid-2</guid>
    </item>
  </channel>
</rss>`)
	})
	server := httptest.NewServer(handler)
	defer server.Close()

	// Setup temp DB
	dbPath := "test_syncer.db"
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

	// Setup temp template
	tmplDir, err := os.MkdirTemp("", "templates")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmplDir)
	tmplPath := "template.tmpl"
	err = os.WriteFile(filepath.Join(tmplDir, tmplPath), []byte("{{.Title}}"), 0644)
	if err != nil {
		t.Fatalf("Failed to write template: %v", err)
	}

	mockPoster := &MockPoster{}
	syncer := &Syncer{
		feedParser: gofeed.NewParser(),
		poster:     mockPoster,
		dao:        dao,
		tmplDir:    tmplDir,
	}

	alreadyProcessed := make(map[string]*gofeed.Item)

	// Test SyncFeed
	err = syncer.SyncFeed(server.URL, tmplPath, alreadyProcessed)
	if err != nil {
		t.Fatalf("SyncFeed failed: %v", err)
	}

	if len(mockPoster.postedItems) != 2 {
		t.Errorf("Expected 2 items posted, got %d", len(mockPoster.postedItems))
	}

	// Run again, should not post anything
	mockPoster.postedItems = nil
	err = syncer.SyncFeed(server.URL, tmplPath, alreadyProcessed)
	if err != nil {
		t.Fatalf("Second SyncFeed failed: %v", err)
	}

	if len(mockPoster.postedItems) != 0 {
		t.Errorf("Expected 0 items posted on second run, got %d", len(mockPoster.postedItems))
	}
}
