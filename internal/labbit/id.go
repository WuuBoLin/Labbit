// Copyright (C) 2026 WuuBoLin
// SPDX-License-Identifier: GPL-3.0-or-later

package labbit

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
	"strings"
	"time"

	"github.com/go-webauthn/webauthn/webauthn"
)

const (
	UserStatusPending = "pending"
	UserStatusActive  = "active"

	VisibilityPublic  = "public"
	VisibilityPrivate = "private"

	DefaultUsername   = ""
	LocalUserID       = "local"
	LocalUserUsername = "local"
)

var usernameRE = regexp.MustCompile(`^[A-Za-z0-9_]{3,30}$`)

var ErrUsernameTaken = errors.New("username already taken")

type WebAuthnUser struct {
	User
	Credentials []webauthn.Credential
}

func (u WebAuthnUser) WebAuthnID() []byte {
	return []byte(u.ID)
}

func (u WebAuthnUser) WebAuthnName() string {
	if u.Username != "" {
		return u.Username
	}
	return DefaultUsername
}

func (u WebAuthnUser) WebAuthnDisplayName() string {
	return u.WebAuthnName()
}

func (u WebAuthnUser) WebAuthnCredentials() []webauthn.Credential {
	return u.Credentials
}

func NormalizeUsername(username string) string {
	return strings.ToLower(strings.TrimSpace(username))
}

func CleanUsername(username string) string {
	return strings.TrimSpace(username)
}

func ValidateUsername(username string) error {
	if !usernameRE.MatchString(username) {
		return fmt.Errorf("username must be 3-30 characters and use letters, numbers, or underscores")
	}
	return nil
}

func NormalizeVisibility(visibility string) string {
	if strings.EqualFold(strings.TrimSpace(visibility), VisibilityPrivate) {
		return VisibilityPrivate
	}
	return VisibilityPublic
}

func NewUUID() (string, error) {
	var b [16]byte
	if _, err := rand.Read(b[:]); err != nil {
		return "", err
	}
	b[6] = (b[6] & 0x0f) | 0x40
	b[8] = (b[8] & 0x3f) | 0x80
	return fmt.Sprintf("%x-%x-%x-%x-%x", b[0:4], b[4:6], b[6:8], b[8:10], b[10:16]), nil
}

func NewToken() (string, error) {
	var b [32]byte
	if _, err := rand.Read(b[:]); err != nil {
		return "", err
	}
	return hex.EncodeToString(b[:]), nil
}

func HashToken(token string) string {
	sum := sha256.Sum256([]byte(token))
	return hex.EncodeToString(sum[:])
}

func (s *Store) CreatePendingUser(ctx context.Context) (*User, error) {
	id, err := NewUUID()
	if err != nil {
		return nil, err
	}
	now := time.Now().UTC()
	_, err = s.db.ExecContext(ctx, `INSERT INTO users(id, status, created_at, updated_at) VALUES(?,?,?,?)`, id, UserStatusPending, now.Format(time.RFC3339), now.Format(time.RFC3339))
	if err != nil {
		return nil, err
	}
	return &User{ID: id, Status: UserStatusPending, CreatedAt: now, UpdatedAt: now}, nil
}

func (s *Store) GetUser(ctx context.Context, id string) (*User, error) {
	var user User
	var username sql.NullString
	var created, updated string
	err := s.db.QueryRowContext(ctx, `SELECT id, username, status, created_at, updated_at FROM users WHERE id=?`, id).Scan(&user.ID, &username, &user.Status, &created, &updated)
	if err != nil {
		return nil, err
	}
	user.Username = username.String
	user.CreatedAt, _ = time.Parse(time.RFC3339, created)
	user.UpdatedAt, _ = time.Parse(time.RFC3339, updated)
	return &user, nil
}

