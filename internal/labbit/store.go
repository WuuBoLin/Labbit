// Copyright (C) 2026 WuuBoLin
// SPDX-License-Identifier: GPL-3.0-or-later

package labbit

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	_ "github.com/mattn/go-sqlite3"
)

type Store struct {
	db *sql.DB
}

func NewStoreFromEnv() (*Store, error) {
	dsn := os.Getenv("DB_URL")
	if strings.TrimSpace(dsn) == "" {
		dsn = "labbit.db"
	}
	return NewStore(dsn)
}

func NewStore(dsn string) (*Store, error) {
	if err := ensureSQLiteParent(dsn); err != nil {
		return nil, err
	}
	db, err := sql.Open("sqlite3", dsn)
	if err != nil {
		return nil, err
	}
	db.SetMaxOpenConns(1)
	store := &Store{db: db}
	return store, store.Migrate(context.Background())
}

func ensureSQLiteParent(dsn string) error {
	path := strings.TrimSpace(dsn)
	if path == "" || path == ":memory:" {
		return nil
	}
	if strings.HasPrefix(path, "file:") {
		path = strings.TrimPrefix(path, "file:")
		if i := strings.Index(path, "?"); i >= 0 {
			path = path[:i]
		}
		if path == "" || path == ":memory:" {
			return nil
		}
	}
	dir := filepath.Dir(path)
	if dir == "." || dir == "" {
		return nil
	}
	return os.MkdirAll(dir, 0o755)
}

func NewMemoryStore() (*Store, error) {
	db, err := sql.Open("sqlite3", ":memory:")
	if err != nil {
		return nil, err
	}
	db.SetMaxOpenConns(1)
	store := &Store{db: db}
	return store, store.Migrate(context.Background())
}

func (s *Store) Close() error {
	return s.db.Close()
}

func (s *Store) Ping(ctx context.Context) error {
	return s.db.PingContext(ctx)
}

