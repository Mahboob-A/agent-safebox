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
}

// New constructs a new Tracer with default output directed to os.Stderr.
func New(enabled bool) *Tracer {
	return &Tracer{
		Enabled: enabled,
		Out:     os.Stderr,
	}
}

// Step executes fn and, if tracing is enabled, records its elapsed duration
// and renders its status to Out in the standard [safebox] trace format.
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

	fmt.Fprintf(out, "[safebox] %-28s %s  %v\n", name, status, elapsed)
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

	fmt.Fprintf(out, "[safebox] %-28s %s  %v\n", name, status, elapsed)
}
