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

func TestRenderMarkdownBasicSyntax(t *testing.T) {
	html := RenderMarkdown(`
Heading
=======

#### Checklist

Use **strong**, *emphasis*, and ~~stale~~ text.

> Quote
> - nested item

1. first
2. second

---
`)
	for _, want := range []string{
		`<h1 id="heading">Heading</h1>`,
		`<h4 id="checklist">Checklist</h4>`,
		`<strong>strong</strong>`,
		`<em>emphasis</em>`,
		`<del>stale</del>`,
		`<blockquote>`,
		`<ol class="labbit-ordered">`,
		`<hr />`,
	} {
		if !strings.Contains(html, want) {
			t.Fatalf("expected %q in rendered markdown: %s", want, html)
		}
	}
}

func TestRenderMarkdownExtendedSyntax(t *testing.T) {
	html := RenderMarkdown(`
- [x] done
- [ ] next

Term
: Definition text

Footnote marker.[^n]

[^n]: Footnote body
`)
	for _, want := range []string{
		`<input checked="" disabled="" type="checkbox" />`,
		`<input disabled="" type="checkbox" />`,
		`<dl>`,
		`<dt>Term</dt>`,
		`<dd>Definition text</dd>`,
		`class="footnote-ref"`,
		`Footnote body`,
	} {
		if !strings.Contains(html, want) {
			t.Fatalf("expected %q in rendered markdown: %s", want, html)
		}
	}
}

func TestRenderMarkdownListsUseLabbitClasses(t *testing.T) {
	html := RenderMarkdown(`
- unordered
  - nested unordered

4. ordered
5. next
   1. nested ordered
`)
	for _, want := range []string{
		`<ul class="labbit-list">`,
		`<ol class="labbit-ordered" start="4">`,
		`<li>unordered`,
		`<li>nested unordered</li>`,
		`<li>ordered</li>`,
		`<ol class="labbit-ordered">`,
		`<li>nested ordered</li>`,
	} {
		if !strings.Contains(html, want) {
			t.Fatalf("expected %q in rendered markdown: %s", want, html)
		}
	}
}

func TestRenderMarkdownLinksAndImagesAreConservative(t *testing.T) {
	html := RenderMarkdown(`
[safe](https://example.com)
[fragment](#local)
[bad](javascript:alert(1))
![Remote diagram](https://example.com/diagram.png)
`)
	if !strings.Contains(html, `<a href="https://example.com">safe</a>`) {
		t.Fatalf("safe link was not rendered: %s", html)
	}
	if !strings.Contains(html, `<a href="#local">fragment</a>`) {
		t.Fatalf("fragment link was not rendered: %s", html)
	}
	if strings.Contains(html, `javascript:`) || strings.Contains(html, `<a href="">bad</a>`) {
		t.Fatalf("dangerous link was made active: %s", html)
	}
	if strings.Contains(html, `<img`) {
		t.Fatalf("markdown image tag was rendered: %s", html)
	}
	if !strings.Contains(html, `Remote diagram`) {
		t.Fatalf("markdown image alt text was not preserved: %s", html)
	}
}

func TestRenderMarkdownEscapesRawHTML(t *testing.T) {
	html := RenderMarkdown(`<script>alert(1)</script>`)
	if strings.Contains(html, "<script>") || strings.Contains(html, "raw HTML omitted") {
		t.Fatalf("raw HTML was not escaped cleanly: %s", html)
	}
	if !strings.Contains(html, `&lt;script&gt;alert(1)&lt;/script&gt;`) {
		t.Fatalf("raw HTML content was not preserved as escaped text: %s", html)
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
