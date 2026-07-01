// Copyright (C) 2026 WuuBoLin
// SPDX-License-Identifier: GPL-3.0-or-later

package labbit

import (
	"bytes"
	"encoding/xml"
	"errors"
	"fmt"
	"html"
	"io"
	"regexp"
	"strings"
	"unicode"
)

type rawDocument struct {
	XMLName   xml.Name       `xml:"labbit"`
	Title     string         `xml:"title,attr"`
	Slug      string         `xml:"slug,attr"`
	Accent    string         `xml:"accent,attr"`
	Overview  rawText        `xml:"overview"`
	LabTopics []rawTopic     `xml:"lab>topic"`
	QuizTopic []rawQuizTopic `xml:"quiz>topic"`
}

type rawTopic struct {
	ID    string    `xml:"id,attr"`
	Title string    `xml:"title,attr"`
	Tasks []rawTask `xml:"task"`
}

type rawTask struct {
	ID       string    `xml:"id,attr"`
	Title    string    `xml:"title,attr"`
	Hints    []rawHint `xml:"hint"`
	Solution rawText   `xml:"solution"`
	Answer   rawText   `xml:"answer"`
	Inner    string    `xml:",innerxml"`
}

type rawHint struct {
	ID    string `xml:"id,attr"`
	Title string `xml:"title,attr"`
	Inner string `xml:",innerxml"`
}

type rawQuizTopic struct {
	ID        string        `xml:"id,attr"`
	Title     string        `xml:"title,attr"`
	Questions []rawQuestion `xml:"question"`
}

type rawQuestion struct {
	ID          string      `xml:"id,attr"`
	Kind        string      `xml:"type,attr"`
	Prompt      rawText     `xml:"prompt"`
	Options     []rawOption `xml:"option"`
	Explanation rawText     `xml:"explanation"`
}

type rawOption struct {
	ID      string `xml:"id,attr"`
	Correct bool   `xml:"correct,attr"`
	Inner   string `xml:",innerxml"`
}

type rawText struct {
	Inner string `xml:",innerxml"`
}

const DefaultAccent = "#1d9bf0"

var (
	idRE  = regexp.MustCompile(`[^a-z0-9]+`)
	hexRE = regexp.MustCompile(`^#[0-9a-fA-F]{6}$`)
)

func Parse(r io.Reader) (*Document, error) {
	body, err := io.ReadAll(io.LimitReader(r, 2<<20))
	if err != nil {
		return nil, err
	}
	var raw rawDocument
	if err := xml.NewDecoder(bytes.NewReader(body)).Decode(&raw); err != nil {
		return nil, fmt.Errorf("parse labbit XML: %w", err)
	}
	if strings.TrimSpace(raw.Title) == "" {
		return nil, errors.New("labbit title is required")
	}
	if strings.TrimSpace(raw.Overview.Inner) == "" {
		return nil, errors.New("overview is required")
	}

	doc := &Document{
		Slug:     slugOrTitle(raw.Slug, raw.Title),
		Title:    strings.TrimSpace(raw.Title),
		Accent:   normalizeAccent(raw.Accent),
		Overview: RenderMarkdown(componentMarkdown(raw.Overview.Inner)),
	}
	for i, topic := range raw.LabTopics {
		t := Topic{
			ID:    stableID(topic.ID, topic.Title, fmt.Sprintf("lab-%d", i+1)),
			Kind:  "lab",
			Title: fallback(topic.Title, fmt.Sprintf("Lab Topic %d", i+1)),
		}
		for j, task := range topic.Tasks {
			taskID := stableID(task.ID, task.Title, fmt.Sprintf("%s-task-%d", t.ID, j+1))
			inlineHints := parseInlineHints(taskID, task.Hints)
			prompt := taskPrompt(task.Inner, taskID, inlineHints)
			item := Task{
				ID:     taskID,
				Title:  fallback(task.Title, fmt.Sprintf("Task %d", j+1)),
				Prompt: RenderMarkdown(prompt),
			}
			item.Hints = append(item.Hints, inlineHints...)
			solution := componentMarkdown(task.Solution.Inner)
			if solution == "" {
				solution = componentMarkdown(task.Answer.Inner)
			}
			if solution != "" {
				rendered := RenderMarkdown(solution)
				item.Answer = rendered
				item.Hints = append(item.Hints, Hint{
					ID:    fmt.Sprintf("%s-solution", taskID),
					Kind:  "solution",
					Title: "Solution",
					Body:  rendered,
				})
				item.AnswerCount = 1
			}
			item.HintCount = len(item.Hints)
			t.Items = append(t.Items, item)
		}
		doc.Topics = append(doc.Topics, t)
	}
	for i, topic := range raw.QuizTopic {
		topicID := stableID(topic.ID, topic.Title, fmt.Sprintf("quiz-%d", i+1))
		for j, q := range topic.Questions {
			question := Question{
				ID:          stableID(q.ID, textOnly(q.Prompt.Inner), fmt.Sprintf("%s-question-%d", topicID, j+1)),
				TopicID:     topicID,
				TopicTitle:  fallback(topic.Title, fmt.Sprintf("Quiz Topic %d", i+1)),
				Kind:        questionKind(q.Kind),
				Prompt:      RenderMarkdown(componentMarkdown(q.Prompt.Inner)),
				Explanation: RenderMarkdown(componentMarkdown(q.Explanation.Inner)),
			}
			for k, opt := range q.Options {
				question.Options = append(question.Options, Option{
					ID:      stableID(opt.ID, opt.Inner, fmt.Sprintf("%s-option-%d", question.ID, k+1)),
					Label:   strings.TrimSpace(textOnly(opt.Inner)),
					Correct: opt.Correct,
				})
			}
			if len(question.Options) == 0 {
				return nil, fmt.Errorf("question %s must include at least one option", question.ID)
			}
			doc.Questions = append(doc.Questions, question)
		}
	}
	return doc, nil
}

