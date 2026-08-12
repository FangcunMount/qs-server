package main

import (
	"fmt"
	"io"
	"os"
	"sort"
	"strings"
	"sync"
	"time"
)

const selectorProgressBarWidth = 24

type selectorProgress struct {
	mu        sync.Mutex
	out       io.Writer
	disabled  bool
	startedAt time.Time
	total     int
	completed int
	rows      int
	active    map[string]time.Time
}

func newSelectorProgress(total int, disabled bool) *selectorProgress {
	progress := &selectorProgress{
		out:       os.Stderr,
		disabled:  disabled,
		startedAt: time.Now(),
		total:     total,
		active:    make(map[string]time.Time),
	}
	progress.print("ready")
	return progress
}

func (p *selectorProgress) Start(date string) {
	if p == nil || p.disabled {
		return
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	p.active[date] = time.Now()
	p.printLocked("started=" + date)
}

func (p *selectorProgress) Finish(date string, rows int, duration time.Duration) {
	if p == nil || p.disabled {
		return
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	delete(p.active, date)
	p.completed++
	p.rows += rows
	p.printLocked(fmt.Sprintf("finished=%s shard_rows=%d shard_elapsed=%s", date, rows, roundDuration(duration)))
}

func (p *selectorProgress) Fail(date string, duration time.Duration) {
	if p == nil || p.disabled {
		return
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	delete(p.active, date)
	p.printLocked(fmt.Sprintf("failed=%s shard_elapsed=%s", date, roundDuration(duration)))
}

func (p *selectorProgress) Snapshot() {
	if p == nil || p.disabled {
		return
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	active := make([]string, 0, len(p.active))
	for date, started := range p.active {
		active = append(active, fmt.Sprintf("%s(%s)", date, roundDuration(time.Since(started))))
	}
	sort.Strings(active)
	detail := "waiting"
	if len(active) > 0 {
		detail = "active=" + strings.Join(active, ",")
	}
	p.printLocked(detail)
}

func (p *selectorProgress) print(detail string) {
	if p == nil || p.disabled {
		return
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	p.printLocked(detail)
}

func (p *selectorProgress) printLocked(detail string) {
	completed := p.completed
	if completed > p.total {
		completed = p.total
	}
	filled := 0
	percent := 100.0
	if p.total > 0 {
		filled = completed * selectorProgressBarWidth / p.total
		percent = float64(completed) * 100 / float64(p.total)
	}
	bar := strings.Repeat("#", filled) + strings.Repeat("-", selectorProgressBarWidth-filled)
	_, _ = fmt.Fprintf(
		p.out,
		"[selector] [%s] %d/%d %5.1f%% rows=%d elapsed=%s %s\n",
		bar,
		p.completed,
		p.total,
		percent,
		p.rows,
		roundDuration(time.Since(p.startedAt)),
		detail,
	)
}

func roundDuration(duration time.Duration) time.Duration {
	if duration < time.Second {
		return duration.Round(time.Millisecond)
	}
	return duration.Round(time.Second)
}
