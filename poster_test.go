package main

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"text/template"
	"time"

	skybot "github.com/danrusei/gobot-bsky"
	mdon "github.com/mattn/go-mastodon"
	"github.com/mmcdole/gofeed"
	ext "github.com/mmcdole/gofeed/extensions"
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

func TestFeatureImageURL(t *testing.T) {
	item := &gofeed.Item{
		Extensions: ext.Extensions{
			"media": {
				"content": []ext.Extension{
					{Attrs: map[string]string{"url": "https://example.com/posts/foo/feature.png"}},
				},
			},
		},
	}
	if got := featureImageURL(item); got != "https://example.com/posts/foo/feature.png" {
		t.Errorf("expected feature image url, got %q", got)
	}

	if got := featureImageURL(&gofeed.Item{}); got != "" {
		t.Errorf("expected empty string for item without media:content, got %q", got)
	}
}

func TestBlueskyPoster_Post_UploadsFeatureImageThumb(t *testing.T) {
	imgServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "image/png")
		w.Write([]byte("fake-png-bytes"))
	}))
	defer imgServer.Close()

	var uploadBlobCalled bool
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch {
		case strings.Contains(r.URL.Path, "com.atproto.server.createSession"):
			json.NewEncoder(w).Encode(map[string]any{
				"accessJwt": "test-access-jwt", "refreshJwt": "test-refresh-jwt",
				"handle": "test-handle", "did": "did:plc:test-did",
			})
		case strings.Contains(r.URL.Path, "com.atproto.repo.uploadBlob"):
			uploadBlobCalled = true
			json.NewEncoder(w).Encode(map[string]any{
				"blob": map[string]any{
					"$type":    "blob",
					"ref":      map[string]any{"$link": "bafkreicwamkg77pijyudfbdmskelsnuztr6gp62lqfjv3e3urbs3gxnv2m"},
					"mimeType": "image/png",
					"size":     14,
				},
			})
		case strings.Contains(r.URL.Path, "com.atproto.repo.createRecord"):
			json.NewEncoder(w).Encode(map[string]any{
				"cid": "test-cid",
				"uri": "at://did:plc:test-did/app.bsky.feed.post/test-post-id",
			})
		default:
			json.NewEncoder(w).Encode(map[string]any{"cid": "test-cid", "uri": "test-uri"})
		}
	})
	server := httptest.NewServer(handler)
	defer server.Close()

	ctx := context.Background()
	agent := skybot.NewAgent(ctx, server.URL, "handle", "apikey")
	if err := agent.Connect(ctx); err != nil {
		t.Fatalf("Connect failed: %v", err)
	}

	poster := &BlueskyPoster{skyAgent: &agent}

	tmpl, _ := template.New("test").Parse("{{.Title}}")
	item := &gofeed.Item{
		Title: "Hello Bluesky",
		Link:  "https://example.com/1",
		Extensions: ext.Extensions{
			"media": {
				"content": []ext.Extension{
					{Attrs: map[string]string{"url": imgServer.URL}},
				},
			},
		},
	}

	if _, err := poster.Post(item, tmpl); err != nil {
		t.Fatalf("Post failed: %v", err)
	}

	if !uploadBlobCalled {
		t.Error("expected uploadBlob to be called for the item's feature image")
	}
}

func TestBlueskyPoster_Post_PublishedParsedCreatedAt(t *testing.T) {
	pubDate := time.Date(2026, 9, 29, 19, 45, 0, 0, time.UTC)
	var capturedCreatedAt string

	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if strings.Contains(r.URL.Path, "com.atproto.server.createSession") {
			json.NewEncoder(w).Encode(map[string]any{
				"accessJwt":  "test-access-jwt",
				"refreshJwt": "test-refresh-jwt",
				"handle":     "test-handle",
				"did":        "did:plc:test-did",
			})
			return
		}
		if strings.Contains(r.URL.Path, "com.atproto.repo.createRecord") {
			var body map[string]any
			json.NewDecoder(r.Body).Decode(&body)
			if record, ok := body["record"].(map[string]any); ok {
				if ca, ok := record["createdAt"].(string); ok {
					capturedCreatedAt = ca
				}
			}
			json.NewEncoder(w).Encode(map[string]any{
				"cid": "test-cid",
				"uri": "at://did:plc:test-did/app.bsky.feed.post/test-post-id",
			})
			return
		}
		json.NewEncoder(w).Encode(map[string]any{"cid": "test-cid", "uri": "test-uri"})
	})
	server := httptest.NewServer(handler)
	defer server.Close()

	ctx := context.Background()
	agent := skybot.NewAgent(ctx, server.URL, "handle", "apikey")
	if err := agent.Connect(ctx); err != nil {
		t.Fatalf("Connect failed: %v", err)
	}

	poster := &BlueskyPoster{skyAgent: &agent}

	tmpl, _ := template.New("test").Parse("{{.Title}}")
	item := &gofeed.Item{
		Title:           "Item With PubDate",
		Link:            "https://example.com/item",
		PublishedParsed: &pubDate,
	}

	if _, err := poster.Post(item, tmpl); err != nil {
		t.Fatalf("Post failed: %v", err)
	}

	expected := pubDate.Format(time.RFC3339)
	if capturedCreatedAt != expected {
		t.Errorf("expected createdAt %q, got %q", expected, capturedCreatedAt)
	}
}
