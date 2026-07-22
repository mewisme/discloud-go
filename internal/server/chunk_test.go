package server

import (
	"bytes"
	"strings"
	"testing"
)

func TestForEachChunk(t *testing.T) {
	cases := []struct {
		name      string
		size      int
		chunkSize int
		wantLens  []int
	}{
		{"empty", 0, 8, nil},
		{"smaller than chunk", 5, 8, []int{5}},
		{"exactly one chunk", 8, 8, []int{8}},
		{"one byte over", 9, 8, []int{8, 1}},
		{"multiple exact", 24, 8, []int{8, 8, 8}},
		{"multiple with tail", 20, 8, []int{8, 8, 4}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			input := bytes.Repeat([]byte{'x'}, tc.size)
			var gotLens []int
			var got []byte
			total, err := forEachChunk(bytes.NewReader(input), tc.chunkSize, func(idx int, data []byte) error {
				if idx != len(gotLens) {
					t.Fatalf("chunk index = %d, want %d", idx, len(gotLens))
				}
				gotLens = append(gotLens, len(data))
				got = append(got, data...)
				return nil
			})
			if err != nil {
				t.Fatal(err)
			}
			if total != int64(tc.size) {
				t.Errorf("total = %d, want %d", total, tc.size)
			}
			if len(gotLens) != len(tc.wantLens) {
				t.Fatalf("chunk lens = %v, want %v", gotLens, tc.wantLens)
			}
			for i := range gotLens {
				if gotLens[i] != tc.wantLens[i] {
					t.Errorf("chunk %d len = %d, want %d", i, gotLens[i], tc.wantLens[i])
				}
			}
			if !bytes.Equal(got, input) {
				t.Error("reassembled bytes differ from input")
			}
		})
	}
}

func TestParseRange(t *testing.T) {
	const size, window = 100, 10
	cases := []struct {
		header  string
		want    byteRange
		wantErr bool
	}{
		{"bytes=0-49", byteRange{0, 49}, false},
		{"bytes=50-", byteRange{50, 60}, false},   // open-ended capped by window
		{"bytes=95-", byteRange{95, 99}, false},   // window past EOF clamps
		{"bytes=0-999", byteRange{0, 99}, false},  // end clamps to size-1
		{"bytes=-10", byteRange{90, 99}, false},   // suffix range
		{"bytes=-200", byteRange{0, 99}, false},   // suffix bigger than file
		{"bytes=99-99", byteRange{99, 99}, false}, // single byte
		{"bytes=100-", byteRange{}, true},         // start at EOF
		{"bytes=5-2", byteRange{}, true},          // end before start
		{"bytes=", byteRange{}, true},
		{"bytes=abc-", byteRange{}, true},
		{"items=0-5", byteRange{}, true},
	}
	for _, tc := range cases {
		got, err := parseRange(tc.header, size, window)
		if tc.wantErr != (err != nil) {
			t.Errorf("parseRange(%q) err = %v, wantErr %v", tc.header, err, tc.wantErr)
			continue
		}
		if got != tc.want {
			t.Errorf("parseRange(%q) = %+v, want %+v", tc.header, got, tc.want)
		}
	}
}

func TestPartsForRange(t *testing.T) {
	const chunk = 10
	cases := []struct {
		name string
		r    byteRange
		want []partSpan
	}{
		{"within first part", byteRange{2, 7}, []partSpan{{0, 2, 7}}},
		{"spans two parts", byteRange{5, 14}, []partSpan{{0, 5, 9}, {1, 0, 4}}},
		{"whole middle part", byteRange{10, 19}, []partSpan{{1, 0, 9}}},
		{"three parts", byteRange{9, 21}, []partSpan{{0, 9, 9}, {1, 0, 9}, {2, 0, 1}}},
		{"boundary byte", byteRange{10, 10}, []partSpan{{1, 0, 0}}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := partsForRange(chunk, tc.r)
			if len(got) != len(tc.want) {
				t.Fatalf("got %+v, want %+v", got, tc.want)
			}
			for i := range got {
				if got[i] != tc.want[i] {
					t.Errorf("span %d = %+v, want %+v", i, got[i], tc.want[i])
				}
			}
		})
	}
}

func TestFormatFileName(t *testing.T) {
	cases := map[string]string{
		"My File (1).PDF":  "my-file-1.pdf",
		"hello.tar.gz":     "hello-tar.gz",
		"???":              "file",
		"no-extension":     "no-extension",
		".hidden":          "hidden", // leading dot is not an extension separator
		"already-fine.txt": "already-fine.txt",
	}
	for in, want := range cases {
		if got := formatFileName(in); got != want {
			t.Errorf("formatFileName(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestHumanBytes(t *testing.T) {
	if got := humanBytes(8 << 20); got != "8.00 MB" {
		t.Errorf("humanBytes(8MB) = %q", got)
	}
	if got := humanBytes(512); !strings.HasSuffix(got, " B") {
		t.Errorf("humanBytes(512) = %q", got)
	}
}

func TestSafeDownloadHeaders(t *testing.T) {
	cases := []struct {
		name, wantCT, wantDisp string
		forceDownload          bool
	}{
		{"x.html", "application/octet-stream", "attachment", false},
		{"x.svg", "application/octet-stream", "attachment", false},
		{"x.png", "image/png", "inline", false},
		{"x.png", "image/png", "attachment", true},
		{"x.bin", "application/octet-stream", "attachment", false},
		{"x.pdf", "application/pdf", "inline", false},
	}
	for _, tc := range cases {
		ct, disp := safeDownloadHeaders(tc.name, tc.forceDownload)
		if ct != tc.wantCT || disp != tc.wantDisp {
			t.Errorf("safeDownloadHeaders(%q, %v) = (%q, %q), want (%q, %q)",
				tc.name, tc.forceDownload, ct, disp, tc.wantCT, tc.wantDisp)
		}
	}
}
