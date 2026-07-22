package ui

import (
	"fmt"
	"io"
	"os"
	"sync"

	"github.com/mewisme/discloud-go/internal/client"
)

// ProgressBar writes a race-safe progress line to stderr.
type ProgressBar struct {
	mu     sync.Mutex
	w      io.Writer
	width  int
	last   int64
	total  int64
	closed bool
}

// NewProgressBar creates a progress bar writing to w (stderr if nil).
func NewProgressBar(w io.Writer) *ProgressBar {
	if w == nil {
		w = os.Stderr
	}
	return &ProgressBar{w: w, width: 20}
}

// Update advances the bar to sent/total (monotonic on sent).
func (p *ProgressBar) Update(sent, total int64) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.closed {
		return
	}
	if total > 0 {
		p.total = total
	}
	if sent > p.last {
		p.last = sent
	}
	p.renderLocked()
}

// Finish draws 100% and closes the bar.
func (p *ProgressBar) Finish() {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.closed {
		return
	}
	if p.total > 0 {
		p.last = p.total
	}
	p.renderLocked()
	fmt.Fprintln(p.w)
	p.closed = true
}

func (p *ProgressBar) renderLocked() {
	total := p.total
	sent := p.last
	pct := 0.0
	filled := 0
	if total > 0 {
		pct = float64(sent) * 100 / float64(total)
		if pct > 100 {
			pct = 100
		}
		filled = int(float64(p.width) * float64(sent) / float64(total))
		if filled > p.width {
			filled = p.width
		}
		if sent >= total {
			filled = p.width
			pct = 100
		}
	}
	bar := make([]rune, p.width)
	for i := 0; i < p.width; i++ {
		if i < filled {
			bar[i] = '█'
		} else {
			bar[i] = '░'
		}
	}
	on := false
	if f, ok := p.w.(*os.File); ok {
		on = ColorOn(f)
	}
	body := fmt.Sprintf("[%s] %3.0f%% %s / %s",
		string(bar), pct, client.FormatBytes(sent), client.FormatBytes(total))
	if pct >= 100 {
		body = Green(on, body)
	} else {
		body = Cyan(on, body)
	}
	fmt.Fprintf(p.w, "\r%s", body)
}
