package main

import (
	"os"
	"path/filepath"
	"testing"
)

func TestRandSeq(t *testing.T) {
	s1 := randSeq(10)
	s2 := randSeq(10)

	if len(s1) != 10 {
		t.Errorf("Expected length 10, got %d", len(s1))
	}
	if s1 == s2 {
		t.Error("Expected different random sequences, got identical ones")
	}
}

func TestTooter_ResolvePath(t *testing.T) {
	tempTootsDir, err := os.MkdirTemp("", "toots")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tempTootsDir)

	tootsPath := filepath.Join(tempTootsDir, "toots.md")
	ttr := &Tooter{
		tootsPath: tootsPath,
	}

	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "relative path",
			input:    "image.jpg",
			expected: filepath.Join(tempTootsDir, "image.jpg"),
		},
		{
			name:     "absolute path",
			input:    "/tmp/abs/path.jpg",
			expected: "/tmp/abs/path.jpg",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := ttr.ResolvePath(tt.input)
			if result != tt.expected {
				t.Errorf("ResolvePath() = %q, want %q", result, tt.expected)
			}
		})
	}
}