func (s *Store) Migrate(ctx context.Context) error {
	_, err := s.db.ExecContext(ctx, `
CREATE TABLE IF NOT EXISTS documents (
	id INTEGER PRIMARY KEY AUTOINCREMENT,
	uid TEXT NOT NULL UNIQUE,
	slug TEXT NOT NULL,
	title TEXT NOT NULL,
	overview_html TEXT NOT NULL,
	accent TEXT NOT NULL DEFAULT '#1d9bf0',
	content_hash TEXT,
	created_at TEXT NOT NULL
);
CREATE TABLE IF NOT EXISTS topics (
	id INTEGER PRIMARY KEY AUTOINCREMENT,
	document_id INTEGER NOT NULL,
	topic_key TEXT NOT NULL,
	kind TEXT NOT NULL,
	title TEXT NOT NULL,
	position INTEGER NOT NULL,
	UNIQUE(document_id, topic_key, kind)
);
CREATE TABLE IF NOT EXISTS tasks (
	id INTEGER PRIMARY KEY AUTOINCREMENT,
	document_id INTEGER NOT NULL,
	topic_key TEXT NOT NULL,
	task_key TEXT NOT NULL,
	title TEXT NOT NULL,
	prompt_html TEXT NOT NULL,
	answer_html TEXT NOT NULL,
	position INTEGER NOT NULL,
	UNIQUE(document_id, task_key)
);
CREATE TABLE IF NOT EXISTS task_hints (
	id INTEGER PRIMARY KEY AUTOINCREMENT,
	document_id INTEGER NOT NULL,
	task_key TEXT NOT NULL,
	hint_key TEXT NOT NULL,
	kind TEXT NOT NULL DEFAULT 'hint',
	title TEXT NOT NULL,
	body_html TEXT NOT NULL,
	position INTEGER NOT NULL,
	UNIQUE(document_id, task_key, hint_key)
);
CREATE TABLE IF NOT EXISTS questions (
	id INTEGER PRIMARY KEY AUTOINCREMENT,
	document_id INTEGER NOT NULL,
	topic_key TEXT NOT NULL,
	topic_title TEXT NOT NULL,
	question_key TEXT NOT NULL,
	kind TEXT NOT NULL,
	prompt_html TEXT NOT NULL,
	explanation_html TEXT NOT NULL,
	position INTEGER NOT NULL,
	UNIQUE(document_id, question_key)
);
CREATE TABLE IF NOT EXISTS options (
	id INTEGER PRIMARY KEY AUTOINCREMENT,
	question_id INTEGER NOT NULL,
	option_key TEXT NOT NULL,
	label TEXT NOT NULL,
	correct INTEGER NOT NULL,
	position INTEGER NOT NULL
);
CREATE TABLE IF NOT EXISTS search_entries (
	id INTEGER PRIMARY KEY AUTOINCREMENT,
	document_id INTEGER NOT NULL,
	kind TEXT NOT NULL,
	target_key TEXT NOT NULL,
	section_key TEXT NOT NULL,
	title TEXT NOT NULL,
	body TEXT NOT NULL
);
CREATE TABLE IF NOT EXISTS users (
	id TEXT PRIMARY KEY,
	username TEXT UNIQUE,
	username_normalized TEXT UNIQUE,
	status TEXT NOT NULL DEFAULT 'pending',
	created_at TEXT NOT NULL,
	updated_at TEXT NOT NULL
);
CREATE TABLE IF NOT EXISTS sessions (
	token_hash TEXT PRIMARY KEY,
	user_id TEXT NOT NULL,
	created_at TEXT NOT NULL,
	expires_at TEXT NOT NULL,
	revoked_at TEXT
);
CREATE TABLE IF NOT EXISTS webauthn_credentials (
	credential_id TEXT PRIMARY KEY,
	user_id TEXT NOT NULL,
	credential_json TEXT NOT NULL,
	created_at TEXT NOT NULL,
	updated_at TEXT NOT NULL
);
CREATE TABLE IF NOT EXISTS oidc_identities (
	provider TEXT NOT NULL,
	subject TEXT NOT NULL,
	user_id TEXT NOT NULL,
	username_claim TEXT,
	created_at TEXT NOT NULL,
	updated_at TEXT NOT NULL,
	PRIMARY KEY(provider, subject)
);
CREATE TABLE IF NOT EXISTS auth_challenges (
	nonce TEXT PRIMARY KEY,
	kind TEXT NOT NULL,
	user_id TEXT,
	payload TEXT NOT NULL,
	created_at TEXT NOT NULL,
	expires_at TEXT NOT NULL
);
CREATE TABLE IF NOT EXISTS user_documents (
	user_id TEXT NOT NULL,
	document_id INTEGER NOT NULL,
	visibility TEXT NOT NULL DEFAULT 'public',
	uploaded_at TEXT NOT NULL,
	PRIMARY KEY(user_id, document_id)
);
`)
	if err != nil {
		return err
	}
	if err := s.addColumnIfMissing(ctx, "documents", "accent", "TEXT NOT NULL DEFAULT '#1d9bf0'"); err != nil {
		return err
	}
	if err := s.addColumnIfMissing(ctx, "documents", "content_hash", "TEXT"); err != nil {
		return err
	}
	if err := s.addUniqueIndexIfMissing(ctx, "idx_documents_content_hash", "documents", "content_hash"); err != nil {
		return err
	}
	if err := s.addColumnIfMissing(ctx, "task_hints", "kind", "TEXT NOT NULL DEFAULT 'hint'"); err != nil {
		return err
	}
	if err := s.addColumnIfMissing(ctx, "users", "username_normalized", "TEXT"); err != nil {
		return err
	}
	if err := s.addColumnIfMissing(ctx, "users", "status", "TEXT NOT NULL DEFAULT 'pending'"); err != nil {
		return err
	}
	_, err = s.db.ExecContext(ctx, `
CREATE INDEX IF NOT EXISTS idx_sessions_user ON sessions(user_id);
CREATE UNIQUE INDEX IF NOT EXISTS idx_users_username_normalized ON users(username_normalized) WHERE username_normalized IS NOT NULL AND username_normalized != '';
CREATE INDEX IF NOT EXISTS idx_webauthn_credentials_user ON webauthn_credentials(user_id);
CREATE INDEX IF NOT EXISTS idx_user_documents_user_uploaded ON user_documents(user_id, uploaded_at);
CREATE INDEX IF NOT EXISTS idx_user_documents_document ON user_documents(document_id);
`)
	return err
}

