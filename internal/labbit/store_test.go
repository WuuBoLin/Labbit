// Copyright (C) 2026 WuuBoLin
// SPDX-License-Identifier: GPL-3.0-or-later

package labbit

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestStoreRoundTrip(t *testing.T) {
	store, err := NewMemoryStore()
	if err != nil {
		t.Fatalf("NewMemoryStore() error = %v", err)
	}
	t.Cleanup(func() { _ = store.Close() })

	doc, err := Parse(strings.NewReader(sampleLab))
	if err != nil {
		t.Fatalf("Parse() error = %v", err)
	}
	doc.Hash = "sample-hash"
	doc.UID = "sample-"
	if err := store.SaveDocument(context.Background(), doc); err != nil {
		t.Fatalf("SaveDocument() error = %v", err)
	}
	got, err := store.GetDocument(context.Background(), doc.UID, doc.Slug)
	if err != nil {
		t.Fatalf("GetDocument() error = %v", err)
	}
	if len(got.Topics[0].Items[0].Hints) != 0 {
		t.Fatal("document load should not hydrate hints")
	}
	if got.Topics[0].Items[0].HintCount != 2 {
		t.Fatalf("hint count = %d", got.Topics[0].Items[0].HintCount)
	}
	if got.Topics[0].Items[0].SolutionCount != 1 {
		t.Fatalf("solution count = %d", got.Topics[0].Items[0].SolutionCount)
	}
	if got.Accent != "#ff3366" {
		t.Fatalf("accent = %q", got.Accent)
	}
	byHash, err := store.GetDocumentByHash(context.Background(), "sample-hash")
	if err != nil {
		t.Fatalf("GetDocumentByHash() error = %v", err)
	}
	if byHash.UID != doc.UID {
		t.Fatalf("hash lookup uid = %q, want %q", byHash.UID, doc.UID)
	}
	byUID, err := store.GetDocumentByUID(context.Background(), doc.UID)
	if err != nil {
		t.Fatalf("GetDocumentByUID() error = %v", err)
	}
	if byUID.Slug != doc.Slug {
		t.Fatalf("uid lookup slug = %q, want %q", byUID.Slug, doc.Slug)
	}
	duplicate, err := Parse(strings.NewReader(sampleLab))
	if err != nil {
		t.Fatalf("Parse duplicate error = %v", err)
	}
	duplicate.Hash = "sample-hash"
	duplicate.UID = "sample-"
	if err := store.SaveDocument(context.Background(), duplicate); err == nil {
		t.Fatal("expected duplicate hash save to fail")
	}
	hints, err := store.GetHints(context.Background(), got.ID, "setup-samba")
	if err != nil {
		t.Fatalf("GetHints() error = %v", err)
	}
	if len(hints.Hints) != 2 {
		t.Fatalf("hints = %#v", hints.Hints)
	}
	if hints.Hints[1].Kind != "solution" || !strings.Contains(hints.Hints[1].Body, "dnf install samba") {
		t.Fatalf("solution hint = %#v", hints.Hints[1])
	}
	inline, err := store.GetHint(context.Background(), got.ID, "setup-samba", hints.Hints[0].ID)
	if err != nil {
		t.Fatalf("GetHint() error = %v", err)
	}
	if inline.Kind != "hint" || !strings.Contains(inline.Body, "Samba package") {
		t.Fatalf("inline hint = %#v", inline)
	}
	solution, err := store.GetSolution(context.Background(), got.ID, "setup-samba")
	if err != nil {
		t.Fatalf("GetSolution() error = %v", err)
	}
	if len(solution.Hints) != 1 || solution.Hints[0].Kind != "solution" {
		t.Fatalf("solution = %#v", solution.Hints)
	}
	results, err := store.Search(context.Background(), got.ID, "samba")
	if err != nil {
		t.Fatalf("Search() error = %v", err)
	}
	if len(results) == 0 {
		t.Fatal("expected search result")
	}
	if results[0].Title != "Samba" {
		t.Fatalf("expected section title first, got %#v", results[0])
	}
}

func TestEnsureSQLiteParent(t *testing.T) {
	dsn := filepath.Join(t.TempDir(), "nested", "labbit.db")
	if err := ensureSQLiteParent(dsn); err != nil {
		t.Fatalf("ensureSQLiteParent() error = %v", err)
	}
	if _, err := os.Stat(filepath.Dir(dsn)); err != nil {
		t.Fatalf("parent directory was not created: %v", err)
	}
}
