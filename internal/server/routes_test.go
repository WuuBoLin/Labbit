// Copyright (C) 2026 WuuBoLin
// SPDX-License-Identifier: GPL-3.0-or-later

package server

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"labbit/internal/labbit"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/klauspost/compress/zstd"
)

func TestIDConfigDefaultsToConfiguredLocalPort(t *testing.T) {
	id, err := newIDConfig("", 8081)
	if err != nil {
		t.Fatalf("newIDConfig() error = %v", err)
	}
	if id.publicURL != "http://localhost:8081" || id.origin != "http://localhost:8081" || id.rpID != "localhost" {
		t.Fatalf("id config = %#v", id)
	}
}

func TestLegacyDocRoutesAreRemoved(t *testing.T) {
	store, err := labbit.NewMemoryStore()
	if err != nil {
		t.Fatalf("NewMemoryStore() error = %v", err)
	}
	t.Cleanup(func() { _ = store.Close() })
	doc, err := labbit.Parse(strings.NewReader(`<labbit title="Linux Services Exam" slug="linux-services"><overview>Overview</overview></labbit>`))
	if err != nil {
		t.Fatalf("Parse() error = %v", err)
	}
	doc.UID = "c40a39f"
	doc.Hash = "sample-hash"
	if err := store.SaveDocument(context.Background(), doc); err != nil {
		t.Fatalf("SaveDocument() error = %v", err)
	}

	req := httptest.NewRequest(http.MethodGet, docRoute("c40a39f"), nil)
	resp := httptest.NewRecorder()
	(&Server{labs: store}).RegisterRoutes().ServeHTTP(resp, req)
	if resp.Code != http.StatusNotFound {
		t.Fatalf("status = %d", resp.Code)
	}
}

func TestStaticCacheHeaders(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/assets/js/labbit.js", nil)
	resp := httptest.NewRecorder()
	(&Server{}).RegisterRoutes().ServeHTTP(resp, req)
	if resp.Code != http.StatusOK {
		t.Fatalf("status = %d", resp.Code)
	}
	if got := resp.Header().Get("Cache-Control"); got != "public, max-age=300, stale-while-revalidate=86400" {
		t.Fatalf("Cache-Control = %q", got)
	}

	req = httptest.NewRequest(http.MethodGet, "/assets/js/htmx.min.js", nil)
	resp = httptest.NewRecorder()
	(&Server{}).RegisterRoutes().ServeHTTP(resp, req)
	if got := resp.Header().Get("Cache-Control"); got != "public, max-age=31536000, immutable" {
		t.Fatalf("HTMX Cache-Control = %q", got)
	}
}

func TestSkillMarkdownAsset(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/assets/SKILL.md", nil)
	resp := httptest.NewRecorder()
	(&Server{}).RegisterRoutes().ServeHTTP(resp, req)

	if resp.Code != http.StatusOK {
		t.Fatalf("GET /assets/SKILL.md status = %d, want 200: %s", resp.Code, resp.Body.String())
	}
	if got := resp.Header().Get("Content-Type"); !strings.HasPrefix(got, "text/markdown") {
		t.Fatalf("Content-Type = %q, want text/markdown", got)
	}
	if body := resp.Body.String(); !strings.Contains(body, "# Labbit Authoring Guide") || !strings.Contains(body, "valid Labbit XML") {
		t.Fatalf("GET /assets/SKILL.md returned unexpected body: %s", body)
	}
}

func TestHomePageRendersWebsiteOpenGraphMetadata(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	resp := httptest.NewRecorder()
	(&Server{publicURL: "https://labbit.example"}).RegisterRoutes().ServeHTTP(resp, req)
	if resp.Code != http.StatusOK {
		t.Fatalf("status = %d", resp.Code)
	}
	assertHTMLContainsAll(t, resp.Body.String(), []string{
		`<meta property="og:type" content="website">`,
		`<meta property="og:title" content="Labbit · Lab and Quiz viewer">`,
		`<meta property="og:description" content="Web viewer for lab exam notes. Upload a Labbit XML file and Labbit turns it into a documentation-style workspace with LABs and QUIZ.">`,
		`<meta property="og:url" content="https://labbit.example/">`,
		`<meta property="og:image" content="https://labbit.example/assets/img/social-card.png">`,
		`<meta property="og:image:width" content="1200">`,
		`<meta property="og:image:height" content="630">`,
		`<meta name="twitter:card" content="summary_large_image">`,
		`<meta name="twitter:image" content="https://labbit.example/assets/img/social-card.png">`,
		`<link rel="canonical" href="https://labbit.example/">`,
	})
}

func TestUserPageRendersProfileOpenGraphMetadata(t *testing.T) {
	store, err := labbit.NewMemoryStore()
	if err != nil {
		t.Fatalf("NewMemoryStore() error = %v", err)
	}
	t.Cleanup(func() { _ = store.Close() })
	activeTestUser(t, store, "alice")
	handler := (&Server{labs: store, publicURL: "https://labbit.example"}).RegisterRoutes()

	req := httptest.NewRequest(http.MethodGet, "/@alice?q=ignored", nil)
	resp := httptest.NewRecorder()
	handler.ServeHTTP(resp, req)
	if resp.Code != http.StatusOK {
		t.Fatalf("status = %d", resp.Code)
	}
	assertHTMLContainsAll(t, resp.Body.String(), []string{
		`<meta property="og:type" content="profile">`,
		`<meta property="og:title" content="alice&#39;s docs · Labbit">`,
		`<meta property="og:description" content="Labbit documents by alice.">`,
		`<meta property="og:url" content="https://labbit.example/@alice">`,
		`<meta property="og:image" content="https://labbit.example/assets/img/icon-512.png">`,
		`<meta property="og:image:width" content="512">`,
		`<meta property="og:image:height" content="512">`,
		`<meta property="og:image:alt" content="Labbit icon">`,
		`<meta name="twitter:card" content="summary">`,
		`<meta name="twitter:image" content="https://labbit.example/assets/img/icon-512.png">`,
		`<meta property="profile:username" content="alice">`,
		`<link rel="canonical" href="https://labbit.example/@alice">`,
	})
	if strings.Contains(resp.Body.String(), "Public Labbit documents by alice") {
		t.Fatalf("profile description still contains Public: %s", resp.Body.String())
	}
}