func (s *Store) addColumnIfMissing(ctx context.Context, table, column, definition string) error {
	rows, err := s.db.QueryContext(ctx, `PRAGMA table_info(`+table+`)`)
	if err != nil {
		return err
	}
	defer rows.Close()
	for rows.Next() {
		var cid int
		var name, typ string
		var notNull, pk int
		var defaultValue sql.NullString
		if err := rows.Scan(&cid, &name, &typ, &notNull, &defaultValue, &pk); err != nil {
			return err
		}
		if name == column {
			return nil
		}
	}
	if err := rows.Err(); err != nil {
		return err
	}
	_, err = s.db.ExecContext(ctx, fmt.Sprintf("ALTER TABLE %s ADD COLUMN %s %s", table, column, definition))
	return err
}

func (s *Store) addUniqueIndexIfMissing(ctx context.Context, name, table, column string) error {
	_, err := s.db.ExecContext(ctx, fmt.Sprintf("CREATE UNIQUE INDEX IF NOT EXISTS %s ON %s(%s) WHERE %s IS NOT NULL AND %s != ''", name, table, column, column, column))
	return err
}

func (s *Store) SaveDocument(ctx context.Context, doc *Document) error {
	now := time.Now().UTC()
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()

	if doc.Accent == "" {
		doc.Accent = DefaultAccent
	}
	res, err := tx.ExecContext(ctx, `INSERT INTO documents(uid, slug, title, overview_html, accent, content_hash, created_at) VALUES(?,?,?,?,?,?,?)`, doc.UID, doc.Slug, doc.Title, doc.Overview, doc.Accent, doc.Hash, now.Format(time.RFC3339))
	if err != nil {
		return err
	}
	docID, err := res.LastInsertId()
	if err != nil {
		return err
	}
	doc.ID = docID
	doc.CreatedAt = now
	if _, err := tx.ExecContext(ctx, `INSERT INTO search_entries(document_id, kind, target_key, section_key, title, body) VALUES(?,?,?,?,?,?)`, docID, "overview", "overview", "overview", "Overview", stripHTML(doc.Overview)); err != nil {
		return err
	}
	for i, topic := range doc.Topics {
		if _, err := tx.ExecContext(ctx, `INSERT INTO topics(document_id, topic_key, kind, title, position) VALUES(?,?,?,?,?)`, docID, topic.ID, topic.Kind, topic.Title, i); err != nil {
			return err
		}
		if _, err := tx.ExecContext(ctx, `INSERT INTO search_entries(document_id, kind, target_key, section_key, title, body) VALUES(?,?,?,?,?,?)`, docID, "section", topic.ID, topic.ID, topic.Title, topic.Title); err != nil {
			return err
		}
		for j, task := range topic.Items {
			if _, err := tx.ExecContext(ctx, `INSERT INTO tasks(document_id, topic_key, task_key, title, prompt_html, answer_html, position) VALUES(?,?,?,?,?,?,?)`, docID, topic.ID, task.ID, task.Title, task.Prompt, "", j); err != nil {
				return err
			}
			for k, hint := range task.Hints {
				if _, err := tx.ExecContext(ctx, `INSERT INTO task_hints(document_id, task_key, hint_key, kind, title, body_html, position) VALUES(?,?,?,?,?,?,?)`, docID, task.ID, hint.ID, fallback(hint.Kind, "hint"), hint.Title, hint.Body, k); err != nil {
					return err
				}
			}
			body := stripHTML(task.Prompt + " " + strings.Join(taskHintBodies(task.Hints), " "))
			if _, err := tx.ExecContext(ctx, `INSERT INTO search_entries(document_id, kind, target_key, section_key, title, body) VALUES(?,?,?,?,?,?)`, docID, "task", task.ID, topic.ID, task.Title, body); err != nil {
				return err
			}
		}
	}
	quizTopics := map[string]int{}
	for i, q := range doc.Questions {
		if _, ok := quizTopics[q.TopicID]; !ok {
			quizTopics[q.TopicID] = len(quizTopics)
			if _, err := tx.ExecContext(ctx, `INSERT INTO topics(document_id, topic_key, kind, title, position) VALUES(?,?,?,?,?)`, docID, q.TopicID, "quiz", q.TopicTitle, len(quizTopics)); err != nil {
				return err
			}
			if _, err := tx.ExecContext(ctx, `INSERT INTO search_entries(document_id, kind, target_key, section_key, title, body) VALUES(?,?,?,?,?,?)`, docID, "section", q.TopicID, q.TopicID, q.TopicTitle, q.TopicTitle); err != nil {
				return err
			}
		}
		res, err := tx.ExecContext(ctx, `INSERT INTO questions(document_id, topic_key, topic_title, question_key, kind, prompt_html, explanation_html, position) VALUES(?,?,?,?,?,?,?,?)`, docID, q.TopicID, q.TopicTitle, q.ID, q.Kind, q.Prompt, q.Explanation, i)
		if err != nil {
			return err
		}
		questionID, err := res.LastInsertId()
		if err != nil {
			return err
		}
		for j, opt := range q.Options {
			correct := 0
			if opt.Correct {
				correct = 1
			}
			if _, err := tx.ExecContext(ctx, `INSERT INTO options(question_id, option_key, label, correct, position) VALUES(?,?,?,?,?)`, questionID, opt.ID, opt.Label, correct, j); err != nil {
				return err
			}
		}
		body := stripHTML(q.Prompt + " " + q.Explanation)
		if _, err := tx.ExecContext(ctx, `INSERT INTO search_entries(document_id, kind, target_key, section_key, title, body) VALUES(?,?,?,?,?,?)`, docID, "question", q.ID, q.TopicID, q.TopicTitle, body); err != nil {
			return err
		}
		_ = i
	}
	return tx.Commit()
}

