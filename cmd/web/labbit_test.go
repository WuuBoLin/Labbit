// Copyright (C) 2026 WuuBoLin
// SPDX-License-Identifier: GPL-3.0-or-later

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
		UID:       "abc1234",
		Slug:      "linux-services",
		Title:     "Linux Services",
		Accent:    labbit.DefaultAccent,
		OwnerName: "alice",
		Overview:  "Overview",
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

func TestViewerShellHidesSignoutWhenSignedOut(t *testing.T) {
	html := renderString(t, ViewerShell(templateDoc(), "overview", "dark", nil, false))
	if strings.Contains(html, `hx-post="/id/signout"`) {
		t.Fatalf("auth disabled viewer shell included signout: %s", html)
	}
	if strings.Contains(html, `has-signout`) {
		t.Fatalf("signed-out viewer shell rendered signed-in bottom state: %s", html)
	}
}

func TestViewerShellRendersMobileAccountSignoutWhenSignedIn(t *testing.T) {
	user := &labbit.User{Status: labbit.UserStatusActive, Username: "alice"}
	html := renderString(t, ViewerShell(templateDoc(), "overview", "dark", user, false))
	for _, want := range []string{
		`class="sidebar-account-mobile lg:hidden mt-6 pt-4 border-t border-zinc-800 flex items-center justify-between"`,
		`href="/@alice"`,
		`href="/id/signout"`,
		`order-first`,
	} {
		if !strings.Contains(html, want) {
			t.Fatalf("signed-in viewer shell missing %q: %s", want, html)
		}
	}
}

func TestSignOutIconFacesLeft(t *testing.T) {
	html := renderString(t, SignOutIcon())
	for _, want := range []string{
		`M15 21h4a2 2 0 0 0 2-2V5a2 2 0 0 0-2-2h-4`,
		`M8 17l-5-5 5-5`,
		`M3 12h12`,
	} {
		if !strings.Contains(html, want) {
			t.Fatalf("signout icon missing left-facing path %q: %s", want, html)
		}
	}
}

func TestHomePageRendersSkillResourceBox(t *testing.T) {
	html := renderString(t, HomePage(nil, nil, "", "dark", "", nil, false))
	for _, want := range []string{
		`href="/assets/SKILL.md"`,
		`download="SKILL.md"`,
		`data-copy data-copy-url="/assets/SKILL.md"`,
		`Use with AI agents to generate Labbit XML.`,
	} {
		if !strings.Contains(html, want) {
			t.Fatalf("home page missing SKILL resource markup %q: %s", want, html)
		}
	}
}

func TestBaseRendersOpenGraphMetadata(t *testing.T) {
	meta := WebsitePageMeta("https://labbit.example", "/")
	html := renderString(t, ComponentWithPageMeta(Base("Fallback", "dark"), meta))
	for _, want := range []string{
		`<meta name="description" content="Web viewer for lab exam notes. Upload a Labbit XML file and Labbit turns it into a documentation-style workspace with LABs and QUIZ.">`,
		`<meta property="og:title" content="Labbit · Lab and Quiz viewer">`,
		`<meta property="og:description" content="Web viewer for lab exam notes. Upload a Labbit XML file and Labbit turns it into a documentation-style workspace with LABs and QUIZ.">`,
		`<meta property="og:type" content="website">`,
		`<meta property="og:url" content="https://labbit.example/">`,
		`<meta property="og:site_name" content="Labbit">`,
		`<meta property="og:locale" content="en_US">`,
		`<meta property="og:determiner" content="auto">`,
		`<meta property="og:image" content="https://labbit.example/assets/img/social-card.png">`,
		`<meta property="og:image:type" content="image/png">`,
		`<meta property="og:image:width" content="1200">`,
		`<meta property="og:image:height" content="630">`,
		`<meta property="og:image:alt" content="Labbit social card">`,
		`<meta name="twitter:card" content="summary_large_image">`,
		`<meta name="twitter:image" content="https://labbit.example/assets/img/social-card.png">`,
		`<meta name="twitter:image:alt" content="Labbit social card">`,
		`<link rel="canonical" href="https://labbit.example/">`,
	} {
		if !strings.Contains(html, want) {
			t.Fatalf("base metadata missing %q: %s", want, html)
		}
	}
}

func TestOnboardingPagePostsNextInQuery(t *testing.T) {
	user := &labbit.User{Status: labbit.UserStatusPending}
	html := renderString(t, OnboardingPage(user, "dark", "", "/after"))
	if !strings.Contains(html, `method="post" action="/i/onboarding?next=%2Fafter"`) {
		t.Fatalf("onboarding page missing query next action: %s", html)
	}
	if strings.Contains(html, `name="next"`) {
		t.Fatalf("onboarding page rendered body next field: %s", html)
	}
	if !strings.Contains(html, `hx-post="/id/signout?next=%2F"`) {
		t.Fatalf("onboarding page missing immediate signout form: %s", html)
	}
	for _, want := range []string{
		`<form class="id-panel id-panel-frame" method="post" action="/i/onboarding?next=%2Fafter">`,
		`class="id-panel-header"`,
		`class="id-icon-tile"`,
		`Choose a username`,
	} {
		if !strings.Contains(html, want) {
			t.Fatalf("onboarding page missing shared ID layout %q: %s", want, html)
		}
	}
}

