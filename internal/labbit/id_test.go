// Copyright (C) 2026 WuuBoLin
// SPDX-License-Identifier: GPL-3.0-or-later

package labbit

import (
	"context"
	"database/sql"
	"errors"
	"strings"
	"testing"
	"time"
)

func TestIDUserSessionAndDocumentLibrary(t *testing.T) {
	store, err := NewMemoryStore()
	if err != nil {
		t.Fatalf("NewMemoryStore() error = %v", err)
	}
	t.Cleanup(func() { _ = store.Close() })

	user, err := store.CreatePendingUser(context.Background())
	if err != nil {
		t.Fatalf("CreatePendingUser() error = %v", err)
	}
	if user.Status != UserStatusPending || user.Username != "" {
		t.Fatalf("pending user = %#v", user)
	}
	user, err = store.ActivateUser(context.Background(), user.ID, "Alice_1")
	if err != nil {
		t.Fatalf("ActivateUser() error = %v", err)
	}
	if user.Username != "Alice_1" || user.Status != UserStatusActive {
		t.Fatalf("active user = %#v", user)
	}
	other, err := store.CreatePendingUser(context.Background())
	if err != nil {
		t.Fatalf("CreatePendingUser() other error = %v", err)
	}
	if _, err := store.ActivateUser(context.Background(), other.ID, "alice_1"); !errors.Is(err, ErrUsernameTaken) {
		t.Fatalf("duplicate username error = %v, want ErrUsernameTaken", err)
	}

	raw, _, err := store.CreateSession(context.Background(), user.ID, time.Hour)
	if err != nil {
		t.Fatalf("CreateSession() error = %v", err)
	}
	got, err := store.GetUserBySession(context.Background(), raw)
	if err != nil {
		t.Fatalf("GetUserBySession() error = %v", err)
	}
	if got.ID != user.ID {
		t.Fatalf("session user = %q, want %q", got.ID, user.ID)
	}
	renewed, _, _, err := store.RenewSession(context.Background(), raw, time.Hour)
	if err != nil {
		t.Fatalf("RenewSession() error = %v", err)
	}
	if _, err := store.GetUserBySession(context.Background(), raw); err == nil {
		t.Fatal("old session should be revoked after renewal")
	}
	if _, err := store.GetUserBySession(context.Background(), renewed); err != nil {
		t.Fatalf("renewed session error = %v", err)
	}

	doc, err := Parse(strings.NewReader(sampleLab))
	if err != nil {
		t.Fatalf("Parse() error = %v", err)
	}
	doc.UID = "authdoc"
	doc.Hash = "id-doc-hash"
	if err := store.SaveDocument(context.Background(), doc); err != nil {
		t.Fatalf("SaveDocument() error = %v", err)
	}
	if err := store.SaveUserDocument(context.Background(), user.ID, doc.ID, VisibilityPublic); err != nil {
		t.Fatalf("SaveUserDocument(public) error = %v", err)
	}
	if err := store.SaveUserDocument(context.Background(), user.ID, doc.ID, VisibilityPrivate); err != nil {
		t.Fatalf("SaveUserDocument(private) error = %v", err)
	}
	recent, err := store.GetRecentDocuments(context.Background(), user.ID, 1, 10)
	if err != nil {
		t.Fatalf("GetRecentDocuments() error = %v", err)
	}
	if len(recent) != 1 || recent[0].Visibility != VisibilityPrivate {
		t.Fatalf("recent docs = %#v", recent)
	}
	owned, err := store.GetUserDocument(context.Background(), user.Username, doc.UID, doc.Slug)
	if err != nil {
		t.Fatalf("GetUserDocument() error = %v", err)
	}
	if owned.OwnerName != user.Username || owned.Visibility != VisibilityPrivate {
		t.Fatalf("owned doc = %#v", owned)
	}
}