func (s *Store) GetDocument(ctx context.Context, uid, slug string) (*Document, error) {
	doc := &Document{}
	var created string
	err := s.db.QueryRowContext(ctx, `SELECT id, uid, slug, title, overview_html, accent, COALESCE(content_hash, ''), created_at FROM documents WHERE uid=? AND slug=?`, uid, slug).Scan(&doc.ID, &doc.UID, &doc.Slug, &doc.Title, &doc.Overview, &doc.Accent, &doc.Hash, &created)
	if err != nil {
		return nil, err
	}
	doc.CreatedAt, _ = time.Parse(time.RFC3339, created)
	topics, err := s.loadTopics(ctx, doc.ID)
	if err != nil {
		return nil, err
	}
	doc.Topics = topics
	questions, err := s.loadQuestions(ctx, doc.ID)
	if err != nil {
		return nil, err
	}
	doc.Questions = questions
	return doc, nil
}

func (s *Store) GetDocumentByUID(ctx context.Context, uid string) (*Document, error) {
	doc := &Document{}
	var created string
	err := s.db.QueryRowContext(ctx, `SELECT id, uid, slug, title, overview_html, accent, COALESCE(content_hash, ''), created_at FROM documents WHERE uid=?`, uid).Scan(&doc.ID, &doc.UID, &doc.Slug, &doc.Title, &doc.Overview, &doc.Accent, &doc.Hash, &created)
	if err != nil {
		return nil, err
	}
	doc.CreatedAt, _ = time.Parse(time.RFC3339, created)
	topics, err := s.loadTopics(ctx, doc.ID)
	if err != nil {
		return nil, err
	}
	doc.Topics = topics
	questions, err := s.loadQuestions(ctx, doc.ID)
	if err != nil {
		return nil, err
	}
	doc.Questions = questions
	return doc, nil
}

func (s *Store) GetDocumentByHash(ctx context.Context, hash string) (*Document, error) {
	hash = strings.TrimSpace(hash)
	if hash == "" {
		return nil, sql.ErrNoRows
	}
	doc := &Document{}
	var created string
	err := s.db.QueryRowContext(ctx, `SELECT id, uid, slug, title, overview_html, accent, COALESCE(content_hash, ''), created_at FROM documents WHERE content_hash=?`, hash).Scan(&doc.ID, &doc.UID, &doc.Slug, &doc.Title, &doc.Overview, &doc.Accent, &doc.Hash, &created)
	if err != nil {
		return nil, err
	}
	doc.CreatedAt, _ = time.Parse(time.RFC3339, created)
	topics, err := s.loadTopics(ctx, doc.ID)
	if err != nil {
		return nil, err
	}
	doc.Topics = topics
	questions, err := s.loadQuestions(ctx, doc.ID)
	if err != nil {
		return nil, err
	}
	doc.Questions = questions
	return doc, nil
}

func taskHintBodies(hints []Hint) []string {
	bodies := make([]string, 0, len(hints))
	for _, hint := range hints {
		if strings.TrimSpace(hint.Body) != "" {
			bodies = append(bodies, hint.Body)
		}
	}
	return bodies
}

