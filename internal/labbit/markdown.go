// Copyright (C) 2026 WuuBoLin
// SPDX-License-Identifier: GPL-3.0-or-later

package labbit

import (
	"bytes"
	"html"
	"net/url"
	"regexp"
	"strings"

	chromahtml "github.com/alecthomas/chroma/v2/formatters/html"
	"github.com/alecthomas/chroma/v2/lexers"
	"github.com/alecthomas/chroma/v2/styles"
)

var inlineCodeRE = regexp.MustCompile("`([^`]+)`")

const (
	collapseStartMarker = ":::labbit-collapse "
	collapseEndMarker   = ":::labbit-endcollapse"
)

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

	for i := 0; i < len(lines); i++ {
		raw := lines[i]
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
		if strings.HasPrefix(trim, collapseStartMarker) {
			closeLists()
			title := strings.TrimSpace(strings.TrimPrefix(trim, collapseStartMarker))
			if decoded, err := url.QueryUnescape(title); err == nil {
				title = decoded
			}
			var body []string
			for i+1 < len(lines) {
				i++
				next := strings.TrimSpace(strings.TrimRight(lines[i], "\r"))
				if next == collapseEndMarker {
					break
				}
				body = append(body, lines[i])
			}
			out.WriteString(renderCollapse(title, strings.Join(body, "\n")))
			continue
		}
		if strings.HasPrefix(trim, `<button class="inline-answer-toggle"`) {
			closeLists()
			out.WriteString(trim)
			continue
		}
		if header, align, ok := tableStart(lines, i); ok {
			closeLists()
			var rows [][]string
			i += 2
			for i < len(lines) {
				cells, ok := tableRow(lines[i])
				if !ok {
					i--
					break
				}
				rows = append(rows, cells)
				i++
			}
			if i >= len(lines) {
				i = len(lines) - 1
			}
			out.WriteString(renderTable(header, align, rows))
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

func renderCollapse(title, body string) string {
	if strings.TrimSpace(title) == "" {
		title = "Details"
	}
	return `<details class="labbit-collapse"><summary>` + inline(title) + `</summary><div class="labbit-collapse-body">` + RenderMarkdown(body) + `</div></details>`
}

func tableStart(lines []string, i int) ([]string, []string, bool) {
	if i+1 >= len(lines) {
		return nil, nil, false
	}
	header, ok := tableRow(lines[i])
	if !ok {
		return nil, nil, false
	}
	separator, ok := tableRow(lines[i+1])
	if !ok || len(separator) != len(header) {
		return nil, nil, false
	}
	align := make([]string, len(separator))
	for i, cell := range separator {
		value := strings.TrimSpace(cell)
		if len(value) < 3 {
			return nil, nil, false
		}
		left := strings.HasPrefix(value, ":")
		right := strings.HasSuffix(value, ":")
		trimmed := strings.Trim(value, ":")
		if len(trimmed) < 3 || strings.Trim(trimmed, "-") != "" {
			return nil, nil, false
		}
		switch {
		case left && right:
			align[i] = "center"
		case right:
			align[i] = "right"
		default:
			align[i] = "left"
		}
	}
	return header, align, true
}

func tableRow(line string) ([]string, bool) {
	trim := strings.TrimSpace(strings.TrimRight(line, "\r"))
	if trim == "" || !strings.Contains(trim, "|") {
		return nil, false
	}
	if strings.HasPrefix(trim, "|") {
		trim = strings.TrimPrefix(trim, "|")
	}
	if strings.HasSuffix(trim, "|") {
		trim = strings.TrimSuffix(trim, "|")
	}
	parts := strings.Split(trim, "|")
	if len(parts) < 2 {
		return nil, false
	}
	cells := make([]string, len(parts))
	for i, part := range parts {
		cells[i] = strings.TrimSpace(part)
	}
	return cells, true
}

func renderTable(header, align []string, rows [][]string) string {
	var out strings.Builder
	out.WriteString(`<div class="labbit-table-wrap"><table class="labbit-table"><thead><tr>`)
	for i, cell := range header {
		out.WriteString(`<th` + tableAlignAttr(align, i) + `>` + inline(cell) + `</th>`)
	}
	out.WriteString(`</tr></thead><tbody>`)
	for _, row := range rows {
		out.WriteString(`<tr>`)
		for i := range header {
			cell := ""
			if i < len(row) {
				cell = row[i]
			}
			out.WriteString(`<td` + tableAlignAttr(align, i) + `>` + inline(cell) + `</td>`)
		}
		out.WriteString(`</tr>`)
	}
	out.WriteString(`</tbody></table></div>`)
	return out.String()
}

func tableAlignAttr(align []string, i int) string {
	if i >= len(align) || align[i] == "" || align[i] == "left" {
		return ""
	}
	return ` class="align-` + align[i] + `"`
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
