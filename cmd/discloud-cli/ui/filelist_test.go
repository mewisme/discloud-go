package ui

import "testing"

func TestToggleSelectAllVisible(t *testing.T) {
	t.Parallel()
	items := []UploadFile{
		{Path: "/a"}, {Path: "/b"}, {Path: "/c"},
	}
	_ = items
	sel := map[string]bool{"/a": true, "/b": false, "/c": false}
	vis := []string{"/a", "/b"}

	got := ToggleSelectAllVisible(items, sel, vis)
	if !got["/a"] || !got["/b"] || got["/c"] {
		t.Fatalf("select all visible: %v", got)
	}
	got = ToggleSelectAllVisible(items, got, vis)
	if got["/a"] || got["/b"] || got["/c"] {
		t.Fatalf("deselect all visible: %v", got)
	}
}
