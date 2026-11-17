package tmux

import "testing"

func TestMatchProject(t *testing.T) {
	tests := []struct {
		name     string
		projects []string
		cwd      string
		want     string
	}{
		{"nested parent first", []string{"/a/proj", "/a/proj/child"}, "/a/proj/child/sub", "/a/proj/child"},
		{"nested child first", []string{"/a/proj/child", "/a/proj"}, "/a/proj/child/sub", "/a/proj/child"},
		{"path boundary", []string{"/a/proj"}, "/a/project/x", ""},
		{"exact project", []string{"/a/proj"}, "/a/proj", "/a/proj"},
		{"project subdirectory", []string{"/a/proj"}, "/a/proj/sub", "/a/proj"},
		{"cleaned paths", []string{"/a/proj/", "/a/proj/child/../child/"}, "/a/proj/child/sub/../", "/a/proj/child"},
		{"root project", []string{"/"}, "/a/project/x", "/"},
		{"no match", []string{"/a/proj"}, "/b/other", ""},
		{"empty cwd", []string{"/a/proj"}, "", ""},
		{"no projects", nil, "/a/proj", ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := NewDetector(tt.projects).matchProject(tt.cwd); got != tt.want {
				t.Fatalf("matchProject(%q) = %q, want %q", tt.cwd, got, tt.want)
			}
		})
	}
}