func (s *Store) GetHints(ctx context.Context, docID int64, taskID string) (Task, error) {
	var task Task
	err := s.db.QueryRowContext(ctx, `SELECT task_key, title FROM tasks WHERE document_id=? AND task_key=?`, docID, taskID).Scan(&task.ID, &task.Title)
	if err != nil {
		return task, err
	}
	rows, err := s.db.QueryContext(ctx, `SELECT hint_key, kind, title, body_html FROM task_hints WHERE document_id=? AND task_key=? ORDER BY position`, docID, taskID)
	if err != nil {
		return task, err
	}
	defer rows.Close()
	for rows.Next() {
		var hint Hint
		if err := rows.Scan(&hint.ID, &hint.Kind, &hint.Title, &hint.Body); err != nil {
			return task, err
		}
		task.Hints = append(task.Hints, hint)
	}
	if err := rows.Err(); err != nil {
		return task, err
	}
	task.HintCount = len(task.Hints)
	return task, nil
}

func (s *Store) GetSolution(ctx context.Context, docID int64, taskID string) (Task, error) {
	task, err := s.GetHints(ctx, docID, taskID)
	if err != nil {
		return task, err
	}
	var solutions []Hint
	for _, hint := range task.Hints {
		if hint.Kind == "solution" {
			solutions = append(solutions, hint)
		}
	}
	task.Hints = solutions
	task.HintCount = len(solutions)
	if len(solutions) == 0 {
		return task, sql.ErrNoRows
	}
	return task, nil
}

func (s *Store) GetHint(ctx context.Context, docID int64, taskID, hintID string) (Hint, error) {
	var hint Hint
	err := s.db.QueryRowContext(ctx, `SELECT hint_key, kind, title, body_html FROM task_hints WHERE document_id=? AND task_key=? AND hint_key=? AND kind='hint'`, docID, taskID, hintID).Scan(&hint.ID, &hint.Kind, &hint.Title, &hint.Body)
	return hint, err
}

func (s *Store) GetQuestion(ctx context.Context, docID int64, questionID string) (Question, error) {
	var q Question
	var rowID int64
	err := s.db.QueryRowContext(ctx, `SELECT id, topic_key, topic_title, question_key, kind, prompt_html, explanation_html FROM questions WHERE document_id=? AND question_key=?`, docID, questionID).Scan(&rowID, &q.TopicID, &q.TopicTitle, &q.ID, &q.Kind, &q.Prompt, &q.Explanation)
	if err != nil {
		return q, err
	}
	options, err := s.loadOptions(ctx, rowID)
	q.Options = options
	return q, err
}

func (s *Store) Search(ctx context.Context, docID int64, q string) ([]SearchResult, error) {
	q = strings.TrimSpace(q)
	if q == "" {
		return nil, nil
	}
	rows, err := s.db.QueryContext(ctx, `
SELECT kind, target_key, section_key, title, body
FROM search_entries
WHERE document_id=? AND lower(title || ' ' || body) LIKE '%' || lower(?) || '%'
ORDER BY
	CASE
		WHEN lower(title)=lower(?) THEN 0
		WHEN lower(title) LIKE lower(?) || '%' THEN 1
		WHEN lower(title) LIKE '%' || lower(?) || '%' THEN 2
		WHEN kind='overview' THEN 4
		ELSE 3
	END,
	title
LIMIT 12`, docID, q, q, q, q)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var results []SearchResult
	for rows.Next() {
		var body string
		var r SearchResult
		if err := rows.Scan(&r.Kind, &r.TargetID, &r.SectionID, &r.Title, &body); err != nil {
			return nil, err
		}
		r.Excerpt = excerpt(body, q)
		results = append(results, r)
	}
	return results, rows.Err()
}

