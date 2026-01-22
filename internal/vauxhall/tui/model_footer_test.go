package tui

import "testing"

func TestRenderFooterNoPanicOnSmallWidth(t *testing.T) {
	m := Model{}
	m.width = 10
	defer func() {
		if r := recover(); r != nil {
			t.Fatalf("renderFooter panicked: %v", r)
		}
	}()
	_ = m.renderFooter()
}