func (s *Store) GetUserByUsername(ctx context.Context, username string) (*User, error) {
	username = NormalizeUsername(username)
	var user User
	var stored sql.NullString
	var created, updated string
	err := s.db.QueryRowContext(ctx, `SELECT id, username, status, created_at, updated_at FROM users WHERE username_normalized=?`, username).Scan(&user.ID, &stored, &user.Status, &created, &updated)
	if err != nil {
		return nil, err
	}
	user.Username = stored.String
	user.CreatedAt, _ = time.Parse(time.RFC3339, created)
	user.UpdatedAt, _ = time.Parse(time.RFC3339, updated)
	return &user, nil
}

func (s *Store) ActivateUser(ctx context.Context, userID, username string) (*User, error) {
	username = CleanUsername(username)
	if err := ValidateUsername(username); err != nil {
		return nil, err
	}
	normalized := NormalizeUsername(username)
	now := time.Now().UTC()
	res, err := s.db.ExecContext(ctx, `UPDATE users SET username=?, username_normalized=?, status=?, updated_at=? WHERE id=? AND username IS NULL`,
		username, normalized, UserStatusActive, now.Format(time.RFC3339), userID)
	if err != nil {
		if isUniqueConstraint(err) {
			return nil, ErrUsernameTaken
		}
		return nil, err
	}
	changed, err := res.RowsAffected()
	if err != nil {
		return nil, err
	}
	if changed == 0 {
		if _, err := s.GetUserByUsername(ctx, username); err == nil {
			return nil, ErrUsernameTaken
		}
		return nil, sql.ErrNoRows
	}
	return s.GetUser(ctx, userID)
}

func (s *Store) EnsureLocalUser(ctx context.Context) (*User, error) {
	if user, err := s.GetUserByUsername(ctx, LocalUserUsername); err == nil {
		if user.Status == UserStatusActive {
			return user, nil
		}
		return s.activateExistingUser(ctx, user.ID)
	} else if !errors.Is(err, sql.ErrNoRows) {
		return nil, err
	}

	if user, err := s.GetUser(ctx, LocalUserID); err == nil {
		if user.Status != UserStatusActive {
			if user, err = s.activateExistingUser(ctx, user.ID); err != nil {
				return nil, err
			}
		}
		if user.Username != "" {
			return user, nil
		}
		activated, err := s.ActivateUser(ctx, user.ID, LocalUserUsername)
		if errors.Is(err, ErrUsernameTaken) {
			return s.GetUserByUsername(ctx, LocalUserUsername)
		}
		return activated, err
	} else if !errors.Is(err, sql.ErrNoRows) {
		return nil, err
	}

	now := time.Now().UTC()
	_, err := s.db.ExecContext(ctx, `INSERT INTO users(id, username, username_normalized, status, created_at, updated_at) VALUES(?,?,?,?,?,?)`,
		LocalUserID, LocalUserUsername, NormalizeUsername(LocalUserUsername), UserStatusActive, now.Format(time.RFC3339), now.Format(time.RFC3339))
	if err != nil {
		if isUniqueConstraint(err) {
			return s.GetUserByUsername(ctx, LocalUserUsername)
		}
		return nil, err
	}
	return s.GetUser(ctx, LocalUserID)
}

func (s *Store) activateExistingUser(ctx context.Context, userID string) (*User, error) {
	_, err := s.db.ExecContext(ctx, `UPDATE users SET status=?, updated_at=? WHERE id=?`, UserStatusActive, time.Now().UTC().Format(time.RFC3339), userID)
	if err != nil {
		return nil, err
	}
	return s.GetUser(ctx, userID)
}

func (s *Store) CreateSession(ctx context.Context, userID string, ttl time.Duration) (raw string, expires time.Time, err error) {
	raw, err = NewToken()
	if err != nil {
		return "", time.Time{}, err
	}
	now := time.Now().UTC()
	expires = now.Add(ttl)
	_, err = s.db.ExecContext(ctx, `INSERT INTO sessions(token_hash, user_id, created_at, expires_at) VALUES(?,?,?,?)`,
		HashToken(raw), userID, now.Format(time.RFC3339), expires.Format(time.RFC3339))
	return raw, expires, err
}