func TestDeleteUserDocumentOnlyPrunesOrphanDocument(t *testing.T) {
	store, err := NewMemoryStore()
	if err != nil {
		t.Fatalf("NewMemoryStore() error = %v", err)
	}
	t.Cleanup(func() { _ = store.Close() })

	alice := activeStoreUser(t, store, "alice")
	bob := activeStoreUser(t, store, "bob")
	doc := savedStoreDocument(t, store, "shared1", "shared-hash")
	if err := store.SaveUserDocument(context.Background(), alice.ID, doc.ID, VisibilityPublic); err != nil {
		t.Fatalf("SaveUserDocument(alice) error = %v", err)
	}
	if err := store.SaveUserDocument(context.Background(), bob.ID, doc.ID, VisibilityPrivate); err != nil {
		t.Fatalf("SaveUserDocument(bob) error = %v", err)
	}

	if err := store.DeleteUserDocument(context.Background(), alice.ID, doc.UID, doc.Slug); err != nil {
		t.Fatalf("DeleteUserDocument(alice) error = %v", err)
	}
	if _, err := store.GetUserDocument(context.Background(), alice.Username, doc.UID, doc.Slug); err == nil {
		t.Fatal("alice document association still exists")
	}
	if _, err := store.GetUserDocument(context.Background(), bob.Username, doc.UID, doc.Slug); err != nil {
		t.Fatalf("bob document association was removed: %v", err)
	}
	if _, err := store.GetDocument(context.Background(), doc.UID, doc.Slug); err != nil {
		t.Fatalf("shared document was pruned too early: %v", err)
	}

	if err := store.DeleteUserDocument(context.Background(), bob.ID, doc.UID, doc.Slug); err != nil {
		t.Fatalf("DeleteUserDocument(bob) error = %v", err)
	}
	if _, err := store.GetDocument(context.Background(), doc.UID, doc.Slug); !errors.Is(err, sql.ErrNoRows) {
		t.Fatalf("GetDocument() error = %v, want sql.ErrNoRows", err)
	}
	for _, table := range []string{"documents", "topics", "tasks", "task_hints", "questions", "options", "search_entries"} {
		if got := tableRowCount(t, store, table); got != 0 {
			t.Fatalf("%s rows = %d, want 0", table, got)
		}
	}
	if err := store.DeleteUserDocument(context.Background(), bob.ID, doc.UID, doc.Slug); !errors.Is(err, ErrNotFound) {
		t.Fatalf("DeleteUserDocument(missing) error = %v, want ErrNotFound", err)
	}
}

func TestListUserDocumentsFilteredSearchesEscapedTitles(t *testing.T) {
	store, err := NewMemoryStore()
	if err != nil {
		t.Fatalf("NewMemoryStore() error = %v", err)
	}
	t.Cleanup(func() { _ = store.Close() })

	user := activeStoreUser(t, store, "alice")
	docs := []*Document{
		savedTitledStoreDocument(t, store, "linux1", "search-linux", "Linux Services"),
		savedTitledStoreDocument(t, store, "percent1", "search-percent", "100% Coverage"),
		savedTitledStoreDocument(t, store, "under1", "search-under", "Under_score Notes"),
	}
	for _, doc := range docs {
		if err := store.SaveUserDocument(context.Background(), user.ID, doc.ID, VisibilityPublic); err != nil {
			t.Fatalf("SaveUserDocument(%s) error = %v", doc.UID, err)
		}
	}

	tests := []struct {
		query string
		want  string
	}{
		{"linux", "Linux Services"},
		{"%", "100% Coverage"},
		{"_", "Under_score Notes"},
	}
	for _, tt := range tests {
		got, err := store.ListUserDocumentsFiltered(context.Background(), user.ID, 1, 10, tt.query)
		if err != nil {
			t.Fatalf("ListUserDocumentsFiltered(%q) error = %v", tt.query, err)
		}
		if len(got) != 1 || got[0].Document.Title != tt.want {
			t.Fatalf("ListUserDocumentsFiltered(%q) = %#v, want title %q", tt.query, got, tt.want)
		}
	}
}

