package history

import (
	"database/sql"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"multiharness-core/internal/store"
)

func TestArchiveRestartsAndLinksExactArtifacts(t *testing.T) {
	root := filepath.Join(t.TempDir(), "private")
	workspace := filepath.Join(t.TempDir(), "workspace")
	a, err := Open(root)
	if err != nil {
		t.Fatal(err)
	}
	defer a.Close()
	conversationID, err := a.Resume(workspace)
	if err != nil {
		t.Fatal(err)
	}
	plan := store.Plan{ID: NewArtifactID("plan"), Version: 1, Action: store.PlanActionPropose, Title: "Invoice export", Tags: []string{"invoices", "export"}, Summary: "Add a scoped invoice export", HandoffContext: []string{"Keep tenant scoping"}, Steps: []string{"Add export endpoint"}, AcceptanceCriteria: []string{"Tenant isolation test passes"}}
	if err := plan.Validate(); err != nil {
		t.Fatal(err)
	}
	planID, err := a.SaveTurn(conversationID, "Plan the invoice export", plan.Display(), store.TaskOutput{Status: store.TaskStatusAnswered, Summary: plan.Display(), Plan: &plan}, "", "head-one")
	if err != nil || planID != plan.ID {
		t.Fatalf("plan save: %s %v", planID, err)
	}
	impl := &store.ImplementationResult{ID: NewArtifactID("impl"), Version: 1, Summary: "Implemented export", ChangedFiles: []string{"export.go"}}
	review := &store.Review{Approved: true, Summary: "Accepted"}
	implementationPlan := plan
	implementationPlan.Action = store.PlanActionImplement
	_, err = a.SaveTurn(conversationID, "Implement this plan", "Implemented export", store.TaskOutput{Status: store.TaskStatusApproved, Summary: "Accepted", Plan: &implementationPlan, Implementation: impl, LastReview: review}, planID, "head-one")
	if err != nil {
		t.Fatal(err)
	}
	if err := a.Close(); err != nil {
		t.Fatal(err)
	}
	a, err = Open(root)
	if err != nil {
		t.Fatal(err)
	}
	defer a.Close()
	resumed, err := a.Resume(workspace)
	if err != nil || resumed != conversationID {
		t.Fatalf("resume: %s %v", resumed, err)
	}
	focus, err := a.Focus(conversationID)
	if err != nil || focus != planID {
		t.Fatalf("focus: %s %v", focus, err)
	}
	loaded, head, err := a.LoadPlan(workspace, planID)
	if err != nil || head != "head-one" || loaded.Steps[0] != plan.Steps[0] {
		t.Fatalf("load: %+v %s %v", loaded, head, err)
	}
	turns, err := a.Recent(conversationID, 6)
	if err != nil || len(turns) != 2 || turns[0].User != "Plan the invoice export" || turns[0].ID == "" || turns[0].Kind != "planning" || turns[1].PlanID != planID || turns[1].ImplementationID != impl.ID || turns[1].ReviewID == "" {
		t.Fatalf("turns: %+v %v", turns, err)
	}
	var kind, indexedPlanID, indexedImplementationID, indexedReviewID string
	if err := a.db.QueryRow(`SELECT kind,plan_id FROM turns WHERE id=?`, turns[0].ID).Scan(&kind, &indexedPlanID); err != nil || kind != "planning" || indexedPlanID != planID {
		t.Fatalf("planning index: %s %s %v", kind, indexedPlanID, err)
	}
	if err := a.db.QueryRow(`SELECT kind,plan_id,implementation_id,review_id FROM turns WHERE id=?`, turns[1].ID).Scan(&kind, &indexedPlanID, &indexedImplementationID, &indexedReviewID); err != nil || kind != "implementation" || indexedPlanID != planID || indexedImplementationID != impl.ID || indexedReviewID == "" {
		t.Fatalf("thin turn links: %s %s %s %s %v", kind, indexedPlanID, indexedImplementationID, indexedReviewID, err)
	}
	searched, err := a.SearchPlans(workspace, "invoice export", 5)
	if err != nil || len(searched) != 1 || searched[0].ID != planID {
		t.Fatalf("search: %+v %v", searched, err)
	}
	var links int
	if err := a.db.QueryRow(`SELECT COUNT(*) FROM artifact_links WHERE target_id=?`, planID).Scan(&links); err != nil || links != 2 {
		t.Fatalf("plan links: %d %v", links, err)
	}
	if err := a.db.QueryRow(`SELECT COUNT(*) FROM artifact_links WHERE target_id=?`, impl.ID).Scan(&links); err != nil || links != 1 {
		t.Fatalf("implementation links: %d %v", links, err)
	}
	loadedImplementation, err := a.LoadImplementation(workspace, impl.ID)
	if err != nil || loadedImplementation.Summary != impl.Summary {
		t.Fatalf("implementation retrieval: %+v %v", loadedImplementation, err)
	}
	var reviewID string
	if err := a.db.QueryRow(`SELECT id FROM artifacts WHERE kind='review' LIMIT 1`).Scan(&reviewID); err != nil {
		t.Fatal(err)
	}
	loadedReview, err := a.LoadReview(workspace, reviewID)
	if err != nil || !loadedReview.Approved {
		t.Fatalf("review retrieval: %+v %v", loadedReview, err)
	}
	refreshed := loaded
	refreshed.ID, refreshed.Version, refreshed.Action = NewArtifactID("plan"), 2, store.PlanActionPropose
	refreshed.Title = "Invoice export revised"
	refreshed.Steps = []string{"Add export endpoint with pagination"}
	newPlanID, err := a.SaveTurn(conversationID, "Refresh the saved plan", refreshed.Display(), store.TaskOutput{Status: store.TaskStatusAnswered, Summary: refreshed.Display(), Plan: &refreshed}, planID, "head-two")
	if err != nil || newPlanID != refreshed.ID {
		t.Fatalf("refresh: %s %v", newPlanID, err)
	}
	newPlan, _, err := a.LoadPlan(workspace, newPlanID)
	if err != nil || newPlan.CaseID != loaded.CaseID || newPlan.Version != 2 {
		t.Fatalf("case version: %+v %v", newPlan, err)
	}
	var supersedes int
	if err := a.db.QueryRow(`SELECT COUNT(*) FROM artifact_links WHERE source_id=? AND target_id=? AND relation='supersedes'`, newPlanID, planID).Scan(&supersedes); err != nil || supersedes != 1 {
		t.Fatalf("supersedes: %d %v", supersedes, err)
	}
	var leaked string
	if err := a.db.QueryRow(`SELECT sql FROM sqlite_master WHERE name='turns'`).Scan(&leaked); err != nil || strings.Contains(leaked, "assistant TEXT") {
		t.Fatalf("unexpected full-content column: %s %v", leaked, err)
	}
	ro, err := OpenReadOnly(root)
	if err != nil {
		t.Fatal(err)
	}
	defer ro.Close()
	if _, _, err := ro.LoadPlan(workspace, planID); err != nil {
		t.Fatal(err)
	}
	if _, _, err := ro.LoadPlan(filepath.Join(t.TempDir(), "other"), planID); err != sql.ErrNoRows {
		t.Fatalf("cross-workspace access: %v", err)
	}
}