func (s *Store) RevokeSession(ctx context.Context, raw string) error {
	if strings.TrimSpace(raw) == "" {
		return nil
	}
	_, err := s.db.ExecContext(ctx, `UPDATE sessions SET revoked_at=? WHERE token_hash=? AND revoked_at IS NULL`, time.Now().UTC().Format(time.RFC3339), HashToken(raw))
	return err
}

func (s *Store) RenewSession(ctx context.Context, raw string, ttl time.Duration) (newRaw string, expires time.Time, user *User, err error) {
	user, err = s.GetUserBySession(ctx, raw)
	if err != nil {
		return "", time.Time{}, nil, err
	}
	if err := s.RevokeSession(ctx, raw); err != nil {
		return "", time.Time{}, nil, err
	}
	newRaw, expires, err = s.CreateSession(ctx, user.ID, ttl)
	return newRaw, expires, user, err
}

func (s *Store) GetUserBySession(ctx context.Context, raw string) (*User, error) {
	if strings.TrimSpace(raw) == "" {
		return nil, sql.ErrNoRows
	}
	var userID, expires string
	err := s.db.QueryRowContext(ctx, `SELECT user_id, expires_at FROM sessions WHERE token_hash=? AND revoked_at IS NULL`, HashToken(raw)).Scan(&userID, &expires)
	if err != nil {
		return nil, err
	}
	expiresAt, err := time.Parse(time.RFC3339, expires)
	if err != nil {
		return nil, err
	}
	if !expiresAt.After(time.Now().UTC()) {
		_ = s.RevokeSession(ctx, raw)
		return nil, sql.ErrNoRows
	}
	return s.GetUser(ctx, userID)
}

func (s *Store) SaveIDChallenge(ctx context.Context, kind, nonce, userID string, payload []byte, ttl time.Duration) error {
	now := time.Now().UTC()
	_, err := s.db.ExecContext(ctx, `INSERT INTO auth_challenges(nonce, kind, user_id, payload, created_at, expires_at) VALUES(?,?,?,?,?,?)`,
		nonce, kind, nullable(userID), string(payload), now.Format(time.RFC3339), now.Add(ttl).Format(time.RFC3339))
	return err
}

func (s *Store) ConsumeIDChallenge(ctx context.Context, kind, nonce string) (userID string, payload []byte, err error) {
	var storedKind, expires string
	var storedUser sql.NullString
	var body string
	err = s.db.QueryRowContext(ctx, `SELECT kind, user_id, payload, expires_at FROM auth_challenges WHERE nonce=?`, nonce).Scan(&storedKind, &storedUser, &body, &expires)
	if err != nil {
		return "", nil, err
	}
	_, _ = s.db.ExecContext(ctx, `DELETE FROM auth_challenges WHERE nonce=?`, nonce)
	if storedKind != kind {
		return "", nil, sql.ErrNoRows
	}
	expiresAt, err := time.Parse(time.RFC3339, expires)
	if err != nil {
		return "", nil, err
	}
	if !expiresAt.After(time.Now().UTC()) {
		return "", nil, sql.ErrNoRows
	}
	return storedUser.String, []byte(body), nil
}

func (s *Store) SaveWebAuthnCredential(ctx context.Context, userID string, credential webauthn.Credential) error {
	body, err := json.Marshal(credential)
	if err != nil {
		return err
	}
	now := time.Now().UTC()
	_, err = s.db.ExecContext(ctx, `INSERT INTO webauthn_credentials(credential_id, user_id, credential_json, created_at, updated_at) VALUES(?,?,?,?,?)
ON CONFLICT(credential_id) DO UPDATE SET credential_json=excluded.credential_json, updated_at=excluded.updated_at`,
		hex.EncodeToString(credential.ID), userID, string(body), now.Format(time.RFC3339), now.Format(time.RFC3339))
	return err
}

func (s *Store) UpdateWebAuthnCredential(ctx context.Context, credential webauthn.Credential) error {
	body, err := json.Marshal(credential)
	if err != nil {
		return err
	}
	_, err = s.db.ExecContext(ctx, `UPDATE webauthn_credentials SET credential_json=?, updated_at=? WHERE credential_id=?`,
		string(body), time.Now().UTC().Format(time.RFC3339), hex.EncodeToString(credential.ID))
	return err
}