func TestListPublicUserDocumentsFiltered(t *testing.T) {
	store, err := NewMemoryStore()
	if err != nil {
		t.Fatalf("NewMemoryStore() error = %v", err)
	}
	t.Cleanup(func() { _ = store.Close() })

	user := activeStoreUser(t, store, "alice")
	pubDoc1 := savedTitledStoreDocument(t, store, "pub1", "pub-slug1", "Public Doc 1")
	pubDoc2 := savedTitledStoreDocument(t, store, "pub2", "pub-slug2", "Public Doc 2")
	privDoc := savedTitledStoreDocument(t, store, "priv1", "priv-slug", "Private Doc")

	if err := store.SaveUserDocument(context.Background(), user.ID, pubDoc1.ID, VisibilityPublic); err != nil {
		t.Fatalf("SaveUserDocument pub1 error = %v", err)
	}
	if err := store.SaveUserDocument(context.Background(), user.ID, pubDoc2.ID, VisibilityPublic); err != nil {
		t.Fatalf("SaveUserDocument pub2 error = %v", err)
	}
	if err := store.SaveUserDocument(context.Background(), user.ID, privDoc.ID, VisibilityPrivate); err != nil {
		t.Fatalf("SaveUserDocument priv1 error = %v", err)
	}

	got, err := store.ListPublicUserDocumentsFiltered(context.Background(), user.ID, 1, 10, "")
	if err != nil {
		t.Fatalf("ListPublicUserDocumentsFiltered error = %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("ListPublicUserDocumentsFiltered len = %d, want 2", len(got))
	}
}

func TestDeleteUserDocumentsDeletesOwnedUIDs(t *testing.T) {
	store, err := NewMemoryStore()
	if err != nil {
		t.Fatalf("NewMemoryStore() error = %v", err)
	}
	t.Cleanup(func() { _ = store.Close() })

	alice := activeStoreUser(t, store, "alice")
	bob := activeStoreUser(t, store, "bob")
	first := savedTitledStoreDocument(t, store, "bulk1", "bulk-first", "First")
	second := savedTitledStoreDocument(t, store, "bulk2", "bulk-second", "Second")
	bobOnly := savedTitledStoreDocument(t, store, "bulk3", "bulk-bob", "Bob Only")
	for _, doc := range []*Document{first, second} {
		if err := store.SaveUserDocument(context.Background(), alice.ID, doc.ID, VisibilityPublic); err != nil {
			t.Fatalf("SaveUserDocument(alice %s) error = %v", doc.UID, err)
		}
	}
	if err := store.SaveUserDocument(context.Background(), bob.ID, bobOnly.ID, VisibilityPublic); err != nil {
		t.Fatalf("SaveUserDocument(bob) error = %v", err)
	}

	deleted, err := store.DeleteUserDocuments(context.Background(), alice.ID, []string{first.UID, first.UID, bobOnly.UID, "missing"})
	if err != nil {
		t.Fatalf("DeleteUserDocuments() error = %v", err)
	}
	if deleted != 1 {
		t.Fatalf("deleted = %d, want 1", deleted)
	}
	if _, err := store.GetUserDocument(context.Background(), alice.Username, first.UID, first.Slug); err == nil {
		t.Fatal("first document association still exists")
	}
	if _, err := store.GetUserDocument(context.Background(), alice.Username, second.UID, second.Slug); err != nil {
		t.Fatalf("second document association was removed: %v", err)
	}
	if _, err := store.GetUserDocument(context.Background(), bob.Username, bobOnly.UID, bobOnly.Slug); err != nil {
		t.Fatalf("bob document association was removed: %v", err)
	}
}

func activeStoreUser(t *testing.T, store *Store, username string) *User {
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

func savedStoreDocument(t *testing.T, store *Store, uid, hash string) *Document {
	t.Helper()
	return savedTitledStoreDocument(t, store, uid, hash, "")
}

func savedTitledStoreDocument(t *testing.T, store *Store, uid, hash, title string) *Document {
	t.Helper()
	doc, err := Parse(strings.NewReader(sampleLab))
	if err != nil {
		t.Fatalf("Parse() error = %v", err)
	}
	doc.UID = uid
	doc.Hash = hash
	if title != "" {
		doc.Title = title
	}
	if err := store.SaveDocument(context.Background(), doc); err != nil {
		t.Fatalf("SaveDocument() error = %v", err)
	}
	return doc
}

func tableRowCount(t *testing.T, store *Store, table string) int {
	t.Helper()
	var count int
	if err := store.db.QueryRowContext(context.Background(), "SELECT COUNT(*) FROM "+table).Scan(&count); err != nil {
		t.Fatalf("count %s: %v", table, err)
	}
	return count
}
