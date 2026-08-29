package trace

import (
	"fmt"
	"io"
	"os"
	"time"

	"safebox/internal/ui"
)

// Tracer provides structured, timestamped execution step logging for safebox.
type Tracer struct {
	Enabled bool
	Out     io.Writer
	Process string
}

// New constructs a new Tracer with default output directed to os.Stderr.
func New(enabled bool) *Tracer {
	return &Tracer{
		Enabled: enabled,
		Out:     os.Stderr,
	}
}

// NewWithWriter constructs a new Tracer with custom output destination.
func NewWithWriter(enabled bool, out io.Writer) *Tracer {
	return &Tracer{
		Enabled: enabled,
		Out:     out,
	}
}

// NewChild constructs a new Tracer configured with the child process prefix.
func NewChild(enabled bool) *Tracer {
	return &Tracer{
		Enabled: enabled,
		Out:     os.Stderr,
		Process: "child",
	}
}

// Step executes fn and, if tracing is enabled, records its elapsed duration
// and renders its status to Out in the standard [safebox] or [safebox:child] format.
func (t *Tracer) Step(name string, fn func() error) error {
	if t == nil || !t.Enabled {
		return fn()
	}

	start := time.Now()
	err := fn()
	elapsed := time.Since(start)

	status := ui.StyleAllowed.Render("ok")
	if err != nil {
		status = ui.StyleDenied.Render("DENIED")
	}

	out := t.Out
	if out == nil {
		out = os.Stderr
	}

	prefix := ""
	if t.Process == "child" {
		prefix = ":child"
	}

	fmt.Fprintf(out, "[safebox%s] %-28s %s  %v\n", prefix, name, status, elapsed)
	return err
}

// Log writes a completed step to Out with explicit status and elapsed duration.
func (t *Tracer) Log(name string, err error, elapsed time.Duration) {
	if t == nil || !t.Enabled {
		return
	}

	status := ui.StyleAllowed.Render("ok")
	if err != nil {
		status = ui.StyleDenied.Render("DENIED")
	}

	out := t.Out
	if out == nil {
		out = os.Stderr
	}

	prefix := ""
	if t.Process == "child" {
		prefix = ":child"
	}

	fmt.Fprintf(out, "[safebox%s] %-28s %s  %v\n", prefix, name, status, elapsed)
}
