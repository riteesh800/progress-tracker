package tests

import (
	"encoding/json"
	"net/http"
	"net/url"
	"testing"

	"skill-progress-tracker/backend-go/internal/services"
)

func TestTopicNotesPersonalNotesSearchAndDashboard(t *testing.T) {
	env := NewTestEnv(t)

	tokenA, _ := env.RegisterUser("notes_a@example.com", "Password123!", "NotesA")
	tokenB, _ := env.RegisterUser("notes_b@example.com", "Password123!", "NotesB")

	_, skill, _ := env.DoJSON(http.MethodPost, "/skills", map[string]any{
		"name":        "PostgreSQL Internals",
		"description": "MVCC, vacuuming, and WAL indexing",
	}, AuthHeader(tokenA))
	skillID, _ := skill["id"].(string)

	_, topic, _ := env.DoJSON(http.MethodPost, "/topics", map[string]any{
		"skill_id":    skillID,
		"name":        "B-Tree & GIN Indexes",
		"description": "Full-text search with to_tsvector",
	}, AuthHeader(tokenA))
	topicID, _ := topic["id"].(string)

	// Whitespace-only note -> 422 empty_note
	code, emptyErr, _ := env.DoJSON(http.MethodPost, "/notes", map[string]any{
		"topic_id": topicID,
		"content":  "    ",
	}, AuthHeader(tokenA))
	if code != http.StatusUnprocessableEntity || emptyErr["code"] != "empty_note" {
		t.Fatalf("expected 422 empty_note, got %d %v", code, emptyErr)
	}

	// Create valid topic note
	code, note, _ := env.DoJSON(http.MethodPost, "/notes", map[string]any{
		"topic_id": topicID,
		"content":  "Remember to use GIN indexes for tsvector columns.",
	}, AuthHeader(tokenA))
	if code != http.StatusCreated {
		t.Fatalf("create note failed: %d %v", code, note)
	}
	noteID, _ := note["id"].(string)

	// User B cannot access User A's note -> 404
	code, _, _ = env.DoJSON(http.MethodGet, "/notes/"+noteID, nil, AuthHeader(tokenB))
	if code != http.StatusNotFound {
		t.Fatalf("User B GET User A note expected 404, got %d", code)
	}

	// Personal Notes & Tasks checklist
	code, pNote, _ := env.DoJSON(http.MethodPost, "/personal-notes", map[string]any{
		"title": "Weekend Study Plan",
	}, AuthHeader(tokenA))
	if code != http.StatusCreated {
		t.Fatalf("create personal note failed: %d %v", code, pNote)
	}
	pNoteID, _ := pNote["id"].(string)
	pTasks, _ := pNote["tasks"].([]any)
	if len(pTasks) != 1 {
		t.Fatalf("expected 1 default empty task on new personal note, got %d", len(pTasks))
	}

	// Update personal note tasks
	code, pUpdated, _ := env.DoJSON(http.MethodPatch, "/personal-notes/"+pNoteID, map[string]any{
		"title": "Updated Study Plan",
		"tasks": []map[string]any{
			{"content": "Read Chapter 1", "is_completed": true},
			{"content": "Practice SQL queries", "is_completed": false},
		},
	}, AuthHeader(tokenA))
	if code != http.StatusOK || pUpdated["title"] != "Updated Study Plan" {
		t.Fatalf("update personal note failed: %d %v", code, pUpdated)
	}
	upTasks, _ := pUpdated["tasks"].([]any)
	if len(upTasks) != 2 {
		t.Fatalf("expected 2 tasks after update, got %d", len(upTasks))
	}

	// Search across skills, topics, and notes
	code, _, rawHits := env.DoJSON(http.MethodGet, "/search?q=tsvector", nil, AuthHeader(tokenA))
	if code != http.StatusOK {
		t.Fatalf("search failed: %d", code)
	}
	var hits []map[string]any
	_ = json.Unmarshal(rawHits, &hits)
	if len(hits) < 2 {
		t.Fatalf("expected at least 2 hits (topic and note) for 'tsvector', got %d: %v", len(hits), hits)
	}

	// SQL injection / wildcard resistance in search
	injQ := url.QueryEscape(`%' OR 1=1 --`)
	code, _, rawInj := env.DoJSON(http.MethodGet, "/search?q="+injQ, nil, AuthHeader(tokenA))
	if code != http.StatusOK {
		t.Fatalf("injection search expected 200 empty list, got %d", code)
	}
	var injHits []map[string]any
	_ = json.Unmarshal(rawInj, &injHits)
	if len(injHits) != 0 {
		t.Fatalf("SQL injection payload must not match rows, got %d hits", len(injHits))
	}

	// Dashboard & Suggested Next
	code, dash, _ := env.DoJSON(http.MethodGet, "/dashboard", nil, AuthHeader(tokenA))
	if code != http.StatusOK || dash["total_skills"] != float64(1) || dash["total_topics"] != float64(1) {
		t.Fatalf("unexpected dashboard: %d %v", code, dash)
	}
	code, next, _ := env.DoJSON(http.MethodGet, "/dashboard/suggested-next", nil, AuthHeader(tokenA))
	if code != http.StatusOK || next["topic_id"] != topicID {
		t.Fatalf("unexpected suggested-next: %d %v", code, next)
	}
}

func TestStreakOneDayGracePeriodLogic(t *testing.T) {
	// 1. Consecutive days: 2026-04-10, 2026-04-11, 2026-04-12 (today = 2026-04-12) -> current=3, longest=3
	cur, longest := services.ComputeStreak([]string{"2026-04-10", "2026-04-11", "2026-04-12"}, "2026-04-12")
	if cur != 3 || longest != 3 {
		t.Fatalf("consecutive expected (3,3), got (%d,%d)", cur, longest)
	}

	// 2. 1-day gap grace period: 2026-04-09, [miss 04-10], 2026-04-11, 2026-04-12 -> 3 active days in run!
	cur, longest = services.ComputeStreak([]string{"2026-04-09", "2026-04-11", "2026-04-12"}, "2026-04-12")
	if cur != 3 || longest != 3 {
		t.Fatalf("1-day gap grace expected (3,3), got (%d,%d)", cur, longest)
	}

	// 3. 2-day gap breaks streak: 2026-04-01, 2026-04-02, [miss 03 & 04], 2026-04-05 (today = 2026-04-05) -> current=1, longest=2
	cur, longest = services.ComputeStreak([]string{"2026-04-01", "2026-04-02", "2026-04-05"}, "2026-04-05")
	if cur != 1 || longest != 2 {
		t.Fatalf("2-day gap expected (1,2), got (%d,%d)", cur, longest)
	}
}
