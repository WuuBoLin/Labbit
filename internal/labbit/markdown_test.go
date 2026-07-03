// Copyright (C) 2026 WuuBoLin
// SPDX-License-Identifier: GPL-3.0-or-later

package labbit

import (
	"net/url"
	"strings"
	"testing"
)

func TestRenderMarkdownInlineCode(t *testing.T) {
	html := RenderMarkdown("Edit `group_vars/web.yml` for the paste key.")
	if !strings.Contains(html, `<code class="inline-code">group_vars/web.yml</code>`) {
		t.Fatalf("inline code was not rendered: %s", html)
	}
}

func TestRenderMarkdownInlineCodeEscapesContent(t *testing.T) {
	html := RenderMarkdown("Use `<secret>&value` safely.")
	if !strings.Contains(html, `&lt;secret&gt;&amp;value`) {
		t.Fatalf("inline code content was not escaped: %s", html)
	}
}

func TestRenderMarkdownTable(t *testing.T) {
	html := RenderMarkdown(`
| Service | Port |
| --- | --- |
| SSH | 22 |
| HTTPS | 443 |
`)
	if !strings.Contains(html, `<table class="labbit-table">`) {
		t.Fatalf("table was not rendered: %s", html)
	}
	if !strings.Contains(html, `<th>Service</th>`) || !strings.Contains(html, `<td>443</td>`) {
		t.Fatalf("table cells were not rendered: %s", html)
	}
}

func TestRenderMarkdownTableEscapesInlineContent(t *testing.T) {
	html := RenderMarkdown(`
| Path | Value |
| --- | --- |
| ` + "`/etc/app.yml`" + ` | <secret>&value |
`)
	if !strings.Contains(html, `<code class="inline-code">/etc/app.yml</code>`) {
		t.Fatalf("inline code was not rendered in table: %s", html)
	}
	if !strings.Contains(html, `&lt;secret&gt;&amp;value`) {
		t.Fatalf("table cell content was not escaped: %s", html)
	}
}

func TestRenderMarkdownTableAlignment(t *testing.T) {
	html := RenderMarkdown(`
| Left | Center | Right |
| --- | :---: | ---: |
| a | b | c |
`)
	if !strings.Contains(html, `<th class="align-center">Center</th>`) {
		t.Fatalf("center alignment was not rendered: %s", html)
	}
	if !strings.Contains(html, `<td class="align-right">c</td>`) {
		t.Fatalf("right alignment was not rendered: %s", html)
	}
}

func TestRenderMarkdownPipeTextWithoutSeparatorIsParagraph(t *testing.T) {
	html := RenderMarkdown("Use SSH | HTTPS as examples.")
	if strings.Contains(html, "labbit-table") {
		t.Fatalf("plain pipe text rendered as table: %s", html)
	}
	if !strings.Contains(html, `<p>Use SSH | HTTPS as examples.</p>`) {
		t.Fatalf("plain pipe text was not rendered as paragraph: %s", html)
	}
}

func TestRenderMarkdownSVGImageSanitizesContent(t *testing.T) {
	values := url.Values{}
	values.Set("type", "svg")
	values.Set("alt", `Network <graph>`)
	values.Set("body", `<svg viewBox="0 0 100 50" onclick="alert(1)"><script>alert(1)</script><rect x="5" y="5" width="90" height="40" fill="#18181b"/><text x="50" y="30">Web</text></svg>`)
	html := RenderMarkdown(imageMarker + values.Encode())
	if !strings.Contains(html, `<figure class="labbit-image labbit-svg" role="img" aria-label="Network &lt;graph&gt;">`) {
		t.Fatalf("svg image figure was not rendered with escaped alt text: %s", html)
	}
	if !strings.Contains(html, `<rect x="5" y="5" width="90" height="40" fill="#18181b">`) {
		t.Fatalf("safe svg content was not preserved: %s", html)
	}
	if strings.Contains(html, "script") || strings.Contains(html, "onclick") {
		t.Fatalf("unsafe svg content was not removed: %s", html)
	}
}

func TestRenderMarkdownSVGImagePreservesSafeStyleClasses(t *testing.T) {
	values := url.Values{}
	values.Set("type", "svg")
	values.Set("alt", "Styled topology")
	values.Set("body", `<svg viewBox="0 0 100 50"><defs><marker id="arrow" viewBox="0 0 10 10" refX="9" refY="5" markerWidth="7" markerHeight="7" orient="auto"><path d="M 0 0 L 10 5 L 0 10 z" fill="#1d9bf0"/></marker><style>.box > .label { fill:#e4e4e7; } .box { fill:#18181b; stroke:#1d9bf0; } .txt { fill:#e4e4e7; font-size:12px; } .link { stroke:#1d9bf0; marker-end:url(#arrow); }</style></defs><rect x="5" y="5" width="90" height="40" class="box"/><line x1="5" y1="5" x2="90" y2="40" class="link"/><text x="50" y="30" text-anchor="middle" class="txt" style="font-family:Arial, sans-serif; fill:#e4e4e7">SW1</text></svg>`)
	html := RenderMarkdown(imageMarker + values.Encode())
	if !strings.Contains(html, `.box > .label`) {
		t.Fatalf("safe svg style CSS was escaped or removed: %s", html)
	}
	if !strings.Contains(html, `.link { stroke:#1d9bf0; marker-end:url(#arrow); }`) {
		t.Fatalf("safe svg style block was not preserved: %s", html)
	}
	if !strings.Contains(html, `markerWidth="7"`) {
		t.Fatalf("svg marker attributes were not preserved: %s", html)
	}
	if !strings.Contains(html, `<rect x="5" y="5" width="90" height="40" class="box">`) {
		t.Fatalf("svg class attribute was not preserved: %s", html)
	}
	if !strings.Contains(html, `style="font-family:Arial, sans-serif; fill:#e4e4e7"`) {
		t.Fatalf("safe svg style attribute was not preserved: %s", html)
	}
}

func TestRenderMarkdownSVGImageDropsUnsafeStyle(t *testing.T) {
	values := url.Values{}
	values.Set("type", "svg")
	values.Set("alt", "Unsafe style")
	values.Set("body", `<svg viewBox="0 0 100 50"><style>.box { fill:url(https://example.com/x); }</style><rect x="5" y="5" width="90" height="40" class="box"/></svg>`)
	html := RenderMarkdown(imageMarker + values.Encode())
	if strings.Contains(html, "url(") {
		t.Fatalf("unsafe svg style was not removed: %s", html)
	}
}

func TestRenderMarkdownBase64Image(t *testing.T) {
	values := url.Values{}
	values.Set("type", "png")
	values.Set("alt", "Tiny image")
	values.Set("body", "iVBORw0KGgo=")
	html := RenderMarkdown(imageMarker + values.Encode())
	if !strings.Contains(html, `<img src="data:image/png;base64,iVBORw0KGgo=" alt="Tiny image">`) {
		t.Fatalf("base64 image was not rendered: %s", html)
	}
}

func TestRenderMarkdownUnsupportedImageTypeIsDropped(t *testing.T) {
	values := url.Values{}
	values.Set("type", "bmp")
	values.Set("alt", "Bitmap")
	values.Set("body", "iVBORw0KGgo=")
	html := RenderMarkdown(imageMarker + values.Encode())
	if strings.Contains(html, "labbit-image") {
		t.Fatalf("unsupported image type was rendered: %s", html)
	}
}
