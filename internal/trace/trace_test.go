package trace

import (
	"bytes"
	"errors"
	"strings"
	"testing"
	"time"
)

func TestTracerStepEnabled(t *testing.T) {
	var buf bytes.Buffer
	tr := New(true)
	tr.Out = &buf

	err := tr.Step("session setup", func() error {
		time.Sleep(2 * time.Millisecond)
		return nil
	})
	if err != nil {
		t.Fatalf("expected nil error, got: %v", err)
	}

	out := buf.String()
	if !strings.Contains(out, "[safebox]") {
		t.Errorf("expected [safebox] prefix, got: %s", out)
	}
	if !strings.Contains(out, "session setup") {
		t.Errorf("expected step name 'session setup', got: %s", out)
	}
	if !strings.Contains(out, "ok") {
		t.Errorf("expected 'ok' status, got: %s", out)
	}
}

func TestTracerStepFailure(t *testing.T) {
	var buf bytes.Buffer
	tr := New(true)
	tr.Out = &buf

	expectedErr := errors.New("denial error")
	err := tr.Step("landlock apply", func() error {
		return expectedErr
	})
	if !errors.Is(err, expectedErr) {
		t.Fatalf("expected original error %v, got: %v", expectedErr, err)
	}

	out := buf.String()
	if !strings.Contains(out, "[safebox]") {
		t.Errorf("expected [safebox] prefix, got: %s", out)
	}
	if !strings.Contains(out, "landlock apply") {
		t.Errorf("expected step name 'landlock apply', got: %s", out)
	}
	if !strings.Contains(out, "DENIED") {
		t.Errorf("expected 'DENIED' status, got: %s", out)
	}
}

func TestTracerStepDisabled(t *testing.T) {
	var buf bytes.Buffer
	tr := New(false)
	tr.Out = &buf

	executed := false
	err := tr.Step("silent step", func() error {
		executed = true
		return nil
	})
	if err != nil {
		t.Fatalf("expected nil error, got: %v", err)
	}
	if !executed {
		t.Fatal("expected function to execute even when tracer is disabled")
	}
	if buf.Len() != 0 {
		t.Errorf("expected no output when tracer is disabled, got: %s", buf.String())
	}
}

func TestTracerNilSafety(t *testing.T) {
	var tr *Tracer
	executed := false
	err := tr.Step("nil step", func() error {
		executed = true
		return nil
	})
	if err != nil {
		t.Fatalf("expected nil error, got: %v", err)
	}
	if !executed {
		t.Fatal("expected function to execute on nil tracer")
	}

	tr.Log("nil log", nil, 10*time.Millisecond)
}

func TestTracerLog(t *testing.T) {
	var buf bytes.Buffer
	tr := New(true)
	tr.Out = &buf

	tr.Log("overlay mount", nil, 5*time.Millisecond)
	out := buf.String()
	if !strings.Contains(out, "[safebox]") || !strings.Contains(out, "overlay mount") || !strings.Contains(out, "ok") {
		t.Errorf("expected formatted log line, got: %s", out)
	}
}