func TestDocsRowUsesConfirmAndSegmentedVisibility(t *testing.T) {
	doc := templateDoc()
	item := labbit.RecentDocument{Document: doc, Visibility: labbit.VisibilityPublic}
	html := renderString(t, DocsRow(item, 2, ""))
	for _, want := range []string{
		`hx-put="/i/library/abc1234/linux-services/visibility?page=2"`,
		`hx-delete="/i/library/abc1234/linux-services?page=2"`,
		`hx-confirm="Delete this document from your library?"`,
		`class="visibility-segment"`,
		`type="checkbox" name="doc" value="abc1234" form="docs-bulk-form"`,
		`class="visibility-option public active" type="submit" name="visibility" value="public" disabled aria-pressed`,
		`class="visibility-option private" type="submit" name="visibility" value="private"`,
	} {
		if !strings.Contains(html, want) {
			t.Fatalf("docs row missing %q: %s", want, html)
		}
	}
	for _, unwanted := range []string{`/delete`, `onsubmit=`, `method="post"`} {
		if strings.Contains(html, unwanted) {
			t.Fatalf("docs row rendered fallback %q: %s", unwanted, html)
		}
	}
}

func TestDocsListFragmentRendersSearchAndBulkDelete(t *testing.T) {
	doc := templateDoc()
	item := labbit.RecentDocument{Document: doc, Visibility: labbit.VisibilityPublic}
	html := renderString(t, DocsListFragment([]labbit.RecentDocument{item}, 2, true, "linux", ""))
	for _, want := range []string{
		`id="library-search" type="search" name="q" value="linux"`,
		`hx-get="/i/library"`,
		`hx-trigger="input changed delay:150ms, search"`,
		`hx-push-url="true"`,
		`id="docs-bulk-form"`,
		`hx-delete="/i/library"`,
		`hx-confirm="Delete selected docs from your library?"`,
		`name="q" value="linux"`,
		`href="/i/library?q=linux"`,
		`href="/i/library?page=3&amp;q=linux"`,
	} {
		if !strings.Contains(html, want) {
			t.Fatalf("docs list missing %q: %s", want, html)
		}
	}
}

func TestLabTopicHydratesInlineHintControls(t *testing.T) {
	doc := templateDoc()
	html := renderString(t, LabTopicSection(doc, doc.Topics[0], ""))
	for _, want := range []string{
		`hx-get="/@alice/docs/abc1234/linux-services/keys/labs/setup-samba/package"`,
		`hx-swap="outerHTML"`,
		`hx-trigger="click once"`,
	} {
		if !strings.Contains(html, want) {
			t.Fatalf("inline hint control missing %q: %s", want, html)
		}
	}
}

func TestLabTopicUsesNativeSolutionDisclosure(t *testing.T) {
	doc := templateDoc()
	doc.Topics[0].Items[0].SolutionCount = 1
	html := renderString(t, LabTopicSection(doc, doc.Topics[0], ""))
	for _, want := range []string{
		`<details class="solution-toggle-wrap solution-disclosure">`,
		`<summary class="solution-toggle" data-solution-toggle`,
		`hx-get="/@alice/docs/abc1234/linux-services/keys/labs/setup-samba"`,
		`hx-trigger="click once"`,
		`id="solution-setup-samba" class="solution-slot"`,
	} {
		if !strings.Contains(html, want) {
			t.Fatalf("solution disclosure missing %q: %s", want, html)
		}
	}
	for _, unwanted := range []string{`data-solution-loaded`, `class="solution-slot hidden"`, `aria-expanded=`} {
		if strings.Contains(html, unwanted) {
			t.Fatalf("solution disclosure still renders JS-managed state %q: %s", unwanted, html)
		}
	}
}

func TestQuizCardSwapsOuterActionBlock(t *testing.T) {
	doc := templateDoc()
	html := renderString(t, QuizCard(doc, "basics", doc.Questions[0], "01", "daemon", nil, nil, false, false))
	for _, want := range []string{
		`<section class="quiz-card selected" id="daemon" data-quiz-card data-nav-block data-action-block`,
		`hx-target="closest [data-action-block]"`,
		`data-block-link data-share-target="daemon" hx-get="/@alice/docs/abc1234/linux-services/quiz/basics?block=daemon"`,
	} {
		if !strings.Contains(html, want) {
			t.Fatalf("quiz card missing %q: %s", want, html)
		}
	}
}
