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
		`.footnotes ol`,
		`.theme-toggle`,
		`.sidebar-bottom`,
		`content-start`,
		`@apply sticky top-0 z-30`,
		`self-start`,
		`absolute left-0 right-0 top-full`,
		`lg:static`,
		`background: var(--page-bg);`,
		`.sidebar-actions`,
		`.sidebar-theme-mobile`,
		`width: 100%`,
		`lg:grid-cols-[var(--sidebar-rail-width)_1px_1fr]`,
		`lg:grid-cols-[var(--sidebar-width)_1px_1fr]`,
		`margin-left: calc((1px - 0.375rem) / 2);`,
		`margin-right: calc((1px - 0.375rem) / 2);`,
		`bg-transparent`,
		`background: transparent;`,
		`.sidebar-resizer::before`,
		`.sidebar-resizer:hover::before`,
		`.sidebar-resizer:hover`,
		`.search-results`,
		`gap-1`,
		`ring-inset`,
		`.sidebar-collapsed .sidebar-inner`,
		`.sidebar-collapsed .sidebar:hover .sidebar-inner`,
		`html[data-theme="dark"] .theme-toggle-sun`,
		`html[data-theme="light"] .theme-toggle-moon`,
	} {
		if !strings.Contains(style, want) {
			t.Fatalf("theme style %q is missing", want)
		}
	}
}

func TestThemeToggleScriptSyncsDuplicateControls(t *testing.T) {
	js, err := os.ReadFile("assets/js/labbit.js")
	if err != nil {
		t.Fatalf("read app script: %v", err)
	}
	script := string(js)
	for _, want := range []string{
		`function updateThemeControls(theme)`,
		`function closeMobileSidebar()`,
		`matchMedia("(min-width: 64rem)")`,
		`root.dataset.mobileSidebarOpen !== "true"`,
		`root.dataset.mobileSidebarOpen = root.classList.contains("sidebar-collapsed") ? "true" : "false"`,
		`closeMobileSidebar();`,
		`.theme-toggle-form`,
		`document.body.addEventListener("labbitThemeChanged"`,
		`updateThemeControls(theme)`,
	} {
		if !strings.Contains(script, want) {
			t.Fatalf("theme sync script %q is missing", want)
		}
	}
}
