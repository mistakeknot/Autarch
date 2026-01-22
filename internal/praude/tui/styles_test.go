package tui

import "testing"

func TestRenderHeaderFooterStyled(t *testing.T) {
	header := renderHeader("LIST", "LIST")
	footer := renderFooter("keys", "ready")
	if header == "" || footer == "" {
		t.Fatalf("expected non-empty header/footer")
	}
}