func TestDocumentPagesRenderArticleOpenGraphMetadata(t *testing.T) {
	store, err := labbit.NewMemoryStore()
	if err != nil {
		t.Fatalf("NewMemoryStore() error = %v", err)
	}
	t.Cleanup(func() { _ = store.Close() })
	doc, err := labbit.Parse(strings.NewReader(labbitSample()))
	if err != nil {
		t.Fatalf("Parse() error = %v", err)
	}
	doc.UID = "ogdoc1"
	doc.Hash = "og-doc"
	if err := store.SaveDocument(context.Background(), doc); err != nil {
		t.Fatalf("SaveDocument() error = %v", err)
	}
	owner := activeTestUser(t, store, "alice")
	if err := store.SaveUserDocument(context.Background(), owner.ID, doc.ID, labbit.VisibilityPublic); err != nil {
		t.Fatalf("SaveUserDocument() error = %v", err)
	}
	stored, err := store.GetUserDocument(context.Background(), "alice", "ogdoc1", "linux-services")
	if err != nil {
		t.Fatalf("GetUserDocument() error = %v", err)
	}
	published := stored.UploadedAt.UTC().Format(time.RFC3339)
	handler := (&Server{labs: store, publicURL: "https://labbit.example"}).RegisterRoutes()

	req := httptest.NewRequest(http.MethodGet, "/@alice/docs/ogdoc1/linux-services?block=ignored", nil)
	resp := httptest.NewRecorder()
	handler.ServeHTTP(resp, req)
	if resp.Code != http.StatusOK {
		t.Fatalf("doc status = %d", resp.Code)
	}
	assertHTMLContainsAll(t, resp.Body.String(), []string{
		`<meta property="og:type" content="article">`,
		`<meta property="og:title" content="Linux Services Exam">`,
		`<meta property="og:description" content="Overview">`,
		`<meta property="og:url" content="https://labbit.example/@alice/docs/ogdoc1/linux-services">`,
		`<meta property="og:image" content="https://labbit.example/assets/img/social-card.png">`,
		`<meta property="og:image:width" content="1200">`,
		`<meta property="og:image:height" content="630">`,
		`<meta property="og:image:alt" content="Labbit social card">`,
		`<meta name="twitter:card" content="summary_large_image">`,
		`<meta name="twitter:image" content="https://labbit.example/assets/img/social-card.png">`,
		`<meta property="article:author" content="https://labbit.example/@alice">`,
		`<meta property="article:published_time" content="` + published + `">`,
		`<meta property="article:modified_time" content="` + published + `">`,
		`<meta property="article:section" content="Overview">`,
		`<meta property="article:tag" content="Labbit">`,
		`<meta property="article:tag" content="LAB">`,
		`<meta property="article:tag" content="QUIZ">`,
	})

	req = httptest.NewRequest(http.MethodGet, "/@alice/docs/ogdoc1/linux-services/quiz/basics", nil)
	resp = httptest.NewRecorder()
	handler.ServeHTTP(resp, req)
	if resp.Code != http.StatusOK {
		t.Fatalf("quiz section status = %d", resp.Code)
	}
	assertHTMLContainsAll(t, resp.Body.String(), []string{
		`<meta property="og:title" content="Basics · Linux Services Exam · Labbit">`,
		`<meta property="og:url" content="https://labbit.example/@alice/docs/ogdoc1/linux-services/quiz/basics">`,
		`<meta property="og:image" content="https://labbit.example/assets/img/social-card.png">`,
		`<meta name="twitter:card" content="summary_large_image">`,
		`<meta property="article:section" content="QUIZ">`,
	})
}

func TestZstdCompressionStaticTextAsset(t *testing.T) {
	handler := (&Server{}).RegisterRoutes()

	plainReq := httptest.NewRequest(http.MethodGet, "/assets/js/labbit.js", nil)
	plainResp := httptest.NewRecorder()
	handler.ServeHTTP(plainResp, plainReq)
	if plainResp.Code != http.StatusOK {
		t.Fatalf("plain status = %d", plainResp.Code)
	}

	req := httptest.NewRequest(http.MethodGet, "/assets/js/labbit.js", nil)
	req.Header.Set("Accept-Encoding", "br, zstd")
	resp := httptest.NewRecorder()
	handler.ServeHTTP(resp, req)
	if resp.Code != http.StatusOK {
		t.Fatalf("compressed status = %d", resp.Code)
	}
	if got := resp.Header().Get("Content-Encoding"); got != "zstd" {
		t.Fatalf("Content-Encoding = %q, want zstd", got)
	}
	if got := resp.Header().Values("Vary"); !headerValuesContain(got, "Accept-Encoding") {
		t.Fatalf("Vary = %q, want Accept-Encoding", got)
	}
	if got := resp.Header().Get("Content-Length"); got != "" {
		t.Fatalf("Content-Length = %q, want empty", got)
	}
	decoded := decodeZstdBody(t, resp.Body.Bytes())
	if !bytes.Equal(decoded, plainResp.Body.Bytes()) {
		t.Fatalf("decoded body does not match plain body")
	}
}

func TestZstdCompressionNegotiationSkipsUnsupportedAndDisabled(t *testing.T) {
	handler := (&Server{}).RegisterRoutes()
	for _, acceptEncoding := range []string{"gzip, br", "zstd;q=0, gzip"} {
		req := httptest.NewRequest(http.MethodGet, "/assets/js/labbit.js", nil)
		req.Header.Set("Accept-Encoding", acceptEncoding)
		resp := httptest.NewRecorder()
		handler.ServeHTTP(resp, req)
		if resp.Code != http.StatusOK {
			t.Fatalf("Accept-Encoding %q status = %d", acceptEncoding, resp.Code)
		}
		if got := resp.Header().Get("Content-Encoding"); got != "" {
			t.Fatalf("Accept-Encoding %q Content-Encoding = %q, want empty", acceptEncoding, got)
		}
	}
}

func TestZstdCompressionSkipsSmallBinaryRangeAndWebsocketResponses(t *testing.T) {
	store, err := labbit.NewMemoryStore()
	if err != nil {
		t.Fatalf("NewMemoryStore() error = %v", err)
	}
	t.Cleanup(func() { _ = store.Close() })
	handler := (&Server{labs: store}).RegisterRoutes()

	tests := []struct {
		name string
		req  *http.Request
	}{
		{"small health JSON", httptest.NewRequest(http.MethodGet, "/_/healthz", nil)},
		{"binary static asset", httptest.NewRequest(http.MethodGet, "/assets/img/icon-16.png", nil)},
		{"range request", httptest.NewRequest(http.MethodGet, "/assets/js/labbit.js", nil)},
		{"websocket upgrade", httptest.NewRequest(http.MethodGet, "/_/websocket", nil)},
	}
	tests[2].req.Header.Set("Range", "bytes=0-20")
	tests[3].req.Header.Set("Connection", "Upgrade")
	tests[3].req.Header.Set("Upgrade", "websocket")

	for _, tt := range tests {
		tt.req.Header.Set("Accept-Encoding", "zstd")
		resp := httptest.NewRecorder()
		handler.ServeHTTP(resp, tt.req)
		if got := resp.Header().Get("Content-Encoding"); got != "" {
			t.Fatalf("%s Content-Encoding = %q, want empty", tt.name, got)
		}
	}
}