func (s *Store) loadTopics(ctx context.Context, docID int64) ([]Topic, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT topic_key, kind, title FROM topics WHERE document_id=? AND kind='lab' ORDER BY position`, docID)
	if err != nil {
		return nil, err
	}
	var topics []Topic
	for rows.Next() {
		var topic Topic
		if err := rows.Scan(&topic.ID, &topic.Kind, &topic.Title); err != nil {
			rows.Close()
			return nil, err
		}
		topics = append(topics, topic)
	}
	if err := rows.Close(); err != nil {
		return nil, err
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	for i := range topics {
		taskRows, err := s.db.QueryContext(ctx, `
SELECT task_key, title, prompt_html,
	(SELECT COUNT(*) FROM task_hints WHERE task_hints.document_id=tasks.document_id AND task_hints.task_key=tasks.task_key),
	(SELECT COUNT(*) FROM task_hints WHERE task_hints.document_id=tasks.document_id AND task_hints.task_key=tasks.task_key AND task_hints.kind='solution')
FROM tasks
WHERE document_id=? AND topic_key=?
ORDER BY position`, docID, topics[i].ID)
		if err != nil {
			return nil, err
		}
		for taskRows.Next() {
			var task Task
			if err := taskRows.Scan(&task.ID, &task.Title, &task.Prompt, &task.HintCount, &task.SolutionCount); err != nil {
				taskRows.Close()
				return nil, err
			}
			topics[i].Items = append(topics[i].Items, task)
		}
		if err := taskRows.Close(); err != nil {
			return nil, err
		}
		if err := taskRows.Err(); err != nil {
			return nil, err
		}
	}
	return topics, nil
}

func (s *Store) loadQuestions(ctx context.Context, docID int64) ([]Question, error) {
	rows, err := s.db.QueryContext(ctx, `
SELECT q.id, q.topic_key, q.topic_title, q.question_key, q.kind, q.prompt_html, q.explanation_html
FROM questions q
JOIN topics t ON t.document_id=q.document_id AND t.topic_key=q.topic_key AND t.kind='quiz'
WHERE q.document_id=?
ORDER BY t.position, random()`, docID)
	if err != nil {
		return nil, err
	}
	type questionRow struct {
		rowID int64
		q     Question
	}
	var scanned []questionRow
	var questions []Question
	for rows.Next() {
		var q Question
		var rowID int64
		if err := rows.Scan(&rowID, &q.TopicID, &q.TopicTitle, &q.ID, &q.Kind, &q.Prompt, &q.Explanation); err != nil {
			rows.Close()
			return nil, err
		}
		scanned = append(scanned, questionRow{rowID: rowID, q: q})
	}
	if err := rows.Close(); err != nil {
		return nil, err
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	for _, row := range scanned {
		options, err := s.loadOptions(ctx, row.rowID)
		if err != nil {
			return nil, err
		}
		row.q.Options = options
		questions = append(questions, row.q)
	}
	return questions, nil
}

func (s *Store) loadOptions(ctx context.Context, questionRowID int64) ([]Option, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT option_key, label, correct FROM options WHERE question_id=? ORDER BY random()`, questionRowID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var options []Option
	for rows.Next() {
		var opt Option
		var correct int
		if err := rows.Scan(&opt.ID, &opt.Label, &correct); err != nil {
			return nil, err
		}
		opt.Correct = correct == 1
		options = append(options, opt)
	}
	return options, rows.Err()
}

func CheckQuestion(q Question, selected []string) (bool, []string) {
	want := map[string]bool{}
	for _, opt := range q.Options {
		if opt.Correct {
			want[opt.ID] = true
		}
	}
	got := map[string]bool{}
	for _, id := range selected {
		got[id] = true
	}
	if len(want) != len(got) {
		return false, hints(want)
	}
	for id := range want {
		if !got[id] {
			return false, hints(want)
		}
	}
	return true, hints(want)
}

func hints(m map[string]bool) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	return out
}

func excerpt(body, q string) string {
	body = strings.Join(strings.Fields(body), " ")
	if len(body) <= 180 {
		return body
	}
	i := strings.Index(strings.ToLower(body), strings.ToLower(q))
	if i < 0 {
		return body[:180] + "..."
	}
	start := i - 60
	if start < 0 {
		start = 0
	}
	end := start + 180
	if end > len(body) {
		end = len(body)
	}
	return strings.TrimSpace(body[start:end]) + "..."
}

func stripHTML(s string) string {
	replacer := strings.NewReplacer("<", " <", ">", "> ")
	s = replacer.Replace(s)
	var out strings.Builder
	inTag := false
	for _, r := range s {
		switch r {
		case '<':
			inTag = true
		case '>':
			inTag = false
		default:
			if !inTag {
				out.WriteRune(r)
			}
		}
	}
	return strings.Join(strings.Fields(out.String()), " ")
}

var ErrNotFound = errors.New("not found")

func normalizeNotFound(err error) error {
	if errors.Is(err, sql.ErrNoRows) {
		return ErrNotFound
	}
	return fmt.Errorf("%w", err)
}
