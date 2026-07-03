// Copyright (C) 2026 WuuBoLin
// SPDX-License-Identifier: GPL-3.0-or-later

package labbit

import (
	"bytes"
	"encoding/base64"
	"encoding/xml"
	"net/url"
	"regexp"
	"strings"

	chromahtml "github.com/alecthomas/chroma/v2/formatters/html"
	"github.com/alecthomas/chroma/v2/lexers"
	"github.com/alecthomas/chroma/v2/styles"
	"github.com/yuin/goldmark"
	gast "github.com/yuin/goldmark/ast"
	"github.com/yuin/goldmark/extension"
	east "github.com/yuin/goldmark/extension/ast"
	"github.com/yuin/goldmark/parser"
	"github.com/yuin/goldmark/renderer"
	gmhtml "github.com/yuin/goldmark/renderer/html"
	"github.com/yuin/goldmark/util"
	"html"
)

var inlineCodeRE = regexp.MustCompile("`([^`]+)`")
var svgCSSURLRE = regexp.MustCompile(`(?i)url\(\s*['"]?([^'")\s]+)['"]?\s*\)`)

const (
	collapseStartMarker = ":::labbit-collapse "
	collapseEndMarker   = ":::labbit-endcollapse"
	imageMarker         = ":::labbit-image "
)

