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
