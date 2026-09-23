// Package history keeps a small searchable SQLite index and immutable private
// content-addressed files. Provider sessions are never the source of truth.
package history

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
	"time"
	"unicode/utf8"

	"multiharness-core/internal/store"

	_ "modernc.org/sqlite"
)

const maxBlobBytes = 16 << 20

var searchWord = regexp.MustCompile(`[\pL\pN_]+`)

type Archive struct {
	db   *sql.DB
	root string
}

type PlanMeta struct {
	ID        string
	CaseID    string
	Version   int
	Title     string
	Tags      []string
	Summary   string
	CreatedAt string
}

type Turn struct {
	ID               string           `json:"id,omitempty"`
	Kind             string           `json:"kind,omitempty"`
	PlanID           string           `json:"plan_id,omitempty"`
	ImplementationID string           `json:"implementation_id,omitempty"`
	ReviewID         string           `json:"review_id,omitempty"`
	User             string           `json:"user"`
	Assistant        string           `json:"assistant"`
	Output           store.TaskOutput `json:"output"`
}

type turnRef struct{ id, hash, kind, planID, implementationID, reviewID string }

func scanTurnRef(scan func(...any) error) (turnRef, error) {
	var ref turnRef
	err := scan(&ref.id, &ref.hash, &ref.kind, &ref.planID, &ref.implementationID, &ref.reviewID)
	return ref, err
}

func (a *Archive) readTurnRef(ref turnRef) (Turn, error) {
	var turn Turn
	if err := a.readBlob(ref.hash, &turn); err != nil {
		return Turn{}, err
	}
	turn.ID, turn.Kind, turn.PlanID, turn.ImplementationID, turn.ReviewID = ref.id, ref.kind, ref.planID, ref.implementationID, ref.reviewID
	return turn, nil
}

