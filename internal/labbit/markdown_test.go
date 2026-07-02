// Copyright (C) 2026 WuuBoLin
// SPDX-License-Identifier: GPL-3.0-or-later

package labbit

import (
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
