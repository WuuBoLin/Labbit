// Copyright (C) 2026 WuuBoLin
// SPDX-License-Identifier: GPL-3.0-or-later

package server

import (
	"context"
	"labbit/internal/labbit"
	"net/http"
	"net/http/httptest"
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

	req := httptest.NewRequest(http.MethodGet, "/docs/c40a39f", nil)
	resp := httptest.NewRecorder()
	(&Server{labs: store}).RegisterRoutes().ServeHTTP(resp, req)
	if resp.Code != http.StatusMovedPermanently {
		t.Fatalf("status = %d", resp.Code)
	}
	if got, want := resp.Header().Get("Location"), "/docs/c40a39f/linux-services"; got != want {
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