func TestDocumentRoutesUseTypedSectionsAndKeys(t *testing.T) {
	store, err := labbit.NewMemoryStore()
	if err != nil {
		t.Fatalf("NewMemoryStore() error = %v", err)
	}
	t.Cleanup(func() { _ = store.Close() })
	doc, err := labbit.Parse(strings.NewReader(labbitSample()))
	if err != nil {
		t.Fatalf("Parse() error = %v", err)
	}
	doc.UID = "c40a39f"
	doc.Hash = "route-sample"
	if err := store.SaveDocument(context.Background(), doc); err != nil {
		t.Fatalf("SaveDocument() error = %v", err)
	}
	owner := activeTestUser(t, store, "alice")
	if err := store.SaveUserDocument(context.Background(), owner.ID, doc.ID, labbit.VisibilityPublic); err != nil {
		t.Fatalf("SaveUserDocument() error = %v", err)
	}
	handler := (&Server{labs: store}).RegisterRoutes()

	for _, tc := range []struct {
		method string
		path   string
		status int
	}{
		{http.MethodGet, userDocPath("alice", "c40a39f", "linux-services", "labs", "samba"), http.StatusOK},
		{http.MethodGet, userDocPath("alice", "c40a39f", "linux-services", "quiz", "basics"), http.StatusOK},
		{http.MethodGet, userDocPath("alice", "c40a39f", "linux-services", "keys", "labs", "setup-samba"), http.StatusOK},
		{http.MethodGet, userDocPath("alice", "c40a39f", "linux-services", "keys", "setup-samba"), http.StatusNotFound},
		{http.MethodGet, userDocPath("alice", "c40a39f", "linux-services", "answers", "setup-samba"), http.StatusNotFound},
		{http.MethodGet, userDocPath("alice", "c40a39f", "linux-services", "section", "samba"), http.StatusNotFound},
	} {
		req := httptest.NewRequest(tc.method, tc.path, nil)
		resp := httptest.NewRecorder()
		handler.ServeHTTP(resp, req)
		if resp.Code != tc.status {
			t.Fatalf("%s %s status = %d, want %d", tc.method, tc.path, resp.Code, tc.status)
		}
	}
}

func TestPrivateDocumentRequiresOwnerSession(t *testing.T) {
	store, err := labbit.NewMemoryStore()
	if err != nil {
		t.Fatalf("NewMemoryStore() error = %v", err)
	}
	t.Cleanup(func() { _ = store.Close() })
	doc, err := labbit.Parse(strings.NewReader(labbitSample()))
	if err != nil {
		t.Fatalf("Parse() error = %v", err)
	}
	doc.UID = "private1"
	doc.Hash = "private-sample"
	if err := store.SaveDocument(context.Background(), doc); err != nil {
		t.Fatalf("SaveDocument() error = %v", err)
	}
	owner := activeTestUser(t, store, "alice")
	other := activeTestUser(t, store, "bob")
	if err := store.SaveUserDocument(context.Background(), owner.ID, doc.ID, labbit.VisibilityPrivate); err != nil {
		t.Fatalf("SaveUserDocument() error = %v", err)
	}
	handler := (&Server{labs: store}).RegisterRoutes()
	path := userDocPath("alice", "private1", "linux-services")

	req := httptest.NewRequest(http.MethodGet, path, nil)
	resp := httptest.NewRecorder()
	handler.ServeHTTP(resp, req)
	if resp.Code != http.StatusSeeOther {
		t.Fatalf("signed-out status = %d, want 303", resp.Code)
	}
	location, err := url.Parse(resp.Header().Get("Location"))
	if err != nil {
		t.Fatalf("authenticate redirect location is invalid: %v", err)
	}
	if location.Path != "/id/authenticate" || location.Query().Get("next") != path {
		t.Fatalf("authenticate redirect = %q, want /id/authenticate next %q", resp.Header().Get("Location"), path)
	}

	req = httptest.NewRequest(http.MethodGet, path, nil)
	addSessionCookie(t, store, req, other.ID)
	resp = httptest.NewRecorder()
	handler.ServeHTTP(resp, req)
	if resp.Code != http.StatusNotFound {
		t.Fatalf("non-owner status = %d, want 404", resp.Code)
	}

	req = httptest.NewRequest(http.MethodGet, path, nil)
	addSessionCookie(t, store, req, owner.ID)
	resp = httptest.NewRecorder()
	handler.ServeHTTP(resp, req)
	if resp.Code != http.StatusOK {
		t.Fatalf("owner status = %d, want 200", resp.Code)
	}
}

func TestPublicDocumentOnlyShowsSignoutForSignedInSession(t *testing.T) {
	store, err := labbit.NewMemoryStore()
	if err != nil {
		t.Fatalf("NewMemoryStore() error = %v", err)
	}
	t.Cleanup(func() { _ = store.Close() })
	doc, err := labbit.Parse(strings.NewReader(labbitSample()))
	if err != nil {
		t.Fatalf("Parse() error = %v", err)
	}
	doc.UID = "public1"
	doc.Hash = "public-sample"
	if err := store.SaveDocument(context.Background(), doc); err != nil {
		t.Fatalf("SaveDocument() error = %v", err)
	}
	owner := activeTestUser(t, store, "alice")
	if err := store.SaveUserDocument(context.Background(), owner.ID, doc.ID, labbit.VisibilityPublic); err != nil {
		t.Fatalf("SaveUserDocument() error = %v", err)
	}
	handler := (&Server{labs: store}).RegisterRoutes()
	path := userDocPath("alice", "public1", "linux-services")

	req := httptest.NewRequest(http.MethodGet, path, nil)
	resp := httptest.NewRecorder()
	handler.ServeHTTP(resp, req)
	if resp.Code != http.StatusOK {
		t.Fatalf("signed-out status = %d, want 200", resp.Code)
	}
	if strings.Contains(resp.Body.String(), `href="/id/signout"`) {
		t.Fatalf("signed-out public document rendered sign-out link: %s", resp.Body.String())
	}

	req = httptest.NewRequest(http.MethodGet, path, nil)
	addSessionCookie(t, store, req, owner.ID)
	resp = httptest.NewRecorder()
	handler.ServeHTTP(resp, req)
	if resp.Code != http.StatusOK {
		t.Fatalf("signed-in status = %d, want 200", resp.Code)
	}
	if !strings.Contains(resp.Body.String(), `href="/id/signout"`) {
		t.Fatalf("signed-in public document missing sign-out link: %s", resp.Body.String())
	}
}

func TestLibraryPageRequiresActiveSessionAndRenders(t *testing.T) {
	store, err := labbit.NewMemoryStore()
	if err != nil {
		t.Fatalf("NewMemoryStore() error = %v", err)
	}
	t.Cleanup(func() { _ = store.Close() })
	user := activeTestUser(t, store, "alice")
	handler := (&Server{labs: store, publicURL: "https://labbit.example"}).RegisterRoutes()

	req := httptest.NewRequest(http.MethodGet, "/i/library", nil)
	resp := httptest.NewRecorder()
	handler.ServeHTTP(resp, req)
	if resp.Code != http.StatusSeeOther {
		t.Fatalf("signed-out status = %d, want 303", resp.Code)
	}
	location, err := url.Parse(resp.Header().Get("Location"))
	if err != nil {
		t.Fatalf("authenticate redirect location is invalid: %v", err)
	}
	if location.Path != "/id/authenticate" || location.Query().Get("next") != "/i/library" {
		t.Fatalf("authenticate redirect = %q, want /id/authenticate next /i/library", resp.Header().Get("Location"))
	}

	req = httptest.NewRequest(http.MethodGet, "/i/library", nil)
	addSessionCookie(t, store, req, user.ID)
	resp = httptest.NewRecorder()
	handler.ServeHTTP(resp, req)
	if resp.Code != http.StatusOK {
		t.Fatalf("signed-in status = %d, want 200: %s", resp.Code, resp.Body.String())
	}
	for _, want := range []string{`Your docs`, `@alice`, `id="docs-list-region"`} {
		if !strings.Contains(resp.Body.String(), want) {
			t.Fatalf("signed-in docs page missing %q: %s", want, resp.Body.String())
		}
	}
}