func normalizeAccent(value string) string {
	value = strings.TrimSpace(value)
	if value == "" || strings.EqualFold(value, "DEFAULT") {
		return DefaultAccent
	}
	if !hexRE.MatchString(value) {
		return DefaultAccent
	}
	return strings.ToLower(value)
}

func parseInlineHints(taskID string, hints []rawHint) []Hint {
	out := make([]Hint, 0, len(hints))
	for k, hint := range hints {
		body := componentMarkdown(hint.Inner)
		if body == "" {
			continue
		}
		out = append(out, Hint{
			ID:    stableID(hint.ID, hint.Title, fmt.Sprintf("%s-hint-%d", taskID, k+1)),
			Kind:  "hint",
			Title: fallback(hint.Title, "Inline answer"),
			Body:  RenderMarkdown(body),
		})
	}
	return out
}

func taskPrompt(inner, taskID string, inlineHints []Hint) string {
	decoder := xml.NewDecoder(strings.NewReader("<root>" + inner + "</root>"))
	var out strings.Builder
	skipDepth := 0
	hintIndex := 0
	for {
		tok, err := decoder.Token()
		if err != nil {
			break
		}
		switch t := tok.(type) {
		case xml.StartElement:
			if t.Name.Local == "solution" || t.Name.Local == "answer" {
				skipDepth = 1
				continue
			}
			if t.Name.Local == "hint" {
				if hintIndex < len(inlineHints) {
					out.WriteString("\n\n")
					out.WriteString(inlineHintPlaceholder(taskID, inlineHints[hintIndex].ID, inlineHints[hintIndex].Title))
					out.WriteString("\n\n")
					hintIndex++
				}
				skipDepth = 1
				continue
			}
			if skipDepth > 0 {
				skipDepth++
			}
		case xml.EndElement:
			if skipDepth > 0 {
				skipDepth--
				continue
			}
		case xml.CharData:
			if skipDepth == 0 {
				out.Write([]byte(t))
			}
		}
	}
	return dedentMarkdown(out.String())
}

func inlineHintPlaceholder(taskID, hintID, title string) string {
	if strings.TrimSpace(title) == "" {
		title = "Reveal inline answer"
	}
	return `<button class="inline-answer-toggle" type="button" data-inline-answer-toggle data-task-id="` + html.EscapeString(taskID) + `" data-hint-id="` + html.EscapeString(hintID) + `" aria-label="Reveal inline answer">` + html.EscapeString(title) + `</button>`
}

func componentMarkdown(s string) string {
	s = strings.ReplaceAll(s, "<![CDATA[", "")
	s = strings.ReplaceAll(s, "]]>", "")
	s = strings.ReplaceAll(s, "<note>", "\n> Note: ")
	s = strings.ReplaceAll(s, "</note>", "\n")
	s = strings.ReplaceAll(s, "<callout>", "\n> ")
	s = strings.ReplaceAll(s, "</callout>", "\n")
	return dedentMarkdown(textOnly(s))
}

func dedentMarkdown(s string) string {
	lines := strings.Split(strings.Trim(s, "\n\r\t "), "\n")
	minIndent := -1
	for _, line := range lines {
		if strings.TrimSpace(line) == "" {
			continue
		}
		indent := 0
		for _, r := range line {
			if r != ' ' && r != '\t' {
				break
			}
			indent++
		}
		if minIndent == -1 || indent < minIndent {
			minIndent = indent
		}
	}
	if minIndent <= 0 {
		return strings.TrimSpace(strings.Join(lines, "\n"))
	}
	for i, line := range lines {
		remove := 0
		for remove < minIndent && remove < len(line) && (line[remove] == ' ' || line[remove] == '\t') {
			remove++
		}
		lines[i] = line[remove:]
	}
	return strings.TrimSpace(strings.Join(lines, "\n"))
}

func textOnly(s string) string {
	var out strings.Builder
	decoder := xml.NewDecoder(strings.NewReader("<root>" + s + "</root>"))
	for {
		tok, err := decoder.Token()
		if err != nil {
			break
		}
		if data, ok := tok.(xml.CharData); ok {
			out.Write([]byte(data))
		}
	}
	if out.Len() == 0 {
		return strings.TrimSpace(s)
	}
	return strings.TrimSpace(out.String())
}

func questionKind(kind string) string {
	switch strings.ToLower(strings.TrimSpace(kind)) {
	case "multiple", "checkbox", "multiple-choice":
		return "multiple"
	default:
		return "single"
	}
}

func stableID(id, title, fallbackID string) string {
	if strings.TrimSpace(id) != "" {
		return slugOrTitle(id, fallbackID)
	}
	if strings.TrimSpace(title) != "" {
		return slugOrTitle(title, fallbackID)
	}
	return fallbackID
}

func slugOrTitle(slug, title string) string {
	source := strings.ToLower(strings.TrimSpace(slug))
	if source == "" {
		source = strings.ToLower(strings.TrimSpace(title))
	}
	var cleaned strings.Builder
	for _, r := range source {
		if unicode.IsLetter(r) || unicode.IsDigit(r) {
			cleaned.WriteRune(r)
			continue
		}
		cleaned.WriteByte('-')
	}
	out := strings.Trim(idRE.ReplaceAllString(cleaned.String(), "-"), "-")
	if out == "" {
		return "lab"
	}
	return out
}

func fallback(value, fallback string) string {
	if strings.TrimSpace(value) == "" {
		return fallback
	}
	return strings.TrimSpace(value)
}