func TestMissingOrCorruptBlobFailsClosed(t *testing.T) {
	root := filepath.Join(t.TempDir(), "state")
	a, err := Open(root)
	if err != nil {
		t.Fatal(err)
	}
	defer a.Close()
	workspace := t.TempDir()
	conversationID, err := a.NewConversation(workspace)
	if err != nil {
		t.Fatal(err)
	}
	_, err = a.SaveTurn(conversationID, "Secret question", "Exact answer", store.TaskOutput{Status: store.TaskStatusResponded, Summary: "Exact answer"}, "", "")
	if err != nil {
		t.Fatal(err)
	}
	var hash string
	if err := a.db.QueryRow(`SELECT blob_hash FROM turns WHERE conversation_id=?`, conversationID).Scan(&hash); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(root, "blobs", hash[:2], hash+".json")
	if err := os.WriteFile(path, []byte(`{"user":"forged"}`), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := a.Recent(conversationID, 1); err == nil || !strings.Contains(err.Error(), "hash mismatch") {
		t.Fatalf("corrupt content accepted: %v", err)
	}
	if err := os.Remove(path); err != nil {
		t.Fatal(err)
	}
	if _, err := a.Recent(conversationID, 1); err == nil || !strings.Contains(err.Error(), "unavailable") {
		t.Fatalf("missing content accepted: %v", err)
	}
}

func TestOldOrphanIsPrunedButIndexedContentSurvives(t *testing.T) {
	root := filepath.Join(t.TempDir(), "state")
	a, err := Open(root)
	if err != nil {
		t.Fatal(err)
	}
	defer a.Close()
	conversationID, err := a.NewConversation(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	_, err = a.SaveTurn(conversationID, "keep", "answer", store.TaskOutput{Status: store.TaskStatusResponded, Summary: "answer"}, "", "")
	if err != nil {
		t.Fatal(err)
	}
	orphanHash, err := a.writeBlob(map[string]string{"orphan": "old"})
	if err != nil {
		t.Fatal(err)
	}
	orphanPath := filepath.Join(root, "blobs", orphanHash[:2], orphanHash+".json")
	old := time.Now().Add(-48 * time.Hour)
	if err := os.Chtimes(orphanPath, old, old); err != nil {
		t.Fatal(err)
	}
	if err := a.PruneOrphans(); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(orphanPath); !os.IsNotExist(err) {
		t.Fatalf("orphan remains: %v", err)
	}
	if turns, err := a.Recent(conversationID, 1); err != nil || len(turns) != 1 || turns[0].User != "keep" {
		t.Fatalf("indexed content lost: %+v %v", turns, err)
	}
}

func TestVersionOneIndexMigratesWithoutLosingTurns(t *testing.T) {
	root := t.TempDir()
	db, err := sql.Open("sqlite", filepath.Join(root, "index.sqlite"))
	if err != nil {
		t.Fatal(err)
	}
	for _, statement := range []string{
		`CREATE TABLE conversations(id TEXT PRIMARY KEY,workspace TEXT NOT NULL,created_at TEXT NOT NULL)`,
		`CREATE TABLE turns(id TEXT PRIMARY KEY,conversation_id TEXT NOT NULL,blob_hash TEXT NOT NULL,user_hint TEXT NOT NULL,reply_hint TEXT NOT NULL,created_at TEXT NOT NULL)`,
		`PRAGMA user_version=1`,
	} {
		if _, err := db.Exec(statement); err != nil {
			t.Fatal(err)
		}
	}
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}
	a, err := Open(root)
	if err != nil {
		t.Fatal(err)
	}
	defer a.Close()
	var version int
	if err := a.db.QueryRow(`PRAGMA user_version`).Scan(&version); err != nil || version != 2 {
		t.Fatalf("schema version: %d %v", version, err)
	}
	conversationID, err := a.NewConversation(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if _, err := a.SaveTurn(conversationID, "after migration", "answer", store.TaskOutput{Status: store.TaskStatusResponded, Summary: "answer"}, "", ""); err != nil {
		t.Fatal(err)
	}
	var kind string
	if err := a.db.QueryRow(`SELECT kind FROM turns WHERE conversation_id=?`, conversationID).Scan(&kind); err != nil || kind != "answer" {
		t.Fatalf("migrated turn: %s %v", kind, err)
	}
}
