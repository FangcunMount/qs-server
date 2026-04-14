package main

import (
	"fmt"
	"os"
	"strings"
	"sync"

	"github.com/mattn/go-isatty"
)

const seedProgressBarWidth = 24

type seedProgressBar struct {
	mu          sync.Mutex
	label       string
	total       int
	current     int
	tty         bool
	closed      bool
	lastLineLen int
	lastPrinted int
}

func newSeedProgressBar(label string, total int) *seedProgressBar {
	if total <= 0 {
		return nil
	}
	bar := &seedProgressBar{
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

func (p *seedProgressBar) Increment() {
	p.Add(1)
}

func (p *seedProgressBar) Add(delta int) {
	if p == nil || delta <= 0 {
		return
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.closed {
		return
	}
	p.current += delta
	if p.current > p.total {
		p.current = p.total
	}
	p.renderLocked(false)
}

func (p *seedProgressBar) Complete() {
	if p == nil {
		return
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.closed {
		return
	}
	p.current = p.total
	p.renderLocked(true)
}

func (p *seedProgressBar) Close() {
	if p == nil {
		return
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.closed {
		return
	}
	if p.tty {
		fmt.Fprintln(os.Stderr)
	}
	p.closed = true
}

func (p *seedProgressBar) renderLocked(force bool) {
	if p.total <= 0 {
		return
	}
	if p.current < 0 {
		p.current = 0
	}
	if p.current > p.total {
		p.current = p.total
	}

	percent := p.current * 100 / p.total
	if p.tty {
		filled := p.current * seedProgressBarWidth / p.total
		if filled > seedProgressBarWidth {
			filled = seedProgressBarWidth
		}
		bar := strings.Repeat("#", filled) + strings.Repeat("-", seedProgressBarWidth-filled)
		line := fmt.Sprintf("%s [%s] %3d%% (%d/%d)", p.label, bar, percent, p.current, p.total)
		padding := ""
		if p.lastLineLen > len(line) {
			padding = strings.Repeat(" ", p.lastLineLen-len(line))
		}
		fmt.Fprintf(os.Stderr, "\r%s%s", line, padding)
		p.lastLineLen = len(line)
		if force {
			fmt.Fprintln(os.Stderr)
			p.closed = true
		}
		return
	}

	if !force {
		step := p.total / 20
		if step < 1 {
			step = 1
		}
		if p.current != 1 && p.current != p.total && p.current-p.lastPrinted < step {
			return
		}
	}

	fmt.Fprintf(os.Stderr, "%s %d/%d (%d%%)\n", p.label, p.current, p.total, percent)
	p.lastPrinted = p.current
	if force {
		p.closed = true
	}
}