func TestUserPublicPage(t *testing.T) {
	store, err := labbit.NewMemoryStore()
	if err != nil {
		t.Fatalf("NewMemoryStore() error = %v", err)
	}
	t.Cleanup(func() { _ = store.Close() })

	user := activeTestUser(t, store, "alice")
	docPub, err := labbit.Parse(strings.NewReader(labbitSample()))
	if err != nil {
		t.Fatalf("Parse() error = %v", err)
	}
	docPub.UID = "pub1"
	docPub.Title = "Public Lab Title"
	docPub.Hash = "hash-pub"
	if err := store.SaveDocument(context.Background(), docPub); err != nil {
		t.Fatalf("SaveDocument pub error = %v", err)
	}
	if err := store.SaveUserDocument(context.Background(), user.ID, docPub.ID, labbit.VisibilityPublic); err != nil {
		t.Fatalf("SaveUserDocument pub error = %v", err)
	}

	docPriv, err := labbit.Parse(strings.NewReader(labbitSample()))
	if err != nil {
		t.Fatalf("Parse() error = %v", err)
	}
	docPriv.UID = "priv1"
	docPriv.Title = "Private Lab Title"
	docPriv.Hash = "hash-priv"
	if err := store.SaveDocument(context.Background(), docPriv); err != nil {
		t.Fatalf("SaveDocument priv error = %v", err)
	}
	if err := store.SaveUserDocument(context.Background(), user.ID, docPriv.ID, labbit.VisibilityPrivate); err != nil {
		t.Fatalf("SaveUserDocument priv error = %v", err)
	}

	handler := (&Server{labs: store}).RegisterRoutes()

	req := httptest.NewRequest(http.MethodGet, "/@alice", nil)
	resp := httptest.NewRecorder()
	handler.ServeHTTP(resp, req)
	if resp.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200: %s", resp.Code, resp.Body.String())
	}
	body := resp.Body.String()
	if !strings.Contains(body, "Public Lab Title") || strings.Contains(body, "Private Lab Title") {
		t.Fatalf("body should contain public lab and not private lab: %s", body)
	}
	if strings.Contains(body, "private</small>") || strings.Contains(body, `class="user-docs-row private"`) {
		t.Fatalf("public page should not expose private visibility state: %s", body)
	}
	if strings.Contains(body, "docs-bulk-form") || strings.Contains(body, "visibility-segment") {
		t.Fatalf("public page should not contain management controls: %s", body)
	}
	if !strings.Contains(body, "user-docs-row") || !strings.Contains(body, "user-docs-arrow") {
		t.Fatalf("public page should contain user-docs-row and user-docs-arrow: %s", body)
	}

	req = httptest.NewRequest(http.MethodGet, "/@alice", nil)
	addSessionCookie(t, store, req, user.ID)
	resp = httptest.NewRecorder()
	handler.ServeHTTP(resp, req)
	if resp.Code != http.StatusOK {
		t.Fatalf("owner status = %d, want 200: %s", resp.Code, resp.Body.String())
	}
	body = resp.Body.String()
	for _, want := range []string{"Public Lab Title", "Private Lab Title", "public</small>", "private</small>", `class="user-docs-row private"`} {
		if !strings.Contains(body, want) {
			t.Fatalf("owner page missing %q: %s", want, body)
		}
	}
	if strings.Contains(body, "docs-bulk-form") || strings.Contains(body, "visibility-segment") {
		t.Fatalf("owner profile page should not contain management controls: %s", body)
	}

	req = httptest.NewRequest(http.MethodGet, "/@nonexistent", nil)
	resp = httptest.NewRecorder()
	handler.ServeHTTP(resp, req)
	if resp.Code != http.StatusNotFound {
		t.Fatalf("nonexistent user status = %d, want 404", resp.Code)
	}
}

