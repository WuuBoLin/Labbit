package web

import (
	"bytes"
	"context"
	"strings"
	"testing"

	"github.com/a-h/templ"
	"labbit/internal/labbit"
)

func renderString(t *testing.T, component templ.Component) string {
	t.Helper()
	var out bytes.Buffer
	if err := component.Render(context.Background(), &out); err != nil {
		t.Fatalf("render component: %v", err)
	}
	return out.String()
}

func templateDoc() *labbit.Document {
	return &labbit.Document{
		UID:      "abc1234",
		Slug:     "linux-services",
		Title:    "Linux Services",
		Accent:   labbit.DefaultAccent,
		Overview: "Overview",
		Topics: []labbit.Topic{{
			ID:    "samba",
			Kind:  "lab",
			Title: "Samba",
			Items: []labbit.Task{{
				ID:     "setup-samba",
				Title:  "Setup Samba",
				Prompt: `<p>Use this <button class="inline-hint-toggle" type="button" data-inline-hint-toggle data-task-id="setup-samba" data-hint-id="package" aria-label="Reveal inline hint">Package hint</button></p>`,
			}},
		}},
		Questions: []labbit.Question{{
			ID:         "daemon",
			TopicID:    "basics",
			TopicTitle: "Basics",
			Kind:       "single",
			Prompt:     "Which service?",
			Options: []labbit.Option{
				{ID: "a", Label: "smb", Correct: true},
				{ID: "b", Label: "sshd"},
			},
		}},
	}
}

func TestSectionFragmentIncludesOOBActiveNav(t *testing.T) {
	html := renderString(t, SectionFragment(templateDoc(), "samba", "setup-samba"))
	for _, want := range []string{
		`hx-swap-oob="outerHTML"`,
		`class="nav-link active" data-section-id="samba"`,
		`class="task-card selected"`,
	} {
		if !strings.Contains(html, want) {
			t.Fatalf("section fragment missing %q: %s", want, html)
		}
	}
}

func TestLabTopicHydratesInlineHintControls(t *testing.T) {
	doc := templateDoc()
	html := renderString(t, LabTopicSection(doc, doc.Topics[0], ""))
	for _, want := range []string{
		`hx-get="/docs/abc1234/linux-services/keys/labs/setup-samba/package"`,
		`hx-swap="outerHTML"`,
		`hx-trigger="click once"`,
	} {
		if !strings.Contains(html, want) {
			t.Fatalf("inline hint control missing %q: %s", want, html)
		}
	}
}

func TestQuizCardSwapsOuterActionBlock(t *testing.T) {
	doc := templateDoc()
	html := renderString(t, QuizCard(doc, "basics", doc.Questions[0], "01", "daemon", nil, nil, false, false))
	for _, want := range []string{
		`<section class="quiz-card selected" id="daemon" data-quiz-card data-nav-block data-action-block`,
		`hx-target="closest [data-action-block]"`,
		`data-block-link data-share-target="daemon" hx-get="/docs/abc1234/linux-services/quiz/basics?block=daemon"`,
	} {
		if !strings.Contains(html, want) {
			t.Fatalf("quiz card missing %q: %s", want, html)
		}
	}
}