func RenderMarkdown(src string) string {
	lines := strings.Split(strings.TrimSpace(src), "\n")
	var out strings.Builder
	var segment []string

	flush := func() {
		if len(segment) == 0 {
			return
		}
		out.WriteString(renderMarkdownSegment(strings.Join(segment, "\n")))
		segment = nil
	}

	for i := 0; i < len(lines); i++ {
		raw := lines[i]
		line := strings.TrimRight(raw, "\r")
		trim := strings.TrimSpace(line)
		if strings.HasPrefix(trim, collapseStartMarker) {
			flush()
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
		if strings.HasPrefix(trim, imageMarker) {
			flush()
			out.WriteString(renderImageMarker(strings.TrimSpace(strings.TrimPrefix(trim, imageMarker))))
			continue
		}
		if strings.HasPrefix(trim, `<button class="inline-hint-toggle"`) {
			flush()
			out.WriteString(trim)
			continue
		}
		segment = append(segment, line)
	}
	flush()
	return out.String()
}

func renderMarkdownSegment(src string) string {
	src = strings.TrimSpace(src)
	if src == "" {
		return ""
	}
	var out bytes.Buffer
	if err := markdownRenderer().Convert([]byte(src), &out); err != nil {
		return `<p>` + html.EscapeString(src) + `</p>`
	}
	return strings.TrimSpace(out.String())
}

func markdownRenderer() goldmark.Markdown {
	return goldmark.New(
		goldmark.WithExtensions(
			extension.GFM,
			extension.DefinitionList,
			extension.Footnote,
		),
		goldmark.WithParserOptions(parser.WithAutoHeadingID()),
		goldmark.WithRendererOptions(
			gmhtml.WithXHTML(),
			renderer.WithNodeRenderers(util.Prioritized(labbitMarkdownRenderer{}, 1)),
		),
	)
}

type labbitMarkdownRenderer struct{}

func (r labbitMarkdownRenderer) RegisterFuncs(reg renderer.NodeRendererFuncRegisterer) {
	reg.Register(gast.KindCodeBlock, r.renderCodeBlock)
	reg.Register(gast.KindCodeSpan, r.renderCodeSpan)
	reg.Register(gast.KindFencedCodeBlock, r.renderFencedCodeBlock)
	reg.Register(gast.KindHTMLBlock, r.renderHTMLBlock)
	reg.Register(gast.KindImage, r.renderImage)
	reg.Register(gast.KindLink, r.renderLink)
	reg.Register(gast.KindAutoLink, r.renderAutoLink)
	reg.Register(gast.KindRawHTML, r.renderRawHTML)
	reg.Register(east.KindTable, r.renderTable)
	reg.Register(east.KindTableHeader, r.renderTableHeader)
	reg.Register(east.KindTableRow, r.renderTableRow)
	reg.Register(east.KindTableCell, r.renderTableCell)
}

func (r labbitMarkdownRenderer) renderCodeBlock(w util.BufWriter, source []byte, node gast.Node, entering bool) (gast.WalkStatus, error) {
	if !entering {
		return gast.WalkContinue, nil
	}
	_, _ = w.WriteString(renderCodeBlock(string(node.Lines().Value(source)), ""))
	return gast.WalkSkipChildren, nil
}

func (r labbitMarkdownRenderer) renderFencedCodeBlock(w util.BufWriter, source []byte, node gast.Node, entering bool) (gast.WalkStatus, error) {
	if !entering {
		return gast.WalkContinue, nil
	}
	block := node.(*gast.FencedCodeBlock)
	_, _ = w.WriteString(renderCodeBlock(string(block.Lines().Value(source)), string(block.Language(source))))
	return gast.WalkSkipChildren, nil
}

func (r labbitMarkdownRenderer) renderCodeSpan(w util.BufWriter, source []byte, node gast.Node, entering bool) (gast.WalkStatus, error) {
	if !entering {
		_, _ = w.WriteString("</code>")
		return gast.WalkContinue, nil
	}
	_, _ = w.WriteString(`<code class="inline-code">`)
	for child := node.FirstChild(); child != nil; child = child.NextSibling() {
		text := child.(*gast.Text).Segment.Value(source)
		if bytes.HasSuffix(text, []byte("\n")) {
			text = append(text[:len(text)-1], ' ')
		}
		_, _ = w.Write(util.EscapeHTML(text))
	}
	return gast.WalkSkipChildren, nil
}

func (r labbitMarkdownRenderer) renderHTMLBlock(w util.BufWriter, source []byte, node gast.Node, entering bool) (gast.WalkStatus, error) {
	if !entering {
		return gast.WalkContinue, nil
	}
	_, _ = w.Write(util.EscapeHTML(node.Text(source)))
	return gast.WalkSkipChildren, nil
}

func (r labbitMarkdownRenderer) renderImage(w util.BufWriter, source []byte, node gast.Node, entering bool) (gast.WalkStatus, error) {
	if !entering {
		return gast.WalkContinue, nil
	}
	image := node.(*gast.Image)
	if len(image.Text(source)) > 0 {
		_, _ = w.Write(util.EscapeHTML(image.Text(source)))
	}
	return gast.WalkSkipChildren, nil
}

func (r labbitMarkdownRenderer) renderLink(w util.BufWriter, source []byte, node gast.Node, entering bool) (gast.WalkStatus, error) {
	link := node.(*gast.Link)
	if gmhtml.IsDangerousURL(link.Destination) {
		return gast.WalkContinue, nil
	}
	if entering {
		_, _ = w.WriteString(`<a href="`)
		_, _ = w.Write(util.EscapeHTML(util.URLEscape(link.Destination, false)))
		_, _ = w.WriteString(`">`)
	} else {
		_, _ = w.WriteString(`</a>`)
	}
	return gast.WalkContinue, nil
}

func (r labbitMarkdownRenderer) renderAutoLink(w util.BufWriter, source []byte, node gast.Node, entering bool) (gast.WalkStatus, error) {
	if !entering {
		return gast.WalkContinue, nil
	}
	link := node.(*gast.AutoLink)
	destination := link.URL(source)
	if link.AutoLinkType == gast.AutoLinkEmail && !bytes.HasPrefix(bytes.ToLower(destination), []byte("mailto:")) {
		destination = append([]byte("mailto:"), destination...)
	}
	if gmhtml.IsDangerousURL(destination) {
		_, _ = w.Write(util.EscapeHTML(link.Label(source)))
		return gast.WalkSkipChildren, nil
	}
	_, _ = w.WriteString(`<a href="`)
	_, _ = w.Write(util.EscapeHTML(util.URLEscape(destination, false)))
	_, _ = w.WriteString(`">`)
	_, _ = w.Write(util.EscapeHTML(link.Label(source)))
	_, _ = w.WriteString(`</a>`)
	return gast.WalkSkipChildren, nil
}

func (r labbitMarkdownRenderer) renderRawHTML(w util.BufWriter, source []byte, node gast.Node, entering bool) (gast.WalkStatus, error) {
	if !entering {
		return gast.WalkContinue, nil
	}
	_, _ = w.Write(util.EscapeHTML(node.Text(source)))
	return gast.WalkSkipChildren, nil
}

func (r labbitMarkdownRenderer) renderTable(w util.BufWriter, source []byte, node gast.Node, entering bool) (gast.WalkStatus, error) {
	if entering {
		_, _ = w.WriteString(`<div class="labbit-table-wrap"><table class="labbit-table">`)
	} else {
		_, _ = w.WriteString(`</table></div>`)
	}
	return gast.WalkContinue, nil
}

func (r labbitMarkdownRenderer) renderTableHeader(w util.BufWriter, source []byte, node gast.Node, entering bool) (gast.WalkStatus, error) {
	if entering {
		_, _ = w.WriteString("<thead><tr>")
	} else {
		_, _ = w.WriteString("</tr></thead>")
		if node.NextSibling() != nil {
			_, _ = w.WriteString("<tbody>")
		}
	}
	return gast.WalkContinue, nil
}

func (r labbitMarkdownRenderer) renderTableRow(w util.BufWriter, source []byte, node gast.Node, entering bool) (gast.WalkStatus, error) {
	if entering {
		_, _ = w.WriteString("<tr>")
	} else {
		_, _ = w.WriteString("</tr>")
		if node.Parent().LastChild() == node {
			_, _ = w.WriteString("</tbody>")
		}
	}
	return gast.WalkContinue, nil
}

func (r labbitMarkdownRenderer) renderTableCell(w util.BufWriter, source []byte, node gast.Node, entering bool) (gast.WalkStatus, error) {
	cell := node.(*east.TableCell)
	tag := "td"
	if node.Parent().Kind() == east.KindTableHeader {
		tag = "th"
	}
	if entering {
		_ = w.WriteByte('<')
		_, _ = w.WriteString(tag)
		if class := tableCellAlignClass(cell.Alignment); class != "" {
			_, _ = w.WriteString(` class="`)
			_, _ = w.WriteString(class)
			_ = w.WriteByte('"')
		}
		_ = w.WriteByte('>')
	} else {
		_, _ = w.WriteString("</")
		_, _ = w.WriteString(tag)
		_ = w.WriteByte('>')
	}
	return gast.WalkContinue, nil
}

func tableCellAlignClass(align east.Alignment) string {
	switch align {
	case east.AlignCenter:
		return "align-center"
	case east.AlignRight:
		return "align-right"
	default:
		return ""
	}
}

func renderCollapse(title, body string) string {
	if strings.TrimSpace(title) == "" {
		title = "Details"
	}
	return `<details class="labbit-collapse"><summary>` + inline(title) + `</summary><div class="labbit-collapse-body">` + RenderMarkdown(body) + `</div></details>`
}

func renderImageMarker(payload string) string {
	values, err := url.ParseQuery(payload)
	if err != nil {
		return ""
	}
	kind := normalizeImageType(values.Get("type"))
	alt := values.Get("alt")
	if strings.TrimSpace(alt) == "" {
		alt = "Labbit image"
	}
	body := values.Get("body")
	switch kind {
	case "svg":
		svg := sanitizeSVG(body)
		if svg == "" {
			return ""
		}
		return `<figure class="labbit-image labbit-svg" role="img" aria-label="` + html.EscapeString(alt) + `">` + svg + `</figure>`
	case "png", "jpeg", "webp", "gif":
		data := cleanBase64(body)
		if data == "" {
			return ""
		}
		return `<figure class="labbit-image"><img src="data:` + imageMime(kind) + `;base64,` + data + `" alt="` + html.EscapeString(alt) + `"></figure>`
	default:
		return ""
	}
}

func normalizeImageType(kind string) string {
	switch strings.ToLower(strings.TrimSpace(kind)) {
	case "svg", "image/svg+xml":
		return "svg"
	case "png", "image/png":
		return "png"
	case "jpg", "jpeg", "image/jpg", "image/jpeg":
		return "jpeg"
	case "webp", "image/webp":
		return "webp"
	case "gif", "image/gif":
		return "gif"
	default:
		return ""
	}
}

func imageMime(kind string) string {
	if kind == "jpeg" {
		return "image/jpeg"
	}
	return "image/" + kind
}

func cleanBase64(value string) string {
	value = strings.TrimSpace(value)
	if strings.Contains(value, ",") && strings.HasPrefix(strings.ToLower(value), "data:") {
		value = value[strings.Index(value, ",")+1:]
	}
	value = strings.Join(strings.Fields(value), "")
	if value == "" {
		return ""
	}
	if _, err := base64.StdEncoding.DecodeString(value); err != nil {
		return ""
	}
	return value
}

var (
	svgElements = map[string]bool{
		"svg": true, "g": true, "defs": true, "title": true, "desc": true,
		"path": true, "rect": true, "circle": true, "ellipse": true, "line": true, "polyline": true, "polygon": true,
		"text": true, "tspan": true, "marker": true, "linearGradient": true, "radialGradient": true, "stop": true,
		"style": true,
	}
	svgAttrs = map[string]bool{
		"aria-label": true, "aria-hidden": true, "class": true, "cx": true, "cy": true, "d": true, "dx": true, "dy": true,
		"fill": true, "fill-opacity": true, "font-family": true, "font-size": true, "font-weight": true,
		"gradientUnits": true, "height": true, "id": true, "marker-end": true, "marker-mid": true, "marker-start": true,
		"markerHeight": true, "markerWidth": true,
		"offset": true, "opacity": true, "orient": true, "points": true, "preserveAspectRatio": true,
		"r": true, "refX": true, "refY": true, "rx": true, "ry": true, "spreadMethod": true, "stop-color": true, "stop-opacity": true,
		"style":  true,
		"stroke": true, "stroke-dasharray": true, "stroke-linecap": true, "stroke-linejoin": true, "stroke-opacity": true, "stroke-width": true,
		"text-anchor": true, "transform": true, "viewBox": true, "width": true, "x": true, "x1": true, "x2": true,
		"y": true, "y1": true, "y2": true,
	}
)

func sanitizeSVG(src string) string {
	decoder := xml.NewDecoder(strings.NewReader(strings.TrimSpace(src)))
	var out strings.Builder
	var stack []bool
	var names []string
	sawSVG := false
	for {
		tok, err := decoder.Token()
		if err != nil {
			break
		}
		switch t := tok.(type) {
		case xml.StartElement:
			name := t.Name.Local
			parentAllowed := len(stack) == 0 || stack[len(stack)-1]
			allowed := parentAllowed && svgElements[name]
			stack = append(stack, allowed)
			names = append(names, name)
			if !allowed {
				continue
			}
			if name == "svg" {
				sawSVG = true
			}
			out.WriteByte('<')
			out.WriteString(name)
			for _, attr := range t.Attr {
				attrName := attr.Name.Local
				if !svgAttrs[attrName] || unsafeSVGAttr(attrName, attr.Value) {
					continue
				}
				out.WriteByte(' ')
				out.WriteString(attrName)
				out.WriteString(`="`)
				out.WriteString(html.EscapeString(attr.Value))
				out.WriteByte('"')
			}
			out.WriteByte('>')
		case xml.EndElement:
			if len(stack) == 0 {
				continue
			}
			allowed := stack[len(stack)-1]
			stack = stack[:len(stack)-1]
			name := names[len(names)-1]
			names = names[:len(names)-1]
			if allowed {
				out.WriteString("</")
				out.WriteString(name)
				out.WriteByte('>')
			}
		case xml.CharData:
			if len(stack) > 0 && stack[len(stack)-1] {
				text := string(t)
				if names[len(names)-1] == "style" {
					if unsafeSVGStyle(text) {
						continue
					}
					out.WriteString(text)
				} else {
					out.WriteString(html.EscapeString(text))
				}
			}
		}
	}
	if !sawSVG {
		return ""
	}
	return out.String()
}

func unsafeSVGAttr(name, value string) bool {
	lowerName := strings.ToLower(strings.TrimSpace(name))
	lowerValue := strings.ToLower(strings.TrimSpace(value))
	return strings.HasPrefix(lowerName, "on") ||
		(lowerName == "style" && unsafeSVGStyle(value)) ||
		strings.Contains(lowerValue, "javascript:") ||
		strings.Contains(lowerValue, "data:") ||
		(strings.Contains(lowerValue, "url(") && !localSVGURLValue(lowerValue))
}

func unsafeSVGStyle(value string) bool {
	lower := strings.ToLower(value)
	if strings.Contains(lower, "@import") ||
		strings.Contains(lower, "javascript:") ||
		strings.Contains(lower, "data:") ||
		strings.Contains(lower, "<") {
		return true
	}
	for _, match := range svgCSSURLRE.FindAllStringSubmatch(value, -1) {
		if len(match) < 2 || !strings.HasPrefix(strings.TrimSpace(match[1]), "#") {
			return true
		}
	}
	return false
}

func localSVGURLValue(value string) bool {
	matches := svgCSSURLRE.FindAllStringSubmatch(value, -1)
	if len(matches) == 0 {
		return false
	}
	for _, match := range matches {
		if len(match) < 2 || !strings.HasPrefix(strings.TrimSpace(match[1]), "#") {
			return false
		}
	}
	return true
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
