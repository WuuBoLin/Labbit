// Copyright (C) 2026 WuuBoLin
// SPDX-License-Identifier: GPL-3.0-or-later

package server

import (
	"context"
	"labbit/internal/labbit"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
)

func TestShortHashUID(t *testing.T) {
	hash := fileHash([]byte("labbit"))
	if got := shortHashUID(hash); got != hash[:7] {
		t.Fatalf("shortHashUID() = %q, want %q", got, hash[:7])
	}
}

func TestDocUIDRedirect(t *testing.T) {
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
	if resp.Code != http.StatusMovedPermanently {
		t.Fatalf("status = %d", resp.Code)
	}
	if got, want := resp.Header().Get("Location"), docRoute("c40a39f", "linux-services"); got != want {
		t.Fatalf("Location = %q, want %q", got, want)
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
	handler := (&Server{labs: store}).RegisterRoutes()

	for _, tc := range []struct {
		method string
		path   string
		status int
	}{
		{http.MethodGet, docRoute("c40a39f", "linux-services", "labs", "samba"), http.StatusOK},
		{http.MethodGet, docRoute("c40a39f", "linux-services", "quiz", "basics"), http.StatusOK},
		{http.MethodGet, docRoute("c40a39f", "linux-services", "keys", "labs", "setup-samba"), http.StatusOK},
		{http.MethodGet, docRoute("c40a39f", "linux-services", "keys", "setup-samba"), http.StatusNotFound},
		{http.MethodGet, docRoute("c40a39f", "linux-services", "answers", "setup-samba"), http.StatusNotFound},
		{http.MethodGet, docRoute("c40a39f", "linux-services", "section", "samba"), http.StatusNotFound},
	} {
		req := httptest.NewRequest(tc.method, tc.path, nil)
		resp := httptest.NewRecorder()
		handler.ServeHTTP(resp, req)
		if resp.Code != tc.status {
			t.Fatalf("%s %s status = %d, want %d", tc.method, tc.path, resp.Code, tc.status)
		}
	}
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
	form := url.Values{"theme": {"light"}}
	req := httptest.NewRequest(http.MethodPost, "/i/theme", strings.NewReader(form.Encode()))
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
}

func TestThemeHandlerDefaultsInvalidThemeToDark(t *testing.T) {
	form := url.Values{"theme": {"sepia"}}
	req := httptest.NewRequest(http.MethodPost, "/i/theme", strings.NewReader(form.Encode()))
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
