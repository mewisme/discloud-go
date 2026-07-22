package main

import "testing"

func TestDecodeFileList(t *testing.T) {
	t.Parallel()
	raw := map[string]any{
		"files": []any{
			map[string]any{
				"fileId":     "abc",
				"fileName":   "x.bin",
				"fileSize":   float64(1024),
				"visibility": "public",
			},
		},
	}
	list, err := decode[FilesList](raw)
	if err != nil {
		t.Fatal(err)
	}
	if len(list.Files) != 1 || list.Files[0].FileID != "abc" {
		t.Fatalf("%+v", list)
	}
}
