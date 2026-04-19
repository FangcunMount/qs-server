package progress

import (
	"fmt"
	"os"
	"strings"
	"sync"

	"github.com/mattn/go-isatty"
)

const barWidth = 24

type Bar struct {
	mu          sync.Mutex
	label       string
	total       int
	current     int
	tty         bool
	closed      bool
	lastLineLen int
	lastPrinted int
}

func New(label string, total int) *Bar {
	if total <= 0 {
		return nil
	}
	bar := &Bar{
		label: strings.TrimSpace(label),
		total: total,
		tty:   isatty.IsTerminal(os.Stderr.Fd()) || isatty.IsCygwinTerminal(os.Stderr.Fd()),
	}
	if bar.label == "" {
		bar.label = "progress"
	}
	if bar.tty {
		bar.renderLocked(false)
	}
	return bar
}

func (b *Bar) Increment() {
	b.Add(1)
}

func (b *Bar) Add(delta int) {
	if b == nil || delta <= 0 {
		return
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.closed {
		return
	}
	b.current += delta
	if b.current > b.total {
		b.current = b.total
	}
	b.renderLocked(false)
}

func (b *Bar) Complete() {
	if b == nil {
		return
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.closed {
		return
	}
	b.current = b.total
	b.renderLocked(true)
}

func (b *Bar) Close() {
	if b == nil {
		return
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.closed {
		return
	}
	if b.tty {
		fmt.Fprintln(os.Stderr)
	}
	b.closed = true
}

func (b *Bar) renderLocked(force bool) {
	if b.total <= 0 {
		return
	}
	if b.current < 0 {
		b.current = 0
	}
	if b.current > b.total {
		b.current = b.total
	}

	percent := b.current * 100 / b.total
	if b.tty {
		filled := b.current * barWidth / b.total
		if filled > barWidth {
			filled = barWidth
		}
		line := fmt.Sprintf(
			"%s [%s] %3d%% (%d/%d)",
			b.label,
			strings.Repeat("#", filled)+strings.Repeat("-", barWidth-filled),
			percent,
			b.current,
			b.total,
		)
		padding := ""
		if b.lastLineLen > len(line) {
			padding = strings.Repeat(" ", b.lastLineLen-len(line))
		}
		fmt.Fprintf(os.Stderr, "\r%s%s", line, padding)
		b.lastLineLen = len(line)
		if force {
			fmt.Fprintln(os.Stderr)
			b.closed = true
		}
		return
	}

	if !force {
		step := b.total / 20
		if step < 1 {
			step = 1
		}
		if b.current != 1 && b.current != b.total && b.current-b.lastPrinted < step {
			return
		}
	}

	fmt.Fprintf(os.Stderr, "%s %d/%d (%d%%)\n", b.label, b.current, b.total, percent)
	b.lastPrinted = b.current
	if force {
		b.closed = true
	}
}
