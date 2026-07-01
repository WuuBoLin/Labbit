// Copyright (C) 2026 WuuBoLin
// SPDX-License-Identifier: GPL-3.0-or-later

package labbit

import (
	"bytes"
	"html"
	"regexp"
	"strings"

	chromahtml "github.com/alecthomas/chroma/v2/formatters/html"
	"github.com/alecthomas/chroma/v2/lexers"
	"github.com/alecthomas/chroma/v2/styles"
)

var inlineCodeRE = regexp.MustCompile("`([^`]+)`")

func RenderMarkdown(src string) string {
	lines := strings.Split(strings.TrimSpace(src), "\n")
	var out strings.Builder
	var inCode bool
	var codeLang string
	var code strings.Builder
	var inUL bool
	var inOL bool

	closeLists := func() {
		if inUL {
			out.WriteString("</ul>")
			inUL = false
		}
		if inOL {
			out.WriteString("</ol>")
			inOL = false
		}
	}

	for _, raw := range lines {
		line := strings.TrimRight(raw, "\r")
		trim := strings.TrimSpace(line)
		if strings.HasPrefix(trim, "```") {
			if inCode {
				out.WriteString(renderCodeBlock(code.String(), codeLang))
				code.Reset()
				codeLang = ""
				inCode = false
			} else {
				closeLists()
				codeLang = strings.TrimSpace(strings.TrimPrefix(trim, "```"))
				inCode = true
			}
			continue
		}
		if inCode {
			code.WriteString(line)
			code.WriteByte('\n')
			continue
		}
		if trim == "" {
			closeLists()
			continue
		}
		if strings.HasPrefix(trim, `<button class="inline-answer-toggle"`) {
			closeLists()
			out.WriteString(trim)
			continue
		}
		if strings.HasPrefix(trim, "### ") {
			closeLists()
			out.WriteString(`<h3 class="mt-6 text-base font-semibold text-zinc-100">` + inline(trim[4:]) + `</h3>`)
			continue
		}
		if strings.HasPrefix(trim, "## ") {
			closeLists()
			out.WriteString(`<h2 class="mt-8 text-xl font-semibold text-zinc-50">` + inline(trim[3:]) + `</h2>`)
			continue
		}
		if strings.HasPrefix(trim, "# ") {
			closeLists()
			out.WriteString(`<h1 class="mt-2 text-2xl font-semibold text-zinc-50">` + inline(trim[2:]) + `</h1>`)
			continue
		}
		if strings.HasPrefix(trim, "- ") || strings.HasPrefix(trim, "* ") {
			if inOL {
				out.WriteString("</ol>")
				inOL = false
			}
			if !inUL {
				out.WriteString(`<ul class="labbit-list">`)
				inUL = true
			}
			out.WriteString(`<li>` + inline(trim[2:]) + `</li>`)
			continue
		}
		if orderedItem(trim) {
			if inUL {
				out.WriteString("</ul>")
				inUL = false
			}
			if !inOL {
				out.WriteString(`<ol class="labbit-ordered">`)
				inOL = true
			}
			item := strings.TrimSpace(trim[strings.Index(trim, ".")+1:])
			out.WriteString(`<li>` + inline(item) + `</li>`)
			continue
		}
		closeLists()
		out.WriteString(`<p>` + inline(trim) + `</p>`)
	}
	if inCode {
		out.WriteString(renderCodeBlock(code.String(), codeLang))
	}
	closeLists()
	return out.String()
}

func inline(s string) string {
	parts := inlineCodeRE.FindAllStringSubmatchIndex(s, -1)
	if len(parts) == 0 {
		return html.EscapeString(strings.TrimSpace(s))
	}
	var out strings.Builder
	last := 0
	for _, part := range parts {
		out.WriteString(html.EscapeString(s[last:part[0]]))
		out.WriteString(`<code class="inline-code">`)
		out.WriteString(html.EscapeString(s[part[2]:part[3]]))
		out.WriteString(`</code>`)
		last = part[1]
	}
	out.WriteString(html.EscapeString(s[last:]))
	return strings.TrimSpace(out.String())
}

func renderCodeBlock(code, lang string) string {
	lang = html.EscapeString(strings.TrimSpace(lang))
	raw := strings.TrimRight(code, "\n")
	return `<div class="code-shell"><div class="code-bar"><span>` + langLabel(lang) + `</span><button class="copy-btn" type="button" data-copy>Copy</button></div><div class="highlighted-code" data-code="` + html.EscapeString(raw) + `">` + highlightCode(raw, lang) + `</div></div>`
}

func highlightCode(code, lang string) string {
	lexer := strings.TrimSpace(lang)
	if lexer == "" || lexer == "code" {
		lexer = "fallback"
	}
	l := lexers.Get(lexer)
	if l == nil {
		l = lexers.Fallback
	}
	iterator, err := l.Tokenise(nil, code)
	if err != nil {
		return `<pre><code>` + html.EscapeString(code) + `</code></pre>`
	}
	var out bytes.Buffer
	formatter := chromahtml.New(chromahtml.Standalone(false), chromahtml.WithClasses(false))
	style := styles.Get("doom-one")
	if style == nil {
		style = styles.Fallback
	}
	if err := formatter.Format(&out, style, iterator); err != nil {
		return `<pre><code>` + html.EscapeString(code) + `</code></pre>`
	}
	return out.String()
}

func langLabel(lang string) string {
	if lang == "" {
		return "code"
	}
	return lang
}

func orderedItem(s string) bool {
	dot := strings.Index(s, ".")
	if dot < 1 {
		return false
	}
	for _, r := range s[:dot] {
		if r < '0' || r > '9' {
			return false
		}
	}
	return strings.TrimSpace(s[dot+1:]) != ""
}