func (s *Store) GetWebAuthnUser(ctx context.Context, userID string) (*WebAuthnUser, error) {
	user, err := s.GetUser(ctx, userID)
	if err != nil {
		return nil, err
	}
	credentials, err := s.GetWebAuthnCredentials(ctx, userID)
	if err != nil {
		return nil, err
	}
	return &WebAuthnUser{User: *user, Credentials: credentials}, nil
}

func (s *Store) GetWebAuthnCredentials(ctx context.Context, userID string) ([]webauthn.Credential, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT credential_json FROM webauthn_credentials WHERE user_id=?`, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var credentials []webauthn.Credential
	for rows.Next() {
		var body string
		if err := rows.Scan(&body); err != nil {
			return nil, err
		}
		var credential webauthn.Credential
		if err := json.Unmarshal([]byte(body), &credential); err != nil {
			return nil, err
		}
		credentials = append(credentials, credential)
	}
	return credentials, rows.Err()
}

func (s *Store) GetWebAuthnUserByCredential(ctx context.Context, credentialID []byte, userHandle []byte) (*WebAuthnUser, error) {
	var userID string
	err := s.db.QueryRowContext(ctx, `SELECT user_id FROM webauthn_credentials WHERE credential_id=?`, hex.EncodeToString(credentialID)).Scan(&userID)
	if err != nil {
		return nil, err
	}
	if len(userHandle) > 0 && string(userHandle) != userID {
		return nil, sql.ErrNoRows
	}
	return s.GetWebAuthnUser(ctx, userID)
}

func (s *Store) GetOIDCIdentity(ctx context.Context, provider, subject string) (*User, error) {
	var userID string
	err := s.db.QueryRowContext(ctx, `SELECT user_id FROM oidc_identities WHERE provider=? AND subject=?`, provider, subject).Scan(&userID)
	if err != nil {
		return nil, err
	}
	return s.GetUser(ctx, userID)
}

func (s *Store) LinkOIDCIdentity(ctx context.Context, userID, provider, subject, usernameClaim string) error {
	now := time.Now().UTC()
	_, err := s.db.ExecContext(ctx, `INSERT INTO oidc_identities(provider, subject, user_id, username_claim, created_at, updated_at) VALUES(?,?,?,?,?,?)
ON CONFLICT(provider, subject) DO UPDATE SET user_id=excluded.user_id, username_claim=excluded.username_claim, updated_at=excluded.updated_at`,
		provider, subject, userID, nullable(usernameClaim), now.Format(time.RFC3339), now.Format(time.RFC3339))
	return err
}

func (s *Store) SaveUserDocument(ctx context.Context, userID string, docID int64, visibility string) error {
	now := time.Now().UTC()
	visibility = NormalizeVisibility(visibility)
	_, err := s.db.ExecContext(ctx, `INSERT INTO user_documents(user_id, document_id, visibility, uploaded_at) VALUES(?,?,?,?)
ON CONFLICT(user_id, document_id) DO UPDATE SET visibility=excluded.visibility, uploaded_at=excluded.uploaded_at`,
		userID, docID, visibility, now.Format(time.RFC3339))
	return err
}

func (s *Store) GetRecentDocuments(ctx context.Context, userID string, page, perPage int) ([]RecentDocument, error) {
	if perPage < 1 || perPage > 50 {
		perPage = 10
	}
	return s.ListUserDocumentsFiltered(ctx, userID, page, perPage, "")
}

func (s *Store) ListUserDocuments(ctx context.Context, userID string, page, perPage int) ([]RecentDocument, error) {
	return s.ListUserDocumentsFiltered(ctx, userID, page, perPage, "")
}

func (s *Store) ListUserDocumentsFiltered(ctx context.Context, userID string, page, perPage int, query string) ([]RecentDocument, error) {
	if page < 1 {
		page = 1
	}
	if perPage < 1 || perPage > 50 {
		perPage = 20
	}
	query = strings.TrimSpace(query)
	filter := ""
	args := []any{userID}
	if query != "" {
		filter = ` AND LOWER(d.title) LIKE ? ESCAPE '\'`
		args = append(args, "%"+escapeLike(strings.ToLower(query))+"%")
	}
	args = append(args, perPage, (page-1)*perPage)
	rows, err := s.db.QueryContext(ctx, `
SELECT d.id, d.uid, d.slug, d.title, d.overview_html, d.accent, COALESCE(d.content_hash, ''), d.created_at, ud.visibility, ud.uploaded_at, u.username
FROM user_documents ud
JOIN documents d ON d.id=ud.document_id
JOIN users u ON u.id=ud.user_id
WHERE ud.user_id=?
`+filter+`
ORDER BY ud.uploaded_at DESC
LIMIT ? OFFSET ?`, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var docs []RecentDocument
	for rows.Next() {
		doc := &Document{OwnerID: userID}
		var created, uploaded string
		var username sql.NullString
		var recent RecentDocument
		if err := rows.Scan(&doc.ID, &doc.UID, &doc.Slug, &doc.Title, &doc.Overview, &doc.Accent, &doc.Hash, &created, &recent.Visibility, &uploaded, &username); err != nil {
			return nil, err
		}
		doc.CreatedAt, _ = time.Parse(time.RFC3339, created)
		recent.UploadedAt, _ = time.Parse(time.RFC3339, uploaded)
		doc.UploadedAt = recent.UploadedAt
		doc.Visibility = recent.Visibility
		doc.OwnerName = username.String
		recent.Document = doc
		docs = append(docs, recent)
	}
	return docs, rows.Err()
}