// Open uses a private directory on the configured local state volume. The
// database contains short searchable metadata; exact text lives in blobs.
func Open(root string) (*Archive, error) {
	if !filepath.IsAbs(root) {
		return nil, errors.New("history root must be absolute")
	}
	for _, dir := range []string{root, filepath.Join(root, "blobs")} {
		if err := os.MkdirAll(dir, 0700); err != nil {
			return nil, err
		}
		if err := os.Chmod(dir, 0700); err != nil {
			return nil, err
		}
	}
	db, err := sql.Open("sqlite", filepath.Join(root, "index.sqlite"))
	if err != nil {
		return nil, err
	}
	db.SetMaxOpenConns(1) // one writer, short transactions
	var schemaVersion int
	if err := db.QueryRow(`PRAGMA user_version`).Scan(&schemaVersion); err != nil {
		db.Close()
		return nil, err
	}
	if schemaVersion > 2 {
		db.Close()
		return nil, fmt.Errorf("history schema version %d is newer than this build", schemaVersion)
	}
	queries := []string{
		`PRAGMA journal_mode=WAL`, `PRAGMA busy_timeout=5000`, `PRAGMA foreign_keys=ON`,
		`CREATE TABLE IF NOT EXISTS conversations (id TEXT PRIMARY KEY, workspace TEXT NOT NULL, created_at TEXT NOT NULL)`,
		`CREATE INDEX IF NOT EXISTS conversation_workspace ON conversations(workspace, created_at DESC)`,
		`CREATE TABLE IF NOT EXISTS conversation_focus (conversation_id TEXT PRIMARY KEY REFERENCES conversations(id) ON DELETE CASCADE, plan_id TEXT NOT NULL REFERENCES artifacts(id))`,
		`CREATE TABLE IF NOT EXISTS turns (id TEXT PRIMARY KEY, conversation_id TEXT NOT NULL REFERENCES conversations(id) ON DELETE CASCADE, blob_hash TEXT NOT NULL, user_hint TEXT NOT NULL, reply_hint TEXT NOT NULL, kind TEXT NOT NULL DEFAULT 'answer', plan_id TEXT NOT NULL DEFAULT '', implementation_id TEXT NOT NULL DEFAULT '', review_id TEXT NOT NULL DEFAULT '', created_at TEXT NOT NULL)`,
		`CREATE INDEX IF NOT EXISTS turn_conversation ON turns(conversation_id, created_at DESC)`,
		`CREATE VIRTUAL TABLE IF NOT EXISTS turn_search USING fts5(turn_id UNINDEXED, user_hint, reply_hint)`,
		`CREATE TABLE IF NOT EXISTS artifacts (id TEXT PRIMARY KEY, conversation_id TEXT NOT NULL REFERENCES conversations(id), kind TEXT NOT NULL, version INTEGER NOT NULL, title TEXT NOT NULL, tags TEXT NOT NULL, summary TEXT NOT NULL, blob_hash TEXT NOT NULL, workspace_head TEXT NOT NULL, created_at TEXT NOT NULL)`,
		`CREATE TABLE IF NOT EXISTS cases (id TEXT PRIMARY KEY, workspace TEXT NOT NULL, title TEXT NOT NULL, created_at TEXT NOT NULL)`,
		`CREATE TABLE IF NOT EXISTS artifact_cases (artifact_id TEXT PRIMARY KEY REFERENCES artifacts(id), case_id TEXT NOT NULL REFERENCES cases(id), version INTEGER NOT NULL, UNIQUE(case_id,version))`,
		`CREATE INDEX IF NOT EXISTS artifact_conversation ON artifacts(conversation_id, created_at DESC)`,
		`CREATE TABLE IF NOT EXISTS artifact_links (source_id TEXT NOT NULL REFERENCES artifacts(id), target_id TEXT NOT NULL REFERENCES artifacts(id), relation TEXT NOT NULL, PRIMARY KEY(source_id,target_id,relation))`,
		`CREATE VIRTUAL TABLE IF NOT EXISTS plan_search USING fts5(plan_id UNINDEXED, title, tags, summary)`,
	}
	for _, query := range queries {
		if _, err := db.Exec(query); err != nil {
			db.Close()
			return nil, fmt.Errorf("initialize history: %w", err)
		}
	}
	for _, column := range []struct{ name, definition string }{{"kind", "TEXT NOT NULL DEFAULT 'answer'"}, {"plan_id", "TEXT NOT NULL DEFAULT ''"}, {"implementation_id", "TEXT NOT NULL DEFAULT ''"}, {"review_id", "TEXT NOT NULL DEFAULT ''"}} {
		if err := ensureColumn(db, "turns", column.name, column.definition); err != nil {
			db.Close()
			return nil, err
		}
	}
	for _, query := range []string{`CREATE INDEX IF NOT EXISTS turn_plan ON turns(plan_id)`, `CREATE INDEX IF NOT EXISTS turn_implementation ON turns(implementation_id)`} {
		if _, err := db.Exec(query); err != nil {
			db.Close()
			return nil, err
		}
	}
	if _, err := db.Exec(`PRAGMA user_version=2`); err != nil {
		db.Close()
		return nil, err
	}
	for _, suffix := range []string{"index.sqlite", "index.sqlite-wal", "index.sqlite-shm"} {
		path := filepath.Join(root, suffix)
		if _, err := os.Stat(path); err == nil {
			_ = os.Chmod(path, 0600)
		}
	}
	archive := &Archive{db: db, root: root}
	archive.maybePruneOrphans()
	return archive, nil
}

func ensureColumn(db *sql.DB, table, column, definition string) error {
	rows, err := db.Query(`PRAGMA table_info(` + table + `)`)
	if err != nil {
		return err
	}
	found := false
	for rows.Next() {
		var index, notNull, primary int
		var name, dataType string
		var defaultValue sql.NullString
		if err := rows.Scan(&index, &name, &dataType, &notNull, &defaultValue, &primary); err != nil {
			rows.Close()
			return err
		}
		if name == column {
			found = true
		}
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return err
	}
	rows.Close()
	if found {
		return nil
	}
	_, err = db.Exec(`ALTER TABLE ` + table + ` ADD COLUMN ` + column + ` ` + definition)
	return err
}

