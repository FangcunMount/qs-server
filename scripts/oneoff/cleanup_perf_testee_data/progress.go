package main

import (
	"fmt"
	"io"
	"log"
	"os"
	"strings"
	"sync"
	"time"
)

const progressBarWidth = 32

var prog progressReporter

type progressReporter struct {
	mu        sync.Mutex
	enabled   bool
	tty       bool
	out       io.Writer
	phase     string
	label     string
	current   int64
	total     int64
	started   time.Time
	lastDraw  time.Time
	lastLogAt time.Time
}

func initProgress(disable bool) {
	prog = progressReporter{
		enabled: !disable,
		tty:     isTerminal(os.Stderr),
		out:     os.Stderr,
	}
}

func isTerminal(f *os.File) bool {
	fi, err := f.Stat()
	if err != nil {
		return false
	}
	return (fi.Mode() & os.ModeCharDevice) != 0
}

func (p *progressReporter) Phase(name string) {
	if !p.enabled {
		log.Printf("phase: %s", name)
		return
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	p.phase = name
	p.label = name
	p.current = 0
	p.total = 0
	p.started = time.Now()
	p.lastDraw = time.Time{}
	p.lastLogAt = time.Time{}
	p.renderLocked()
}

func (p *progressReporter) Step(label string, current, total int64) {
	if !p.enabled {
		return
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	p.label = label
	p.current = current
	p.total = total
	if p.started.IsZero() {
		p.started = time.Now()
	}
	p.renderLocked()
}

func (p *progressReporter) Indeterminate(label string) {
	p.Step(label, 0, 0)
}

func (p *progressReporter) Add(delta int64) {
	if !p.enabled {
		return
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	p.current += delta
	p.renderLocked()
}

func (p *progressReporter) Finish(label string, detail string) {
	if !p.enabled {
		msg := label
		if detail != "" {
			msg += ": " + detail
		}
		log.Printf("done: %s", msg)
		return
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	elapsed := formatProgressDuration(time.Since(p.started))
	p.clearLineLocked()
	line := fmt.Sprintf("done: %s (%s)", label, elapsed)
	if detail != "" {
		line += " " + detail
	}
	fmt.Fprintln(p.out, line)
	p.label = ""
	p.current = 0
	p.total = 0
	p.started = time.Time{}
}

func (p *progressReporter) RunStep(label string, index, total int, fn func() error) error {
	if total > 0 {
		p.Step(label, int64(index), int64(total))
	} else {
		p.Indeterminate(label)
	}
	err := fn()
	if err != nil {
		if p.enabled {
			p.mu.Lock()
			p.clearLineLocked()
			fmt.Fprintf(p.out, "failed: %s: %v\n", label, err)
			p.mu.Unlock()
		}
		return err
	}
	if total > 0 && index == total {
		p.Finish(label, "")
	}
	return nil
}

func (p *progressReporter) renderLocked() {
	now := time.Now()
	if p.tty {
		if !p.lastDraw.IsZero() && now.Sub(p.lastDraw) < 100*time.Millisecond {
			return
		}
		p.lastDraw = now
		fmt.Fprintf(p.out, "\r%s", p.buildLineLocked())
		return
	}
	if p.lastLogAt.IsZero() || now.Sub(p.lastLogAt) >= 5*time.Second {
		p.lastLogAt = now
		log.Print(strings.TrimPrefix(p.buildLineLocked(), "\r"))
	}
}

func (p *progressReporter) buildLineLocked() string {
	elapsed := formatProgressDuration(time.Since(p.started))
	title := p.phase
	if p.label != "" && p.label != p.phase {
		title = p.phase + " | " + p.label
	}
	title = truncateProgressText(title, 48)
	if p.total > 0 {
		current := p.current
		if current > p.total {
			current = p.total
		}
		pct := float64(current) / float64(p.total)
		filled := int(pct * progressBarWidth)
		if filled > progressBarWidth {
			filled = progressBarWidth
		}
		bar := strings.Repeat("=", filled) + strings.Repeat("-", progressBarWidth-filled)
		return fmt.Sprintf("%s [%s] %3.0f%% (%d/%d) elapsed=%s", title, bar, pct*100, current, p.total, elapsed)
	}
	if p.current > 0 {
		return fmt.Sprintf("%s ... count=%d elapsed=%s", title, p.current, elapsed)
	}
	return fmt.Sprintf("%s ... elapsed=%s", title, elapsed)
}

func (p *progressReporter) clearLineLocked() {
	if !p.tty {
		return
	}
	fmt.Fprint(p.out, "\r\033[K")
}

func truncateProgressText(s string, max int) string {
	if len(s) <= max {
		return s
	}
	if max <= 3 {
		return s[:max]
	}
	return s[:max-3] + "..."
}

func formatProgressDuration(d time.Duration) string {
	d = d.Round(time.Second)
	if d < time.Minute {
		return d.String()
	}
	m := int(d.Minutes())
	s := int(d.Seconds()) % 60
	return fmt.Sprintf("%dm%02ds", m, s)
}