func (s *Store) ListPublicUserDocumentsFiltered(ctx context.Context, userID string, page, perPage int, query string) ([]RecentDocument, error) {
	if page < 1 {
		page = 1
	}
	if perPage < 1 || perPage > 50 {
		perPage = 20
	}
	query = strings.TrimSpace(query)
	filter := ""
	args := []any{userID, VisibilityPublic}
	if query != "" {
		filter = ` AND LOWER(d.title) LIKE ? ESCAPE '\'`
		args = append(args, "%"+escapeLike(strings.ToLower(query))+"%")
	}
	args = append(args, perPage, (page-1)*perPage)
	rows, err := s.db.QueryContext(ctx, `
SELECT d.id, d.uid, d.slug, d.title, d.overview_html, d.accent, COALESCE(d.content_hash, ''), d.created_at, ud.visibility, ud.uploaded_at, u.username
FROM user_documents ud
JOIN documents d ON d.id=ud.document_id
JOIN users u ON u.id=ud.user_id
WHERE ud.user_id=? AND ud.visibility=?
`+filter+`
ORDER BY ud.uploaded_at DESC
LIMIT ? OFFSET ?`, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var docs []RecentDocument
	for rows.Next() {
		doc := &Document{OwnerID: userID}
		var created, uploaded string
		var username sql.NullString
		var recent RecentDocument
		if err := rows.Scan(&doc.ID, &doc.UID, &doc.Slug, &doc.Title, &doc.Overview, &doc.Accent, &doc.Hash, &created, &recent.Visibility, &uploaded, &username); err != nil {
			return nil, err
		}
		doc.CreatedAt, _ = time.Parse(time.RFC3339, created)
		recent.UploadedAt, _ = time.Parse(time.RFC3339, uploaded)
		doc.UploadedAt = recent.UploadedAt
		doc.Visibility = recent.Visibility
		doc.OwnerName = username.String
		recent.Document = doc
		docs = append(docs, recent)
	}
	return docs, rows.Err()
}

func escapeLike(value string) string {
	replacer := strings.NewReplacer(`\`, `\\`, `%`, `\%`, `_`, `\_`)
	return replacer.Replace(value)
}

func (s *Store) SetUserDocumentVisibility(ctx context.Context, userID, uid, slug, visibility string) error {
	res, err := s.db.ExecContext(ctx, `
UPDATE user_documents
SET visibility=?
WHERE user_id=?
  AND document_id=(SELECT id FROM documents WHERE uid=? AND slug=?)`,
		NormalizeVisibility(visibility), userID, uid, slug)
	if err != nil {
		return err
	}
	changed, err := res.RowsAffected()
	if err != nil {
		return err
	}
	if changed == 0 {
		return ErrNotFound
	}
	return nil
}

func (s *Store) DeleteUserDocument(ctx context.Context, userID, uid, slug string) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()

	var docID int64
	err = tx.QueryRowContext(ctx, `
SELECT d.id
FROM user_documents ud
JOIN documents d ON d.id=ud.document_id
WHERE ud.user_id=? AND d.uid=? AND d.slug=?`, userID, uid, slug).Scan(&docID)
	if errors.Is(err, sql.ErrNoRows) {
		return ErrNotFound
	}
	if err != nil {
		return err
	}

	res, err := tx.ExecContext(ctx, `DELETE FROM user_documents WHERE user_id=? AND document_id=?`, userID, docID)
	if err != nil {
		return err
	}
	changed, err := res.RowsAffected()
	if err != nil {
		return err
	}
	if changed == 0 {
		return ErrNotFound
	}

	var references int
	if err := tx.QueryRowContext(ctx, `SELECT COUNT(*) FROM user_documents WHERE document_id=?`, docID).Scan(&references); err != nil {
		return err
	}
	if references > 0 {
		return tx.Commit()
	}

	for _, query := range []string{
		`DELETE FROM options WHERE question_id IN (SELECT id FROM questions WHERE document_id=?)`,
		`DELETE FROM questions WHERE document_id=?`,
		`DELETE FROM task_hints WHERE document_id=?`,
		`DELETE FROM tasks WHERE document_id=?`,
		`DELETE FROM topics WHERE document_id=?`,
		`DELETE FROM search_entries WHERE document_id=?`,
		`DELETE FROM documents WHERE id=?`,
	} {
		if _, err := tx.ExecContext(ctx, query, docID); err != nil {
			return err
		}
	}
	return tx.Commit()
}

func (s *Store) DeleteUserDocuments(ctx context.Context, userID string, uids []string) (int, error) {
	seen := map[string]bool{}
	deleted := 0
	for _, uid := range uids {
		uid = strings.TrimSpace(uid)
		if uid == "" || seen[uid] {
			continue
		}
		seen[uid] = true

		var slug string
		err := s.db.QueryRowContext(ctx, `
SELECT d.slug
FROM user_documents ud
JOIN documents d ON d.id=ud.document_id
WHERE ud.user_id=? AND d.uid=?`, userID, uid).Scan(&slug)
		if errors.Is(err, sql.ErrNoRows) {
			continue
		}
		if err != nil {
			return deleted, err
		}
		if err := s.DeleteUserDocument(ctx, userID, uid, slug); err != nil {
			if errors.Is(err, ErrNotFound) {
				continue
			}
			return deleted, err
		}
		deleted++
	}
	return deleted, nil
}

func (s *Store) GetUserDocument(ctx context.Context, username, uid, slug string) (*Document, error) {
	user, err := s.GetUserByUsername(ctx, username)
	if err != nil {
		return nil, err
	}
	doc, err := s.GetDocument(ctx, uid, slug)
	if err != nil {
		return nil, err
	}
	var visibility, uploaded string
	err = s.db.QueryRowContext(ctx, `SELECT visibility, uploaded_at FROM user_documents WHERE user_id=? AND document_id=?`, user.ID, doc.ID).Scan(&visibility, &uploaded)
	if err != nil {
		return nil, err
	}
	doc.OwnerID = user.ID
	doc.OwnerName = user.Username
	doc.Visibility = visibility
	doc.UploadedAt, _ = time.Parse(time.RFC3339, uploaded)
	return doc, nil
}

func nullable(value string) any {
	if strings.TrimSpace(value) == "" {
		return nil
	}
	return value
}

func isUniqueConstraint(err error) bool {
	if err == nil {
		return false
	}
	return strings.Contains(strings.ToLower(err.Error()), "unique constraint")
}
