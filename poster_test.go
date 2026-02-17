package main

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"text/template"

	skybot "github.com/danrusei/gobot-bsky"
	mdon "github.com/mattn/go-mastodon"
	"github.com/mmcdole/gofeed"
)

func TestMastodonPoster_Post(t *testing.T) {
	// Setup mock Mastodon server
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "POST" {
			t.Errorf("Expected POST request, got %s", r.Method)
		}
		if r.URL.Path != "/api/v1/statuses" {
			t.Errorf("Expected /api/v1/statuses path, got %s", r.URL.Path)
		}

		// Mock response
		resp := mdon.Status{
			ID: "test-status-id",
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(resp)
	})
	server := httptest.NewServer(handler)
	defer server.Close()

	// Initialize Mastodon client pointing to mock server
	client := mdon.NewClient(&mdon.Config{
		Server: server.URL,
	})

	poster := &MastodonPoster{
		mClient: client,
	}

	tmpl, _ := template.New("test").Parse("Title: {{.Title}}")
	item := &gofeed.Item{
		Title: "Hello Mastodon",
	}

	id, err := poster.Post(item, tmpl)
	if err != nil {
		t.Fatalf("Post failed: %v", err)
	}

	if id != "test-status-id" {
		t.Errorf("Expected ID %q, got %q", "test-status-id", id)
	}
}

func TestMastodonPoster_Post_Truncation(t *testing.T) {
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		r.ParseForm()
		status := r.Form.Get("status")
		if len(status) > kMastodonMaxTootLen {
			t.Errorf("Status was not truncated: length %d", len(status))
		}

		resp := mdon.Status{ID: "id"}
		json.NewEncoder(w).Encode(resp)
	})
	server := httptest.NewServer(handler)
	defer server.Close()

	client := mdon.NewClient(&mdon.Config{
		Server: server.URL,
	})
	poster := &MastodonPoster{mClient: client}

	var longTitle strings.Builder
	for range 600 {
		longTitle.WriteString("a")
	}
	tmpl, _ := template.New("test").Parse("{{.Title}}")
	item := &gofeed.Item{Title: longTitle.String()}

	_, err := poster.Post(item, tmpl)
	if err != nil {
		t.Fatalf("Post failed: %v", err)
	}
}

func TestBlueskyPoster_Post(t *testing.T) {
	// Setup mock Bluesky server
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if strings.Contains(r.URL.Path, "com.atproto.server.createSession") {
			resp := map[string]any{
				"accessJwt":  "test-access-jwt",
				"refreshJwt": "test-refresh-jwt",
				"handle":     "test-handle",
				"did":        "did:plc:test-did",
			}
			json.NewEncoder(w).Encode(resp)
			return
		}
		if strings.Contains(r.URL.Path, "com.atproto.repo.createRecord") {
			resp := map[string]any{
				"cid": "test-cid",
				"uri": "at://did:plc:test-did/app.bsky.feed.post/test-post-id",
			}
			json.NewEncoder(w).Encode(resp)
			return
		}
		// Default mock response for other XRPC or similar
		resp := map[string]any{
			"cid": "test-cid",
			"uri": "test-uri",
		}
		json.NewEncoder(w).Encode(resp)
	})
	server := httptest.NewServer(handler)
	defer server.Close()

	ctx := context.Background()
	agent := skybot.NewAgent(ctx, server.URL, "handle", "apikey")
	err := agent.Connect(ctx)
	if err != nil {
		t.Fatalf("Connect failed: %v", err)
	}

	poster := &BlueskyPoster{
		skyAgent: &agent,
	}

	tmpl, _ := template.New("test").Parse("{{.Title}}")
	item := &gofeed.Item{
		Title: "Hello Bluesky",
		Link:  "https://example.com/1",
	}

	cid, err := poster.Post(item, tmpl)
	if err != nil {
		t.Fatalf("Post failed: %v", err)
	}

	if cid == "" {
		t.Error("Expected non-empty CID")
	}
}
