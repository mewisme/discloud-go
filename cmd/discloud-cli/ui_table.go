package main

import (
	"fmt"
	"io"
	"os"
	"text/tabwriter"
	"time"

	"github.com/mewisme/discloud-go/internal/client"
)

func printFileTable(w io.Writer, files []FileItem) error {
	on := false
	if f, ok := w.(*os.File); ok {
		on = colorOn(f)
	}
	tw := tabwriter.NewWriter(w, 0, 4, 2, ' ', 0)
	fmt.Fprintln(tw, bold(on, "ID")+"\t"+bold(on, "NAME")+"\t"+bold(on, "SIZE")+"\t"+bold(on, "VISIBILITY")+"\t"+bold(on, "EXPIRES"))
	for _, f := range files {
		fmt.Fprintf(tw, "%s\t%s\t%s\t%s\t%s\n",
			cyan(on, f.FileID),
			f.FileName,
			dim(on, client.FormatBytes(f.FileSize)),
			visibilityLabel(on, f.Visibility),
			dim(on, formatTime(f.ExpiresAt)),
		)
	}
	return tw.Flush()
}

func formatTime(t time.Time) string {
	if t.IsZero() {
		return "-"
	}
	return t.UTC().Format(time.RFC3339)
}

func filePickLabel(f FileItem) string {
	on := colorOn(os.Stderr)
	return fmt.Sprintf("%s  %s  %s  %s",
		cyan(on, f.FileID), f.FileName, dim(on, client.FormatBytes(f.FileSize)), visibilityLabel(on, f.Visibility))
}