// OpenReadOnly does not initialize or modify history. Agents may use this during
// a read-only stage to fetch an exact record by ID.
func OpenReadOnly(root string) (*Archive, error) {
	if !filepath.IsAbs(root) {
		return nil, errors.New("history root must be absolute")
	}
	path := filepath.ToSlash(filepath.Join(root, "index.sqlite"))
	db, err := sql.Open("sqlite", "file:"+path+"?mode=ro")
	if err != nil {
		return nil, err
	}
	db.SetMaxOpenConns(1)
	if _, err := db.Exec(`PRAGMA query_only=ON`); err != nil {
		db.Close()
		return nil, err
	}
	return &Archive{db: db, root: root}, nil
}

func (a *Archive) Close() error { return a.db.Close() }

// PruneOrphans removes only old content files that no committed index entry
// references. The age guard leaves in-flight writers and recent crashes alone.
func (a *Archive) PruneOrphans() error {
	referenced := make(map[string]bool)
	for _, query := range []string{`SELECT blob_hash FROM turns`, `SELECT blob_hash FROM artifacts`} {
		rows, err := a.db.Query(query)
		if err != nil {
			return err
		}
		for rows.Next() {
			var hash string
			if err := rows.Scan(&hash); err != nil {
				rows.Close()
				return err
			}
			referenced[hash] = true
		}
		if err := rows.Err(); err != nil {
			rows.Close()
			return err
		}
		rows.Close()
	}
	cutoff := time.Now().Add(-24 * time.Hour)
	return filepath.WalkDir(filepath.Join(a.root, "blobs"), func(path string, entry os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.IsDir() || !entry.Type().IsRegular() {
			return nil
		}
		name := entry.Name()
		if !strings.HasSuffix(name, ".json") {
			return nil
		}
		hash := strings.TrimSuffix(name, ".json")
		if len(hash) != 64 || strings.Trim(hash, "0123456789abcdef") != "" || referenced[hash] {
			return nil
		}
		info, err := entry.Info()
		if err != nil {
			return err
		}
		if info.ModTime().After(cutoff) {
			return nil
		}
		return os.Remove(path)
	})
}

func (a *Archive) maybePruneOrphans() {
	marker := filepath.Join(a.root, ".last-gc")
	if info, err := os.Stat(marker); err == nil && time.Since(info.ModTime()) < 24*time.Hour {
		return
	}
	if a.PruneOrphans() == nil {
		_ = os.WriteFile(marker, []byte(time.Now().UTC().Format(time.RFC3339)), 0600)
	}
}

func (a *Archive) Resume(workspace string) (string, error) {
	var id string
	err := a.db.QueryRow(`SELECT id FROM conversations WHERE workspace=? ORDER BY created_at DESC LIMIT 1`, workspace).Scan(&id)
	if err == nil {
		return id, nil
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return "", err
	}
	return a.NewConversation(workspace)
}

func (a *Archive) NewConversation(workspace string) (string, error) {
	id := newID("conv")
	_, err := a.db.Exec(`INSERT INTO conversations(id,workspace,created_at) VALUES(?,?,?)`, id, workspace, time.Now().UTC().Format(time.RFC3339Nano))
	return id, err
}

func (a *Archive) Focus(conversationID string) (string, error) {
	var id string
	err := a.db.QueryRow(`SELECT plan_id FROM conversation_focus WHERE conversation_id=?`, conversationID).Scan(&id)
	if errors.Is(err, sql.ErrNoRows) {
		return "", nil
	}
	return id, err
}

func (a *Archive) SetFocus(conversationID, planID string) error {
	_, err := a.db.Exec(`INSERT INTO conversation_focus(conversation_id,plan_id) VALUES(?,?) ON CONFLICT(conversation_id) DO UPDATE SET plan_id=excluded.plan_id`, conversationID, planID)
	return err
}

func newID(prefix string) string {
	var b [12]byte
	_, _ = rand.Read(b[:])
	return prefix + "_" + hex.EncodeToString(b[:])
}

func NewArtifactID(prefix string) string { return newID(prefix) }