func TestLibraryActionsUsePutAndDelete(t *testing.T) {
	store, err := labbit.NewMemoryStore()
	if err != nil {
		t.Fatalf("NewMemoryStore() error = %v", err)
	}
	t.Cleanup(func() { _ = store.Close() })
	doc, err := labbit.Parse(strings.NewReader(labbitSample()))
	if err != nil {
		t.Fatalf("Parse() error = %v", err)
	}
	doc.UID = "library1"
	doc.Hash = "library-actions"
	if err := store.SaveDocument(context.Background(), doc); err != nil {
		t.Fatalf("SaveDocument() error = %v", err)
	}
	user := activeTestUser(t, store, "alice")
	if err := store.SaveUserDocument(context.Background(), user.ID, doc.ID, labbit.VisibilityPublic); err != nil {
		t.Fatalf("SaveUserDocument() error = %v", err)
	}
	handler := (&Server{labs: store}).RegisterRoutes()

	form := url.Values{"visibility": {labbit.VisibilityPrivate}, "page": {"1"}}
	req := httptest.NewRequest(http.MethodPut, "/i/library/library1/linux-services/visibility?page=1", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("HX-Request", "true")
	addSessionCookie(t, store, req, user.ID)
	resp := httptest.NewRecorder()
	handler.ServeHTTP(resp, req)
	if resp.Code != http.StatusOK {
		t.Fatalf("PUT visibility status = %d, want 200: %s", resp.Code, resp.Body.String())
	}
	updated, err := store.GetUserDocument(context.Background(), "alice", "library1", "linux-services")
	if err != nil {
		t.Fatalf("GetUserDocument() after visibility update error = %v", err)
	}
	if updated.Visibility != labbit.VisibilityPrivate {
		t.Fatalf("visibility = %q, want private", updated.Visibility)
	}

	req = httptest.NewRequest(http.MethodDelete, "/i/library/library1/linux-services?page=1", nil)
	req.Header.Set("HX-Request", "true")
	addSessionCookie(t, store, req, user.ID)
	resp = httptest.NewRecorder()
	handler.ServeHTTP(resp, req)
	if resp.Code != http.StatusOK {
		t.Fatalf("DELETE document status = %d, want 200: %s", resp.Code, resp.Body.String())
	}
	if _, err := store.GetUserDocument(context.Background(), "alice", "library1", "linux-services"); err == nil {
		t.Fatal("DELETE document did not remove user document")
	}
}

func TestLibrarySearchFiltersDocuments(t *testing.T) {
	store, err := labbit.NewMemoryStore()
	if err != nil {
		t.Fatalf("NewMemoryStore() error = %v", err)
	}
	t.Cleanup(func() { _ = store.Close() })
	user := activeTestUser(t, store, "alice")
	linux := savedRouteDocument(t, store, "search1", "route-search-linux", "Linux Services")
	aws := savedRouteDocument(t, store, "search2", "route-search-aws", "AWS Networking")
	for _, doc := range []*labbit.Document{linux, aws} {
		if err := store.SaveUserDocument(context.Background(), user.ID, doc.ID, labbit.VisibilityPublic); err != nil {
			t.Fatalf("SaveUserDocument(%s) error = %v", doc.UID, err)
		}
	}
	handler := (&Server{labs: store}).RegisterRoutes()

	req := httptest.NewRequest(http.MethodGet, "/i/library?q=linux", nil)
	req.Header.Set("HX-Request", "true")
	addSessionCookie(t, store, req, user.ID)
	resp := httptest.NewRecorder()
	handler.ServeHTTP(resp, req)
	if resp.Code != http.StatusOK {
		t.Fatalf("GET /i/library?q=linux status = %d, want 200: %s", resp.Code, resp.Body.String())
	}
	if got := resp.Header().Get("HX-Push-Url"); got != "/i/library?q=linux" {
		t.Fatalf("HX-Push-Url = %q, want /i/library?q=linux", got)
	}
	if !strings.Contains(resp.Body.String(), "Linux Services") || strings.Contains(resp.Body.String(), "AWS Networking") {
		t.Fatalf("filtered library body = %s", resp.Body.String())
	}
}

func TestLibraryBulkDeleteUsesSelectedDocs(t *testing.T) {
	store, err := labbit.NewMemoryStore()
	if err != nil {
		t.Fatalf("NewMemoryStore() error = %v", err)
	}
	t.Cleanup(func() { _ = store.Close() })
	user := activeTestUser(t, store, "alice")
	first := savedRouteDocument(t, store, "bulk1", "route-bulk-first", "First Bulk Doc")
	second := savedRouteDocument(t, store, "bulk2", "route-bulk-second", "Second Bulk Doc")
	for _, doc := range []*labbit.Document{first, second} {
		if err := store.SaveUserDocument(context.Background(), user.ID, doc.ID, labbit.VisibilityPublic); err != nil {
			t.Fatalf("SaveUserDocument(%s) error = %v", doc.UID, err)
		}
	}
	handler := (&Server{labs: store}).RegisterRoutes()

	form := url.Values{"page": {"1"}}
	req := httptest.NewRequest(http.MethodDelete, "/i/library", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("HX-Request", "true")
	addSessionCookie(t, store, req, user.ID)
	resp := httptest.NewRecorder()
	handler.ServeHTTP(resp, req)
	if resp.Code != http.StatusOK {
		t.Fatalf("empty bulk DELETE status = %d, want 200: %s", resp.Code, resp.Body.String())
	}
	if !strings.Contains(resp.Body.String(), "Select at least one doc to delete.") {
		t.Fatalf("empty bulk DELETE missing notice: %s", resp.Body.String())
	}

	form = url.Values{"page": {"1"}, "doc": {first.UID}}
	req = httptest.NewRequest(http.MethodDelete, "/i/library", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("HX-Request", "true")
	addSessionCookie(t, store, req, user.ID)
	resp = httptest.NewRecorder()
	handler.ServeHTTP(resp, req)
	if resp.Code != http.StatusOK {
		t.Fatalf("bulk DELETE status = %d, want 200: %s", resp.Code, resp.Body.String())
	}
	if _, err := store.GetUserDocument(context.Background(), user.Username, first.UID, first.Slug); err == nil {
		t.Fatal("bulk DELETE did not remove selected document")
	}
	if _, err := store.GetUserDocument(context.Background(), user.Username, second.UID, second.Slug); err != nil {
		t.Fatalf("bulk DELETE removed unselected document: %v", err)
	}
	if strings.Contains(resp.Body.String(), "First Bulk Doc") || !strings.Contains(resp.Body.String(), "Second Bulk Doc") {
		t.Fatalf("bulk DELETE response body = %s", resp.Body.String())
	}
}

func TestDocsPathIsNotMounted(t *testing.T) {
	store, err := labbit.NewMemoryStore()
	if err != nil {
		t.Fatalf("NewMemoryStore() error = %v", err)
	}
	t.Cleanup(func() { _ = store.Close() })
	user := activeTestUser(t, store, "alice")

	req := httptest.NewRequest(http.MethodGet, "/i/docs", nil)
	addSessionCookie(t, store, req, user.ID)
	resp := httptest.NewRecorder()
	(&Server{labs: store}).RegisterRoutes().ServeHTTP(resp, req)
	if resp.Code != http.StatusNotFound {
		t.Fatalf("GET /i/docs status = %d, want 404", resp.Code)
	}
}

func TestHomeRecentDocsLinksToUserPage(t *testing.T) {
	store, err := labbit.NewMemoryStore()
	if err != nil {
		t.Fatalf("NewMemoryStore() error = %v", err)
	}
	t.Cleanup(func() { _ = store.Close() })
	user := activeTestUser(t, store, "alice")
	doc, err := labbit.Parse(strings.NewReader(labbitSample()))
	if err != nil {
		t.Fatalf("Parse() error = %v", err)
	}
	doc.UID = "recent1"
	doc.Hash = "recent-sample"
	if err := store.SaveDocument(context.Background(), doc); err != nil {
		t.Fatalf("SaveDocument() error = %v", err)
	}
	if err := store.SaveUserDocument(context.Background(), user.ID, doc.ID, labbit.VisibilityPublic); err != nil {
		t.Fatalf("SaveUserDocument() error = %v", err)
	}

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	addSessionCookie(t, store, req, user.ID)
	resp := httptest.NewRecorder()
	(&Server{labs: store}).RegisterRoutes().ServeHTTP(resp, req)
	if resp.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200: %s", resp.Code, resp.Body.String())
	}
	if !strings.Contains(resp.Body.String(), `class="secondary-btn view-all-link" href="/@alice"`) ||
		!strings.Contains(resp.Body.String(), `<span>View all</span>`) ||
		!strings.Contains(resp.Body.String(), `<path d="m13 6 6 6-6 6">`) {
		t.Fatalf("home page missing View all link: %s", resp.Body.String())
	}
}

func TestPasskeyActionsUseIDEndpoints(t *testing.T) {
	store, err := labbit.NewMemoryStore()
	if err != nil {
		t.Fatalf("NewMemoryStore() error = %v", err)
	}
	t.Cleanup(func() { _ = store.Close() })
	handler := (&Server{labs: store, publicURL: "https://labbit.example"}).RegisterRoutes()

	req := httptest.NewRequest(http.MethodGet, "/id/register?next=%2Fafter", nil)
	resp := httptest.NewRecorder()
	handler.ServeHTTP(resp, req)
	if resp.Code != http.StatusSeeOther {
		t.Fatalf("GET /id/register status = %d, want 303", resp.Code)
	}
	if got := resp.Header().Get("Location"); got != "/id/authenticate?next=%2Fafter" {
		t.Fatalf("GET /id/register redirect = %q, want /id/authenticate?next=%%2Fafter", got)
	}

	for _, path := range []string{
		"/id/authenticate?step=begin&next=%2Fafter",
		"/id/register?step=begin&next=%2Fafter",
	} {
		req = httptest.NewRequest(http.MethodPost, path, nil)
		resp = httptest.NewRecorder()
		handler.ServeHTTP(resp, req)
		if resp.Code != http.StatusOK {
			t.Fatalf("POST %s status = %d, want 200: %s", path, resp.Code, resp.Body.String())
		}
		var payload map[string]any
		if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
			t.Fatalf("POST %s JSON error = %v", path, err)
		}
		if payload["state"] == "" || payload["options"] == nil {
			t.Fatalf("POST %s payload = %#v", path, payload)
		}
	}

	req = httptest.NewRequest(http.MethodGet, "/id/authenticate?next=%2Fafter", nil)
	resp = httptest.NewRecorder()
	handler.ServeHTTP(resp, req)
	if resp.Code != http.StatusOK {
		t.Fatalf("GET /id/authenticate status = %d, want 200", resp.Code)
	}
	assertIdentityMetadata(t, resp.Body.String(), "https://labbit.example/id/authenticate")

	req = httptest.NewRequest(http.MethodPost, "/id/authenticate", nil)
	resp = httptest.NewRecorder()
	handler.ServeHTTP(resp, req)
	if resp.Code != http.StatusBadRequest {
		t.Fatalf("invalid authentication action status = %d, want 400", resp.Code)
	}

	for _, path := range []string{
		"/id/signin",
		"/id/passkey/register/begin",
		"/id/passkey/signin/begin",
	} {
		req := httptest.NewRequest(http.MethodPost, path, nil)
		resp := httptest.NewRecorder()
		handler.ServeHTTP(resp, req)
		if resp.Code != http.StatusNotFound {
			t.Fatalf("POST %s status = %d, want 404", path, resp.Code)
		}
	}
}

func TestAuthenticateGetRedirectsSignedInUserToNext(t *testing.T) {
	store, err := labbit.NewMemoryStore()
	if err != nil {
		t.Fatalf("NewMemoryStore() error = %v", err)
	}
	t.Cleanup(func() { _ = store.Close() })
	user := activeTestUser(t, store, "alice")
	handler := (&Server{labs: store, publicURL: "https://labbit.example"}).RegisterRoutes()

	req := httptest.NewRequest(http.MethodGet, "/id/authenticate?next=%2Fafter", nil)
	addSessionCookie(t, store, req, user.ID)
	resp := httptest.NewRecorder()
	handler.ServeHTTP(resp, req)
	if resp.Code != http.StatusSeeOther {
		t.Fatalf("GET /id/authenticate status = %d, want 303", resp.Code)
	}
	if got := resp.Header().Get("Location"); got != "/after" {
		t.Fatalf("GET /id/authenticate redirect = %q, want /after", got)
	}
}

func TestIDFinishResponseCarriesNextThroughOnboarding(t *testing.T) {
	user := &labbit.User{Status: labbit.UserStatusPending}
	payload := idFinishResponse(user, "/after")
	if got := payload["next"]; got != "/i/onboarding?next=%2Fafter" {
		t.Fatalf("pending finish next = %q, want onboarding next", got)
	}
	user.Status = labbit.UserStatusActive
	payload = idFinishResponse(user, "/after")
	if got := payload["next"]; got != "/after" {
		t.Fatalf("active finish next = %q, want /after", got)
	}
}

func TestSignoutGetRendersPageAndPostRevokesSession(t *testing.T) {
	store, err := labbit.NewMemoryStore()
	if err != nil {
		t.Fatalf("NewMemoryStore() error = %v", err)
	}
	t.Cleanup(func() { _ = store.Close() })
	user := activeTestUser(t, store, "alice")
	handler := (&Server{labs: store, publicURL: "https://labbit.example"}).RegisterRoutes()

	req := httptest.NewRequest(http.MethodGet, "/id/signout?next=%2Fprevious", nil)
	addSessionCookie(t, store, req, user.ID)
	sessionCookie := req.Cookies()[0]
	resp := httptest.NewRecorder()
	handler.ServeHTTP(resp, req)
	if resp.Code != http.StatusOK {
		t.Fatalf("GET /id/signout status = %d, want 200", resp.Code)
	}
	assertSignOutMetadata(t, resp.Body.String(), "https://labbit.example/id/signout")
	if !strings.Contains(resp.Body.String(), `method="post" action="/id/signout?next=%2Fprevious"`) {
		t.Fatalf("GET /id/signout did not render POST form: %s", resp.Body.String())
	}
	if strings.Contains(resp.Body.String(), `name="next"`) {
		t.Fatalf("GET /id/signout rendered body next: %s", resp.Body.String())
	}
	if !strings.Contains(resp.Body.String(), `onclick="history.back()"`) {
		t.Fatalf("GET /id/signout did not render cancel target: %s", resp.Body.String())
	}
	if _, err := store.GetUserBySession(context.Background(), sessionCookie.Value); err != nil {
		t.Fatalf("GET /id/signout revoked session: %v", err)
	}

	form := strings.NewReader("")
	req = httptest.NewRequest(http.MethodPost, "/id/signout?next=%2Fprevious", form)
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.AddCookie(sessionCookie)
	resp = httptest.NewRecorder()
	handler.ServeHTTP(resp, req)
	if resp.Code != http.StatusSeeOther {
		t.Fatalf("POST /id/signout status = %d, want 303", resp.Code)
	}
	if _, err := store.GetUserBySession(context.Background(), sessionCookie.Value); err == nil {
		t.Fatal("POST /id/signout did not revoke session")
	}
	if got := resp.Header().Get("Location"); got != "/previous" {
		t.Fatalf("POST /id/signout redirect = %q, want /previous", got)
	}
}

func TestSignoutWithoutNextUsesReferer(t *testing.T) {
	store, err := labbit.NewMemoryStore()
	if err != nil {
		t.Fatalf("NewMemoryStore() error = %v", err)
	}
	t.Cleanup(func() { _ = store.Close() })
	user := activeTestUser(t, store, "alice")
	handler := (&Server{labs: store}).RegisterRoutes()

	req := httptest.NewRequest(http.MethodGet, "/id/signout", nil)
	req.Header.Set("Referer", "http://example.com/@alice/docs/public1/linux-services")
	addSessionCookie(t, store, req, user.ID)
	sessionCookie := req.Cookies()[0]
	resp := httptest.NewRecorder()
	handler.ServeHTTP(resp, req)
	if resp.Code != http.StatusOK {
		t.Fatalf("GET /id/signout status = %d, want 200", resp.Code)
	}
	if !strings.Contains(resp.Body.String(), `method="post" action="/id/signout?next=%2F%40alice%2Fdocs%2Fpublic1%2Flinux-services"`) {
		t.Fatalf("GET /id/signout without next did not use referer as POST next: %s", resp.Body.String())
	}
	if strings.Contains(resp.Body.String(), `name="next"`) {
		t.Fatalf("GET /id/signout rendered body next from referer: %s", resp.Body.String())
	}
	if !strings.Contains(resp.Body.String(), `href="/@alice/docs/public1/linux-services"`) {
		t.Fatalf("GET /id/signout without next did not use referer as cancel target: %s", resp.Body.String())
	}

	req = httptest.NewRequest(http.MethodPost, "/id/signout", strings.NewReader(""))
	req.Header.Set("Referer", "http://example.com/@alice/docs/public1/linux-services")
	req.AddCookie(sessionCookie)
	resp = httptest.NewRecorder()
	handler.ServeHTTP(resp, req)
	if resp.Code != http.StatusSeeOther {
		t.Fatalf("POST /id/signout status = %d, want 303", resp.Code)
	}
	if got := resp.Header().Get("Location"); got != "/@alice/docs/public1/linux-services" {
		t.Fatalf("POST /id/signout without next redirect = %q, want referer", got)
	}
}

func TestPendingSessionRedirectsToOnboarding(t *testing.T) {
	store, err := labbit.NewMemoryStore()
	if err != nil {
		t.Fatalf("NewMemoryStore() error = %v", err)
	}
	t.Cleanup(func() { _ = store.Close() })
	user, err := store.CreatePendingUser(context.Background())
	if err != nil {
		t.Fatalf("CreatePendingUser() error = %v", err)
	}
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	addSessionCookie(t, store, req, user.ID)
	resp := httptest.NewRecorder()
	(&Server{labs: store}).RegisterRoutes().ServeHTTP(resp, req)
	if resp.Code != http.StatusSeeOther {
		t.Fatalf("status = %d, want 303", resp.Code)
	}
	if got := resp.Header().Get("Location"); got != "/i/onboarding" {
		t.Fatalf("Location = %q, want /i/onboarding", got)
	}
}

func TestAuthDisabledRendersWorkspaceWithoutAuthRoutes(t *testing.T) {
	store, err := labbit.NewMemoryStore()
	if err != nil {
		t.Fatalf("NewMemoryStore() error = %v", err)
	}
	t.Cleanup(func() { _ = store.Close() })
	handler := (&Server{labs: store, disableAuth: true}).RegisterRoutes()

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	resp := httptest.NewRecorder()
	handler.ServeHTTP(resp, req)
	if resp.Code != http.StatusOK {
		t.Fatalf("GET / status = %d, want 200: %s", resp.Code, resp.Body.String())
	}
	body := resp.Body.String()
	for _, want := range []string{`Upload lab`, `type="file"`} {
		if !strings.Contains(body, want) {
			t.Fatalf("auth-disabled home missing %q: %s", want, body)
		}
	}
	for _, unwanted := range []string{`Sign in with passkey`, `Create a passkey`, `/id/signout`, `@local`} {
		if strings.Contains(body, unwanted) {
			t.Fatalf("auth-disabled home rendered auth UI %q: %s", unwanted, body)
		}
	}
	if _, err := store.GetUserByUsername(context.Background(), labbit.LocalUserUsername); err != nil {
		t.Fatalf("local user was not created: %v", err)
	}

	req = httptest.NewRequest(http.MethodGet, "/i/library", nil)
	resp = httptest.NewRecorder()
	handler.ServeHTTP(resp, req)
	if resp.Code != http.StatusOK {
		t.Fatalf("GET /i/library status = %d, want 200: %s", resp.Code, resp.Body.String())
	}
	if strings.Contains(resp.Body.String(), `/id/signout`) || strings.Contains(resp.Body.String(), `@local`) {
		t.Fatalf("auth-disabled library rendered account controls: %s", resp.Body.String())
	}

	for _, path := range []string{"/id", "/id/authenticate", "/id/register", "/id/signout", "/i/onboarding"} {
		req = httptest.NewRequest(http.MethodGet, path, nil)
		resp = httptest.NewRecorder()
		handler.ServeHTTP(resp, req)
		if resp.Code != http.StatusNotFound {
			t.Fatalf("GET %s status = %d, want 404", path, resp.Code)
		}
	}
}

func TestAuthDisabledAllowsPrivateDocumentWithoutSession(t *testing.T) {
	store, err := labbit.NewMemoryStore()
	if err != nil {
		t.Fatalf("NewMemoryStore() error = %v", err)
	}
	t.Cleanup(func() { _ = store.Close() })
	doc, err := labbit.Parse(strings.NewReader(labbitSample()))
	if err != nil {
		t.Fatalf("Parse() error = %v", err)
	}
	doc.UID = "private2"
	doc.Hash = "auth-disabled-private"
	if err := store.SaveDocument(context.Background(), doc); err != nil {
		t.Fatalf("SaveDocument() error = %v", err)
	}
	owner := activeTestUser(t, store, "alice")
	if err := store.SaveUserDocument(context.Background(), owner.ID, doc.ID, labbit.VisibilityPrivate); err != nil {
		t.Fatalf("SaveUserDocument() error = %v", err)
	}
	handler := (&Server{labs: store, disableAuth: true}).RegisterRoutes()

	req := httptest.NewRequest(http.MethodGet, userDocPath("alice", "private2", "linux-services"), nil)
	resp := httptest.NewRecorder()
	handler.ServeHTTP(resp, req)
	if resp.Code != http.StatusOK {
		t.Fatalf("private document status = %d, want 200: %s", resp.Code, resp.Body.String())
	}
	if strings.Contains(resp.Body.String(), `/id/signout`) || strings.Contains(resp.Body.String(), `@local`) {
		t.Fatalf("auth-disabled document rendered auth controls: %s", resp.Body.String())
	}
}

func activeTestUser(t *testing.T, store *labbit.Store, username string) *labbit.User {
	t.Helper()
	user, err := store.CreatePendingUser(context.Background())
	if err != nil {
		t.Fatalf("CreatePendingUser() error = %v", err)
	}
	user, err = store.ActivateUser(context.Background(), user.ID, username)
	if err != nil {
		t.Fatalf("ActivateUser() error = %v", err)
	}
	return user
}

func userDocPath(username string, parts ...string) string {
	return "/@" + username + "/docs/" + strings.Join(parts, "/")
}

func addSessionCookie(t *testing.T, store *labbit.Store, req *http.Request, userID string) {
	t.Helper()
	raw, _, err := store.CreateSession(context.Background(), userID, sessionTTL)
	if err != nil {
		t.Fatalf("CreateSession() error = %v", err)
	}
	req.AddCookie(&http.Cookie{Name: sessionCookieName, Value: raw})
}

func decodeZstdBody(t *testing.T, body []byte) []byte {
	t.Helper()
	reader, err := zstd.NewReader(bytes.NewReader(body))
	if err != nil {
		t.Fatalf("zstd.NewReader() error = %v", err)
	}
	defer reader.Close()
	decoded, err := io.ReadAll(reader)
	if err != nil {
		t.Fatalf("ReadAll(zstd) error = %v", err)
	}
	return decoded
}

func headerValuesContain(values []string, target string) bool {
	for _, value := range values {
		for _, part := range strings.Split(value, ",") {
			if strings.EqualFold(strings.TrimSpace(part), target) {
				return true
			}
		}
	}
	return false
}

func assertHTMLContainsAll(t *testing.T, html string, wants []string) {
	t.Helper()
	for _, want := range wants {
		if !strings.Contains(html, want) {
			t.Fatalf("HTML missing %q: %s", want, html)
		}
	}
}

func assertIdentityMetadata(t *testing.T, html string, canonical string) {
	t.Helper()
	assertThumbnailMetadata(t, html, canonical, "Authenticate ID · Labbit", "Sign in to continue to Labbit.")
}

func assertSignOutMetadata(t *testing.T, html string, canonical string) {
	t.Helper()
	assertThumbnailMetadata(t, html, canonical, "Sign out · Labbit", "Sign out of your Labbit session.")
}

func assertThumbnailMetadata(t *testing.T, html string, canonical string, title string, description string) {
	t.Helper()
	assertHTMLContainsAll(t, html, []string{
		`<title>` + title + `</title>`,
		`<meta property="og:type" content="website">`,
		`<meta property="og:title" content="` + title + `">`,
		`<meta property="og:description" content="` + description + `">`,
		`<meta property="og:url" content="` + canonical + `">`,
		`<meta property="og:image" content="https://labbit.example/assets/img/icon-512.png">`,
		`<meta property="og:image:width" content="512">`,
		`<meta property="og:image:height" content="512">`,
		`<meta property="og:image:alt" content="Labbit icon">`,
		`<meta name="twitter:card" content="summary">`,
		`<meta name="twitter:title" content="` + title + `">`,
		`<meta name="twitter:description" content="` + description + `">`,
		`<meta name="twitter:image" content="https://labbit.example/assets/img/icon-512.png">`,
		`<link rel="canonical" href="` + canonical + `">`,
	})
}

func savedRouteDocument(t *testing.T, store *labbit.Store, uid, hash, title string) *labbit.Document {
	t.Helper()
	doc, err := labbit.Parse(strings.NewReader(labbitSample()))
	if err != nil {
		t.Fatalf("Parse() error = %v", err)
	}
	doc.UID = uid
	doc.Hash = hash
	doc.Title = title
	if err := store.SaveDocument(context.Background(), doc); err != nil {
		t.Fatalf("SaveDocument() error = %v", err)
	}
	return doc
}

func labbitSample() string {
	return `<labbit title="Linux Services Exam" slug="linux-services">
<overview>Overview</overview>
<lab>
  <topic id="samba" title="Samba">
    <task id="setup-samba" title="Setup Samba">
Install packages.
<hint title="Package">Use the Samba package.</hint>
<solution>dnf install samba</solution>
    </task>
  </topic>
</lab>
<quiz>
  <topic id="basics" title="Basics">
    <question id="daemon" type="single">
      <prompt>Which service handles SMB file sharing?</prompt>
      <option id="a" correct="true">smb</option>
      <option id="b">sshd</option>
      <explanation>smb provides SMB file shares.</explanation>
    </question>
  </topic>
</quiz>
</labbit>`
}

func TestUploadPageUsesThemeCookie(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.AddCookie(&http.Cookie{Name: "labbit.theme", Value: "light"})
	resp := httptest.NewRecorder()
	(&Server{}).RegisterRoutes().ServeHTTP(resp, req)
	if resp.Code != http.StatusOK {
		t.Fatalf("status = %d", resp.Code)
	}
	if !strings.Contains(resp.Body.String(), `data-theme="light"`) {
		t.Fatalf("upload page did not render light theme: %s", resp.Body.String())
	}
}

func TestUploadPageDefaultsInvalidThemeCookieToDark(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.AddCookie(&http.Cookie{Name: "labbit.theme", Value: "solarized"})
	resp := httptest.NewRecorder()
	(&Server{}).RegisterRoutes().ServeHTTP(resp, req)
	if resp.Code != http.StatusOK {
		t.Fatalf("status = %d", resp.Code)
	}
	if !strings.Contains(resp.Body.String(), `data-theme="dark"`) {
		t.Fatalf("upload page did not fall back to dark theme: %s", resp.Body.String())
	}
}

func TestThemeHandlerSetsCookieAndReturnsToggle(t *testing.T) {
	form := url.Values{"theme": {"light"}, "slot": {"viewer-sidebar"}}
	req := httptest.NewRequest(http.MethodPatch, "/i/theme", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("HX-Request", "true")
	resp := httptest.NewRecorder()
	(&Server{}).RegisterRoutes().ServeHTTP(resp, req)
	if resp.Code != http.StatusOK {
		t.Fatalf("status = %d", resp.Code)
	}
	var themeCookie *http.Cookie
	for _, cookie := range resp.Result().Cookies() {
		if cookie.Name == "labbit.theme" {
			themeCookie = cookie
			break
		}
	}
	if themeCookie == nil || themeCookie.Value != "light" || themeCookie.Path != "/" {
		t.Fatalf("theme cookie = %#v", themeCookie)
	}
	if got := resp.Header().Get("HX-Trigger"); !strings.Contains(got, `"theme":"light"`) {
		t.Fatalf("HX-Trigger = %q", got)
	}
	body := resp.Body.String()
	if !strings.Contains(body, `data-theme-toggle`) || !strings.Contains(body, `value="dark"`) {
		t.Fatalf("toggle fragment did not render next dark action: %s", body)
	}
	if !strings.Contains(body, `id="theme-toggle-viewer-sidebar"`) || !strings.Contains(body, `name="slot" value="viewer-sidebar"`) {
		t.Fatalf("toggle fragment did not preserve clicked slot: %s", body)
	}
	if strings.Contains(body, `id="theme-toggle-viewer-sidebar" class="theme-toggle-form" hx-patch="/i/theme" hx-target="this" hx-swap="outerHTML" hx-swap-oob`) {
		t.Fatalf("clicked toggle should not be OOB-only: %s", body)
	}
}

func TestThemeHandlerDefaultsInvalidThemeToDark(t *testing.T) {
	form := url.Values{"theme": {"sepia"}}
	req := httptest.NewRequest(http.MethodPatch, "/i/theme", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	resp := httptest.NewRecorder()
	(&Server{}).RegisterRoutes().ServeHTTP(resp, req)
	if resp.Code != http.StatusOK {
		t.Fatalf("status = %d", resp.Code)
	}
	if got := resp.Result().Cookies()[0].Value; got != "dark" {
		t.Fatalf("theme cookie = %q", got)
	}
	if !strings.Contains(resp.Body.String(), `value="light"`) {
		t.Fatalf("toggle fragment did not fall back to dark: %s", resp.Body.String())
	}
}

func TestVerbSpecificMutationRoutesDoNotAcceptPostFallbacks(t *testing.T) {
	store, err := labbit.NewMemoryStore()
	if err != nil {
		t.Fatalf("NewMemoryStore() error = %v", err)
	}
	t.Cleanup(func() { _ = store.Close() })
	doc, err := labbit.Parse(strings.NewReader(labbitSample()))
	if err != nil {
		t.Fatalf("Parse() error = %v", err)
	}
	doc.UID = "nofall1"
	doc.Hash = "no-fallbacks"
	if err := store.SaveDocument(context.Background(), doc); err != nil {
		t.Fatalf("SaveDocument() error = %v", err)
	}
	user := activeTestUser(t, store, "alice")
	if err := store.SaveUserDocument(context.Background(), user.ID, doc.ID, labbit.VisibilityPublic); err != nil {
		t.Fatalf("SaveUserDocument() error = %v", err)
	}
	handler := (&Server{labs: store}).RegisterRoutes()

	tests := []struct {
		name string
		path string
	}{
		{"theme", "/i/theme"},
		{"visibility", "/i/library/nofall1/linux-services/visibility"},
		{"delete action", "/i/library/nofall1/linux-services/delete"},
	}
	for _, tt := range tests {
		req := httptest.NewRequest(http.MethodPost, tt.path, strings.NewReader(url.Values{"theme": {"light"}, "visibility": {labbit.VisibilityPrivate}}.Encode()))
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		addSessionCookie(t, store, req, user.ID)
		resp := httptest.NewRecorder()
		handler.ServeHTTP(resp, req)
		if resp.Code == http.StatusOK || resp.Code == http.StatusSeeOther {
			t.Fatalf("POST fallback %s status = %d, want rejection", tt.name, resp.Code)
		}
	}
}
