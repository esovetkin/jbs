package run

import (
	"fmt"
	"io"
	"math"
	"time"

	"github.com/schollz/progressbar/v3"
	"golang.org/x/term"
)

type ProgressSnapshot struct {
	Total       int
	NotStarted  int
	Running     int
	Finished    int
	Error       int
	Blocked     int
	Interrupted int
}

func (s ProgressSnapshot) Done() int {
	return s.Finished + s.Error + s.Blocked + s.Interrupted
}

type ProgressMode int

const (
	ProgressAuto ProgressMode = iota
	ProgressBar
	ProgressLines
	ProgressSilent
)

type ProgressOptions struct {
	Mode     ProgressMode
	Width    int
	Throttle time.Duration
}

type Progress struct {
	w        io.Writer
	mode     ProgressMode
	width    int
	throttle time.Duration
	last     ProgressSnapshot
	seen     bool
	bar      *progressbar.ProgressBar
}

func NewProgress(w io.Writer) *Progress {
	return NewProgressWithOptions(w, ProgressOptions{
		Mode:     ProgressAuto,
		Width:    32,
		Throttle: 100 * time.Millisecond,
	})
}

func NewProgressWithOptions(w io.Writer, opts ProgressOptions) *Progress {
	if w == nil || opts.Mode == ProgressSilent {
		return &Progress{mode: ProgressSilent}
	}
	if opts.Width <= 0 {
		opts.Width = 32
	}
	mode := opts.Mode
	if mode == ProgressAuto {
		if isTerminalWriter(w) {
			mode = ProgressBar
		} else {
			mode = ProgressLines
		}
	}
	return &Progress{w: w, mode: mode, width: opts.Width, throttle: opts.Throttle}
}

func (p *Progress) Update(s ProgressSnapshot) {
	if p == nil || p.w == nil {
		return
	}
	if p.seen && s == p.last {
		return
	}
	p.seen = true
	p.last = s
	switch p.mode {
	case ProgressBar:
		p.updateBar(s)
	case ProgressLines:
		p.updateLine(s)
	}
}

func (p *Progress) Close(final Status) {
	if p == nil || p.bar == nil {
		return
	}
	if final == StatusFinished || p.last.Done() == p.last.Total {
		_ = p.bar.Set(p.last.Total)
	}
	_ = p.bar.Exit()
	fmt.Fprintln(p.w)
}

func (p *Progress) updateBar(s ProgressSnapshot) {
	p.ensureBar(s.Total)
	p.bar.Describe(progressSuffix(s))
	_ = p.bar.Set(s.Done())
}

func (p *Progress) ensureBar(total int) {
	if p.bar != nil {
		return
	}
	p.bar = progressbar.NewOptions(total,
		progressbar.OptionSetWriter(p.w),
		progressbar.OptionSetWidth(p.width),
		progressbar.OptionShowCount(),
		progressbar.OptionShowIts(),
		progressbar.OptionSetItsString("it"),
		progressbar.OptionSetPredictTime(false),
		progressbar.OptionSetRenderBlankState(true),
		progressbar.OptionShowDescriptionAtLineEnd(),
		progressbar.OptionThrottle(p.throttle),
		progressbar.OptionSetTheme(progressbar.Theme{
			Saucer:        "█",
			SaucerHead:    "█",
			SaucerPadding: " ",
			BarStart:      "|",
			BarEnd:        "|",
		}),
	)
}

func (p *Progress) updateLine(s ProgressSnapshot) {
	pct := 0
	if s.Total > 0 {
		pct = int(math.Round(float64(s.Done()) * 100 / float64(s.Total)))
	}
	detail := fmt.Sprintf("%d finished", s.Finished)
	if s.Blocked > 0 {
		detail += fmt.Sprintf(", %d blocked", s.Blocked)
	}
	if s.Interrupted > 0 {
		detail += fmt.Sprintf(", %d interrupted", s.Interrupted)
	}
	fmt.Fprintf(p.w, "%d%% (%d/%d) %s, %s\n", pct, s.Done(), s.Total, progressSuffix(s), detail)
}

func progressSuffix(s ProgressSnapshot) string {
	suffix := fmt.Sprintf("%dR|%dE", s.Running, s.Error)
	if s.Blocked > 0 {
		suffix += fmt.Sprintf("|%dB", s.Blocked)
	}
	if s.Interrupted > 0 {
		suffix += fmt.Sprintf("|%dI", s.Interrupted)
	}
	return suffix
}

type fdWriter interface {
	Fd() uintptr
}

func isTerminalWriter(w io.Writer) bool {
	f, ok := w.(fdWriter)
	if !ok {
		return false
	}
	return term.IsTerminal(int(f.Fd()))
}