func (a *Archive) writeBlob(value any) (string, error) {
	data, err := json.Marshal(value)
	if err != nil {
		return "", err
	}
	if len(data) > maxBlobBytes {
		return "", errors.New("history record exceeds the 16 MiB limit")
	}
	sum := sha256.Sum256(data)
	hash := hex.EncodeToString(sum[:])
	dir := filepath.Join(a.root, "blobs", hash[:2])
	if err := os.MkdirAll(dir, 0700); err != nil {
		return "", err
	}
	path := filepath.Join(dir, hash+".json")
	if _, err := os.Stat(path); err == nil {
		var verified json.RawMessage
		return hash, a.readBlob(hash, &verified)
	} else if !errors.Is(err, os.ErrNotExist) {
		return "", err
	}
	f, err := os.CreateTemp(dir, ".blob-*")
	if err != nil {
		return "", err
	}
	defer os.Remove(f.Name())
	if err := f.Chmod(0600); err != nil {
		f.Close()
		return "", err
	}
	if _, err := f.Write(data); err != nil {
		f.Close()
		return "", err
	}
	if err := f.Sync(); err != nil {
		f.Close()
		return "", err
	}
	if err := f.Close(); err != nil {
		return "", err
	}
	if err := os.Rename(f.Name(), path); err != nil {
		if _, statErr := os.Stat(path); statErr != nil {
			return "", err
		}
		var verified json.RawMessage
		if verifyErr := a.readBlob(hash, &verified); verifyErr != nil {
			return "", verifyErr
		}
	}
	if runtime.GOOS != "windows" {
		directory, err := os.Open(dir)
		if err != nil {
			return "", err
		}
		defer directory.Close()
		if err := directory.Sync(); err != nil {
			return "", err
		}
	}
	return hash, nil
}

func (a *Archive) readBlob(hash string, value any) error {
	if len(hash) != 64 || strings.Trim(hash, "0123456789abcdef") != "" {
		return errors.New("invalid history hash")
	}
	data, err := os.ReadFile(filepath.Join(a.root, "blobs", hash[:2], hash+".json"))
	if err != nil {
		return fmt.Errorf("history content unavailable: %w", err)
	}
	if len(data) > maxBlobBytes {
		return errors.New("history content exceeds the read limit")
	}
	sum := sha256.Sum256(data)
	if hex.EncodeToString(sum[:]) != hash {
		return errors.New("history content hash mismatch")
	}
	return json.Unmarshal(data, value)
}

func hint(text string) string {
	text = strings.TrimSpace(strings.Join(strings.Fields(text), " "))
	if len(text) > 180 {
		cut := 180
		for cut > 0 && !utf8.RuneStart(text[cut]) {
			cut--
		}
		text = text[:cut]
	}
	return text
}

