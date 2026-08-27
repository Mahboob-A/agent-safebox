package ui

import (
	"testing"
)

func TestStylesRender(t *testing.T) {
	denied := StyleDenied.Render("DENIED")
	if denied == "" {
		t.Fatal("expected non-empty rendered string for StyleDenied")
	}

	allowed := StyleAllowed.Render("ALLOWED")
	if allowed == "" {
		t.Fatal("expected non-empty rendered string for StyleAllowed")
	}

	meta := StyleMeta.Render("meta")
	if meta == "" {
		t.Fatal("expected non-empty rendered string for StyleMeta")
	}
}
