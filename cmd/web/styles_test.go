package web

import (
	"os"
	"strings"
	"testing"
)

func TestSVGTextColorIsControlledByAppStyles(t *testing.T) {
	css, err := os.ReadFile("styles/input.css")
	if err != nil {
		t.Fatalf("read stylesheet: %v", err)
	}
	style := string(css)
	if !strings.Contains(style, ".labbit-svg svg text") || !strings.Contains(style, "fill: currentColor !important") {
		t.Fatalf("SVG text color override is missing")
	}
}

func TestThemeStylesArePresent(t *testing.T) {
	css, err := os.ReadFile("styles/input.css")
	if err != nil {
		t.Fatalf("read stylesheet: %v", err)
	}
	style := string(css)
	for _, want := range []string{
		`html[data-theme="light"]`,
		`--svg-text:`,
		`.theme-toggle`,
		`.sidebar-bottom`,
		`html[data-theme="dark"] .theme-toggle-sun`,
		`html[data-theme="light"] .theme-toggle-moon`,
	} {
		if !strings.Contains(style, want) {
			t.Fatalf("theme style %q is missing", want)
		}
	}
}