// SaveTurn writes immutable content before committing its index. A crash may
// leave an unreferenced blob, but never an indexed record without its blob.
func (a *Archive) SaveTurn(conversationID, user, assistant string, output store.TaskOutput, selectedPlanID, workspaceHead string) (string, error) {
	turnHash, err := a.writeBlob(Turn{User: user, Assistant: assistant, Output: output})
	if err != nil {
		return "", err
	}
	var planID, planHash, planCaseID, implementationID, implementationHash, reviewID, reviewHash string
	planVersion := 1
	if output.Plan != nil && (output.Plan.Action == store.PlanActionPropose || (selectedPlanID == "" && output.Plan.Action == store.PlanActionImplement)) {
		planID = output.Plan.ID
		if planID == "" {
			planID = newID("plan")
		}
		plan := *output.Plan
		plan.ID = planID
		if plan.Version < 1 {
			plan.Version = 1
		}
		if plan.CaseID == "" {
			plan.CaseID = newID("case")
		}
		planCaseID, planVersion = plan.CaseID, plan.Version
		if plan.Title == "" {
			plan.Title = plan.Summary
		}
		planHash, err = a.writeBlob(plan)
		if err != nil {
			return "", err
		}
	}
	if output.Implementation != nil {
		implementationID = output.Implementation.ID
		if implementationID == "" {
			implementationID = newID("impl")
		}
		implementationHash, err = a.writeBlob(output.Implementation)
		if err != nil {
			return "", err
		}
	}
	if output.LastReview != nil {
		reviewID = newID("review")
		reviewHash, err = a.writeBlob(output.LastReview)
		if err != nil {
			return "", err
		}
	}
	tx, err := a.db.BeginTx(context.Background(), nil)
	if err != nil {
		return "", err
	}
	defer tx.Rollback()
	now := time.Now().UTC().Format(time.RFC3339Nano)
	turnID := newID("turn")
	kind := "answer"
	if output.Direct != nil {
		kind = "direct"
	}
	if output.Plan != nil && output.Plan.Action == store.PlanActionPropose {
		kind = "planning"
	}
	if output.Implementation != nil {
		kind = "implementation"
	}
	linkedPlanID := selectedPlanID
	if planID != "" {
		linkedPlanID = planID
	}
	if _, err = tx.Exec(`INSERT INTO turns(id,conversation_id,blob_hash,user_hint,reply_hint,kind,plan_id,implementation_id,review_id,created_at) VALUES(?,?,?,?,?,?,?,?,?,?)`, turnID, conversationID, turnHash, hint(user), hint(assistant), kind, linkedPlanID, implementationID, reviewID, now); err != nil {
		return "", err
	}
	if _, err = tx.Exec(`INSERT INTO turn_search(turn_id,user_hint,reply_hint) VALUES(?,?,?)`, turnID, hint(user), hint(assistant)); err != nil {
		return "", err
	}
	insert := func(id, kind, title, tags, summary, hash string, version int) error {
		_, e := tx.Exec(`INSERT INTO artifacts(id,conversation_id,kind,version,title,tags,summary,blob_hash,workspace_head,created_at) VALUES(?,?,?,?,?,?,?,?,?,?)`, id, conversationID, kind, version, hint(title), tags, hint(summary), hash, workspaceHead, now)
		return e
	}
	if planID != "" {
		tags, _ := json.Marshal(output.Plan.Tags)
		title := output.Plan.Title
		if title == "" {
			title = output.Plan.Summary
		}
		if _, err = tx.Exec(`INSERT INTO cases(id,workspace,title,created_at) SELECT ?,workspace,?,? FROM conversations WHERE id=? ON CONFLICT(id) DO NOTHING`, planCaseID, hint(title), now, conversationID); err != nil {
			return "", err
		}
		if err = insert(planID, "plan", title, string(tags), output.Plan.Summary, planHash, planVersion); err != nil {
			return "", err
		}
		if _, err = tx.Exec(`INSERT INTO artifact_cases(artifact_id,case_id,version) VALUES(?,?,?)`, planID, planCaseID, planVersion); err != nil {
			return "", err
		}
		if selectedPlanID != "" && output.Plan.Action == store.PlanActionPropose {
			if _, err = tx.Exec(`INSERT INTO artifact_links(source_id,target_id,relation) VALUES(?,?,?)`, planID, selectedPlanID, "supersedes"); err != nil {
				return "", err
			}
		}
		if _, err = tx.Exec(`INSERT INTO plan_search(plan_id,title,tags,summary) VALUES(?,?,?,?)`, planID, hint(title), strings.Join(output.Plan.Tags, " "), hint(output.Plan.Summary)); err != nil {
			return "", err
		}
		if _, err = tx.Exec(`INSERT INTO conversation_focus(conversation_id,plan_id) VALUES(?,?) ON CONFLICT(conversation_id) DO UPDATE SET plan_id=excluded.plan_id`, conversationID, planID); err != nil {
			return "", err
		}
	}
	if implementationID != "" {
		if err = insert(implementationID, "implementation", "Implementation", "[]", output.Implementation.Summary, implementationHash, 1); err != nil {
			return "", err
		}
		linkedPlanID := selectedPlanID
		if linkedPlanID == "" {
			linkedPlanID = planID
		}
		if linkedPlanID != "" {
			if _, err = tx.Exec(`INSERT INTO artifact_links(source_id,target_id,relation) VALUES(?,?,?)`, implementationID, linkedPlanID, "implements"); err != nil {
				return "", err
			}
		}
	}
	if reviewID != "" {
		if err = insert(reviewID, "review", "Review", "[]", output.LastReview.Summary, reviewHash, 1); err != nil {
			return "", err
		}
		linkedPlanID := selectedPlanID
		if linkedPlanID == "" {
			linkedPlanID = planID
		}
		for _, target := range []struct{ id, relation string }{{linkedPlanID, "reviews_plan"}, {implementationID, "reviews_implementation"}} {
			if target.id != "" {
				if _, err = tx.Exec(`INSERT INTO artifact_links(source_id,target_id,relation) VALUES(?,?,?)`, reviewID, target.id, target.relation); err != nil {
					return "", err
				}
			}
		}
	}
	if err := tx.Commit(); err != nil {
		return "", err
	}
	return planID, nil
}

