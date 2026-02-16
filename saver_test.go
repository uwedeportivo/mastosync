package main

import (
	"testing"

	"github.com/jomei/notionapi"
)

func TestExtractTitle(t *testing.T) {
	tests := []struct {
		name     string
		content  string
		username string
		expected string
	}{
		{
			name:     "simple text",
			content:  "<p>Hello world this is a test</p>",
			username: "alice",
			expected: "alice: Hello world this is a",
		},
		{
			name:     "short text",
			content:  "<p>Short</p>",
			username: "bob",
			expected: "bob: Short",
		},
		{
			name:     "with links",
			content:  "<p>Visit <a href=\"https://example.com\">Example</a> today!</p>",
			username: "charlie",
			expected: "charlie: Visit Example today !",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			status := &SavedStatus{
				Content: tt.content,
			}
			status.Account.Username = tt.username
			result := ExtractTitle(status)
			if result != tt.expected {
				t.Errorf("ExtractTitle() = %q, want %q", result, tt.expected)
			}
		})
	}
}

func TestConvertHtml2Blocks(t *testing.T) {
	tests := []struct {
		name     string
		content  string
		expected int // number of blocks
	}{
		{
			name:     "single paragraph",
			content:  "<p>Hello Notion</p>",
			expected: 1,
		},
		{
			name:     "two paragraphs",
			content:  "<p>First</p><p>Second</p>",
			expected: 2,
		},
		{
			name:     "paragraph with link",
			content:  "<p>Check <a href=\"https://google.com\">this</a> out.</p>",
			expected: 1,
		},
		{
			name:     "text outside paragraph",
			content:  "Direct text",
			expected: 1,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			blocks := ConvertHtml2Blocks(tt.content)
			if len(blocks) != tt.expected {
				t.Errorf("ConvertHtml2Blocks() returned %d blocks, want %d", len(blocks), tt.expected)
			}
			if len(blocks) > 0 {
				if blocks[0].GetType() != notionapi.BlockTypeParagraph {
					t.Errorf("First block type = %v, want %v", blocks[0].GetType(), notionapi.BlockTypeParagraph)
				}
			}
		})
	}
}