func (a *Archive) LoadPlan(workspace, id string) (store.Plan, string, error) {
	var hash, head string
	err := a.db.QueryRow(`SELECT a.blob_hash,a.workspace_head FROM artifacts a JOIN conversations c ON c.id=a.conversation_id WHERE a.id=? AND a.kind='plan' AND c.workspace=?`, id, workspace).Scan(&hash, &head)
	if err != nil {
		return store.Plan{}, "", err
	}
	var plan store.Plan
	if err := a.readBlob(hash, &plan); err != nil {
		return store.Plan{}, "", err
	}
	if err := plan.Validate(); err != nil {
		return store.Plan{}, "", err
	}
	return plan, head, nil
}

func (a *Archive) loadArtifact(workspace, id, kind string, value any) error {
	var hash string
	err := a.db.QueryRow(`SELECT a.blob_hash FROM artifacts a JOIN conversations c ON c.id=a.conversation_id WHERE a.id=? AND a.kind=? AND c.workspace=?`, id, kind, workspace).Scan(&hash)
	if err != nil {
		return err
	}
	return a.readBlob(hash, value)
}

func (a *Archive) LoadImplementation(workspace, id string) (store.ImplementationResult, error) {
	var result store.ImplementationResult
	if err := a.loadArtifact(workspace, id, "implementation", &result); err != nil {
		return store.ImplementationResult{}, err
	}
	if err := result.Validate(); err != nil {
		return store.ImplementationResult{}, err
	}
	return result, nil
}

func (a *Archive) LoadReview(workspace, id string) (store.Review, error) {
	var result store.Review
	if err := a.loadArtifact(workspace, id, "review", &result); err != nil {
		return store.Review{}, err
	}
	if err := result.Validate(); err != nil {
		return store.Review{}, err
	}
	return result, nil
}

func (a *Archive) ListPlans(workspace string, limit int) ([]PlanMeta, error) {
	rows, err := a.db.Query(`SELECT a.id,COALESCE(ac.case_id,''),a.version,a.title,a.tags,a.summary,a.created_at FROM artifacts a JOIN conversations c ON c.id=a.conversation_id LEFT JOIN artifact_cases ac ON ac.artifact_id=a.id WHERE a.kind='plan' AND c.workspace=? ORDER BY a.created_at DESC LIMIT ?`, workspace, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var result []PlanMeta
	for rows.Next() {
		var p PlanMeta
		var tags string
		if err := rows.Scan(&p.ID, &p.CaseID, &p.Version, &p.Title, &tags, &p.Summary, &p.CreatedAt); err != nil {
			return nil, err
		}
		if err := json.Unmarshal([]byte(tags), &p.Tags); err != nil {
			return nil, err
		}
		result = append(result, p)
	}
	return result, rows.Err()
}

// SearchPlans searches only short index metadata; exact content is loaded by ID.
func (a *Archive) SearchPlans(workspace, query string, limit int) ([]PlanMeta, error) {
	words := searchWord.FindAllString(query, -1)
	if len(words) == 0 {
		return nil, nil
	}
	if len(words) > 12 {
		words = words[:12]
	}
	for i, word := range words {
		words[i] = `"` + strings.ReplaceAll(word, `"`, "") + `"`
	}
	match := strings.Join(words, " OR ")
	rows, err := a.db.Query(`SELECT a.id,COALESCE(ac.case_id,''),a.version,a.title,a.tags,a.summary,a.created_at FROM plan_search s JOIN artifacts a ON a.id=s.plan_id JOIN conversations c ON c.id=a.conversation_id LEFT JOIN artifact_cases ac ON ac.artifact_id=a.id WHERE plan_search MATCH ? AND c.workspace=? ORDER BY bm25(plan_search),a.created_at DESC LIMIT ?`, match, workspace, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var result []PlanMeta
	for rows.Next() {
		var p PlanMeta
		var tags string
		if err := rows.Scan(&p.ID, &p.CaseID, &p.Version, &p.Title, &tags, &p.Summary, &p.CreatedAt); err != nil {
			return nil, err
		}
		if err := json.Unmarshal([]byte(tags), &p.Tags); err != nil {
			return nil, err
		}
		result = append(result, p)
	}
	return result, rows.Err()
}

func (a *Archive) Recent(conversationID string, limit int) ([]Turn, error) {
	rows, err := a.db.Query(`SELECT id,blob_hash,kind,plan_id,implementation_id,review_id FROM turns WHERE conversation_id=? ORDER BY created_at DESC LIMIT ?`, conversationID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var refs []turnRef
	for rows.Next() {
		ref, err := scanTurnRef(rows.Scan)
		if err != nil {
			return nil, err
		}
		refs = append(refs, ref)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	turns := make([]Turn, 0, len(refs))
	for i := len(refs) - 1; i >= 0; i-- {
		turn, err := a.readTurnRef(refs[i])
		if err != nil {
			return nil, err
		}
		turns = append(turns, turn)
	}
	return turns, nil
}

func (a *Archive) SearchTurns(conversationID, query string, limit int) ([]Turn, error) {
	return a.searchTurns(query, limit, `SELECT t.id,t.blob_hash,t.kind,t.plan_id,t.implementation_id,t.review_id FROM turn_search s JOIN turns t ON t.id=s.turn_id WHERE turn_search MATCH ? AND t.conversation_id=? ORDER BY bm25(turn_search) LIMIT ?`, conversationID)
}

func (a *Archive) SearchWorkspaceTurns(workspace, query string, limit int) ([]Turn, error) {
	return a.searchTurns(query, limit, `SELECT t.id,t.blob_hash,t.kind,t.plan_id,t.implementation_id,t.review_id FROM turn_search s JOIN turns t ON t.id=s.turn_id JOIN conversations c ON c.id=t.conversation_id WHERE turn_search MATCH ? AND c.workspace=? ORDER BY bm25(turn_search) LIMIT ?`, workspace)
}

func (a *Archive) searchTurns(query string, limit int, statement, scope string) ([]Turn, error) {
	words := searchWord.FindAllString(query, -1)
	if len(words) == 0 {
		return nil, nil
	}
	if len(words) > 12 {
		words = words[:12]
	}
	for i, word := range words {
		words[i] = `"` + word + `"`
	}
	rows, err := a.db.Query(statement, strings.Join(words, " OR "), scope, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var refs []turnRef
	for rows.Next() {
		ref, err := scanTurnRef(rows.Scan)
		if err != nil {
			return nil, err
		}
		refs = append(refs, ref)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	turns := make([]Turn, 0, len(refs))
	for _, ref := range refs {
		turn, err := a.readTurnRef(ref)
		if err != nil {
			return nil, err
		}
		turns = append(turns, turn)
	}
	return turns, nil
}

func (a *Archive) LoadTurn(workspace, id string) (Turn, error) {
	ref, err := scanTurnRef(a.db.QueryRow(`SELECT t.id,t.blob_hash,t.kind,t.plan_id,t.implementation_id,t.review_id FROM turns t JOIN conversations c ON c.id=t.conversation_id WHERE t.id=? AND c.workspace=?`, id, workspace).Scan)
	if err != nil {
		return Turn{}, err
	}
	return a.readTurnRef(ref)
}
