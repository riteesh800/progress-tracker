package tests

import (
	"context"
	"encoding/json"
	"net/http"
	"testing"
	"time"
)

func TestSkillsTrashRestorePurgeAndCrossUserIsolation(t *testing.T) {
	env := NewTestEnv(t)

	tokenA, _ := env.RegisterUser("usera@example.com", "Password123!", "UserA")
	tokenB, _ := env.RegisterUser("userb@example.com", "Password123!", "UserB")

	// User A creates a skill
	code, skillA, _ := env.DoJSON(http.MethodPost, "/skills", map[string]any{
		"name":        "Go Backend Mastery",
		"description": "Concurrency and net/http",
	}, AuthHeader(tokenA))
	if code != http.StatusCreated {
		t.Fatalf("create skill A failed: %d %v", code, skillA)
	}
	skillAID, _ := skillA["id"].(string)

	// User A cannot create a duplicate skill with the same name -> 409 skill_exists
	code, dupErr, _ := env.DoJSON(http.MethodPost, "/skills", map[string]any{
		"name": "Go Backend Mastery",
	}, AuthHeader(tokenA))
	if code != http.StatusConflict || dupErr["code"] != "skill_exists" {
		t.Fatalf("expected 409 skill_exists for same user, got %d %v", code, dupErr)
	}

	// User B CAN create a skill with the same name (per-user unique constraint)
	code, skillB, _ := env.DoJSON(http.MethodPost, "/skills", map[string]any{
		"name": "Go Backend Mastery",
	}, AuthHeader(tokenB))
	if code != http.StatusCreated || skillB["id"] == "" {
		t.Fatalf("expected User B to create same-named skill, got %d %v", code, skillB)
	}

	// User B cannot read, patch, or delete User A's skill -> 404 not_found
	code, _, _ = env.DoJSON(http.MethodGet, "/skills/"+skillAID, nil, AuthHeader(tokenB))
	if code != http.StatusNotFound {
		t.Fatalf("User B GET User A skill expected 404, got %d", code)
	}
	code, _, _ = env.DoJSON(http.MethodPatch, "/skills/"+skillAID, map[string]any{"name": "Hacked"}, AuthHeader(tokenB))
	if code != http.StatusNotFound {
		t.Fatalf("User B PATCH User A skill expected 404, got %d", code)
	}
	code, _, _ = env.DoJSON(http.MethodDelete, "/skills/"+skillAID, nil, AuthHeader(tokenB))
	if code != http.StatusNotFound {
		t.Fatalf("User B DELETE User A skill expected 404, got %d", code)
	}

	// Add a topic to User A's skill so delete-impact requires confirmation
	code, topic1, _ := env.DoJSON(http.MethodPost, "/topics", map[string]any{
		"skill_id": skillAID,
		"name":     "Goroutines",
	}, AuthHeader(tokenA))
	if code != http.StatusCreated {
		t.Fatalf("create topic failed: %d %v", code, topic1)
	}

	// Check delete-impact
	code, impact, _ := env.DoJSON(http.MethodGet, "/skills/"+skillAID+"/delete-impact", nil, AuthHeader(tokenA))
	if code != http.StatusOK || impact["requires_confirm"] != true || impact["descendants"] != float64(1) {
		t.Fatalf("unexpected delete-impact: %d %v", code, impact)
	}

	// Deleting without confirm=true fails with 409 confirm_required
	code, delErr, _ := env.DoJSON(http.MethodDelete, "/skills/"+skillAID, nil, AuthHeader(tokenA))
	if code != http.StatusConflict || delErr["code"] != "confirm_required" {
		t.Fatalf("expected 409 confirm_required, got %d %v", code, delErr)
	}

	// Deleting with confirm=true moves skill to 3-day Trash
	code, _, _ = env.DoJSON(http.MethodDelete, "/skills/"+skillAID+"?confirm=true", nil, AuthHeader(tokenA))
	if code != http.StatusOK {
		t.Fatalf("delete skill with confirm=true failed: %d", code)
	}

	// Active skills list for User A is now empty; trash list has 1 skill
	_, _, rawActive := env.DoJSON(http.MethodGet, "/skills", nil, AuthHeader(tokenA))
	var activeList []map[string]any
	_ = json.Unmarshal(rawActive, &activeList)
	if len(activeList) != 0 {
		t.Fatalf("expected 0 active skills after trash, got %d", len(activeList))
	}

	_, _, rawTrash := env.DoJSON(http.MethodGet, "/skills/trash", nil, AuthHeader(tokenA))
	var trashList []map[string]any
	_ = json.Unmarshal(rawTrash, &trashList)
	if len(trashList) != 1 || trashList[0]["id"] != skillAID {
		t.Fatalf("expected 1 trashed skill, got %v", trashList)
	}

	// Restore from trash
	code, restored, _ := env.DoJSON(http.MethodPost, "/skills/"+skillAID+"/restore", nil, AuthHeader(tokenA))
	if code != http.StatusOK || restored["deleted_at"] != nil {
		t.Fatalf("restore skill failed: %d %v", code, restored)
	}

	// Move to trash again, expire purge_at -> restore returns 410 trash_expired
	code, _, _ = env.DoJSON(http.MethodDelete, "/skills/"+skillAID+"?confirm=true", nil, AuthHeader(tokenA))
	if code != http.StatusOK {
		t.Fatalf("re-trash failed: %d", code)
	}
	past := time.Now().UTC().Add(-1 * time.Hour)
	_, _ = env.DB.Exec(context.Background(), nil, `UPDATE skills SET purge_at = ? WHERE id = ?`, env.DB.FormatTime(past), skillAID)

	code, expErr, _ := env.DoJSON(http.MethodPost, "/skills/"+skillAID+"/restore", nil, AuthHeader(tokenA))
	if code != http.StatusGone || expErr["code"] != "trash_expired" {
		t.Fatalf("expected 410 trash_expired, got %d %v", code, expErr)
	}

	// Permanent delete removes from trash
	code, _, _ = env.DoJSON(http.MethodDelete, "/skills/"+skillAID+"/permanent", nil, AuthHeader(tokenA))
	if code != http.StatusOK {
		t.Fatalf("permanent delete failed: %d", code)
	}
}

func TestTopicsHierarchyProgressAndSkillTimer(t *testing.T) {
	env := NewTestEnv(t)
	token, _ := env.RegisterUser("topics@example.com", "Password123!", "TopicUser")

	_, skill, _ := env.DoJSON(http.MethodPost, "/skills", map[string]any{
		"name": "Distributed Systems",
	}, AuthHeader(token))
	skillID, _ := skill["id"].(string)

	// Create root topic R1
	code, r1, _ := env.DoJSON(http.MethodPost, "/topics", map[string]any{
		"skill_id": skillID,
		"name":     "Consensus",
	}, AuthHeader(token))
	if code != http.StatusCreated {
		t.Fatalf("create R1 failed: %d", code)
	}
	r1ID, _ := r1["id"].(string)

	// Complete R1 while it is a leaf
	code, r1Done, _ := env.DoJSON(http.MethodPost, "/topics/"+r1ID+"/complete", map[string]any{
		"completed": true,
	}, AuthHeader(token))
	if code != http.StatusOK || r1Done["is_completed"] != true || r1Done["percent"] != float64(100) {
		t.Fatalf("complete leaf R1 failed: %d %v", code, r1Done)
	}

	// Add child C1 under R1 -> R1 becomes a parent, so its own is_completed is auto-cleared and progress becomes 0%
	code, c1, _ := env.DoJSON(http.MethodPost, "/topics", map[string]any{
		"skill_id":  skillID,
		"parent_id": r1ID,
		"name":      "Raft",
	}, AuthHeader(token))
	if code != http.StatusCreated {
		t.Fatalf("create C1 failed: %d", code)
	}
	c1ID, _ := c1["id"].(string)

	// Add child C2 under R1
	_, c2, _ := env.DoJSON(http.MethodPost, "/topics", map[string]any{
		"skill_id":  skillID,
		"parent_id": r1ID,
		"name":      "Paxos",
	}, AuthHeader(token))
	c2ID, _ := c2["id"].(string)

	// Trying to mark parent R1 complete must fail with 422 not_a_leaf
	code, leafErr, _ := env.DoJSON(http.MethodPost, "/topics/"+r1ID+"/complete", map[string]any{
		"completed": true,
	}, AuthHeader(token))
	if code != http.StatusUnprocessableEntity || leafErr["code"] != "not_a_leaf" {
		t.Fatalf("expected 422 not_a_leaf on parent topic, got %d %v", code, leafErr)
	}

	// Complete C1 -> R1 and Skill progress must be 50% (1 of 2 leaves)
	code, _, _ = env.DoJSON(http.MethodPost, "/topics/"+c1ID+"/complete", map[string]any{
		"completed": true,
	}, AuthHeader(token))
	if code != http.StatusOK {
		t.Fatalf("complete C1 failed: %d", code)
	}
	_, r1Mid, _ := env.DoJSON(http.MethodGet, "/topics/"+r1ID, nil, AuthHeader(token))
	if r1Mid["percent"] != float64(50) || r1Mid["completed_leaves"] != float64(1) || r1Mid["total_leaves"] != float64(2) {
		t.Fatalf("expected R1 50%% (1/2), got %v", r1Mid)
	}

	// Complete C2 -> Skill reaches 100% and logs skill_completed activity!
	code, _, _ = env.DoJSON(http.MethodPost, "/topics/"+c2ID+"/complete", map[string]any{
		"completed": true,
	}, AuthHeader(token))
	if code != http.StatusOK {
		t.Fatalf("complete C2 failed: %d", code)
	}
	_, skillAfter, _ := env.DoJSON(http.MethodGet, "/skills/"+skillID, nil, AuthHeader(token))
	if skillAfter["percent"] != float64(100) {
		t.Fatalf("expected skill 100%%, got %v", skillAfter)
	}

	// Verify disabled topic modification endpoints return 404 or 405
	for _, tc := range []struct {
		method string
		path   string
	}{
		{http.MethodPatch, "/topics/" + c1ID},
		{http.MethodDelete, "/topics/" + c1ID},
		{http.MethodPost, "/topics/" + c1ID + "/move"},
		{http.MethodGet, "/topics/" + c1ID + "/delete-impact"},
	} {
		st, _, _ := env.DoJSON(tc.method, tc.path, map[string]any{"name": "x"}, AuthHeader(token))
		if st != http.StatusNotFound && st != http.StatusMethodNotAllowed {
			t.Fatalf("%s %s expected 404 or 405, got %d", tc.method, tc.path, st)
		}
	}

	// Skill Timer: initially null, then test exact frontend POST /skills/{id}/timer and PUT alias
	code, _, rawTimer := env.DoJSON(http.MethodGet, "/skills/"+skillID+"/timer", nil, AuthHeader(token))
	if code != http.StatusOK || string(rawTimer) != "null\n" && string(rawTimer) != "null" {
		t.Fatalf("expected null timer initially, got %d %q", code, string(rawTimer))
	}

	// Invalid timer range (from_date == to_date) via POST -> 400 invalid_dates
	code, errTimerEq, _ := env.DoJSON(http.MethodPost, "/skills/"+skillID+"/timer", map[string]any{
		"from_date": "2026-06-10",
		"to_date":   "2026-06-10",
	}, AuthHeader(token))
	if code != http.StatusBadRequest || errTimerEq["code"] != "invalid_dates" || errTimerEq["message"] != "to_date must be strictly after from_date" {
		t.Fatalf("expected 400 invalid_dates when from_date == to_date, got %d %v", code, errTimerEq)
	}

	// Invalid timer range (from_date > to_date) via POST -> 400 invalid_dates
	code, errTimerGt, _ := env.DoJSON(http.MethodPost, "/skills/"+skillID+"/timer", map[string]any{
		"from_date": "2026-06-15",
		"to_date":   "2026-06-10",
	}, AuthHeader(token))
	if code != http.StatusBadRequest || errTimerGt["code"] != "invalid_dates" {
		t.Fatalf("expected 400 invalid_dates when from_date > to_date, got %d %v", code, errTimerGt)
	}

	// Malformed date format -> 422 validation_error
	code, errTimerFmt, _ := env.DoJSON(http.MethodPost, "/skills/"+skillID+"/timer", map[string]any{
		"from_date": "not-a-date",
		"to_date":   "2026-06-30",
	}, AuthHeader(token))
	if code != http.StatusUnprocessableEntity || errTimerFmt["code"] != "validation_error" {
		t.Fatalf("expected 422 validation_error on malformed date, got %d %v", code, errTimerFmt)
	}

	// Valid timer upsert using exact frontend POST /skills/{id}/timer method and payload
	code, timerOut, _ := env.DoJSON(http.MethodPost, "/skills/"+skillID+"/timer", map[string]any{
		"from_date": "2026-06-01",
		"to_date":   "2026-06-30",
	}, AuthHeader(token))
	if code != http.StatusOK || timerOut["from_date"] != "2026-06-01" || timerOut["to_date"] != "2026-06-30" {
		t.Fatalf("POST upsert timer failed: %d %v", code, timerOut)
	}

	// Update existing timer using POST /skills/{id}/timer
	code, timerOut2, _ := env.DoJSON(http.MethodPost, "/skills/"+skillID+"/timer", map[string]any{
		"from_date": "2026-07-01",
		"to_date":   "2026-07-31",
	}, AuthHeader(token))
	if code != http.StatusOK || timerOut2["from_date"] != "2026-07-01" || timerOut2["to_date"] != "2026-07-31" {
		t.Fatalf("POST update timer failed: %d %v", code, timerOut2)
	}

	// Backward-compatible PUT /skills/{id}/timer alias also works
	code, timerOutPut, _ := env.DoJSON(http.MethodPut, "/skills/"+skillID+"/timer", map[string]any{
		"from_date": "2026-08-01",
		"to_date":   "2026-08-31",
	}, AuthHeader(token))
	if code != http.StatusOK || timerOutPut["from_date"] != "2026-08-01" || timerOutPut["to_date"] != "2026-08-31" {
		t.Fatalf("PUT upsert timer failed: %d %v", code, timerOutPut)
	}

	// GET timer returns updated dates
	code, timerGet, _ := env.DoJSON(http.MethodGet, "/skills/"+skillID+"/timer", nil, AuthHeader(token))
	if code != http.StatusOK || timerGet["from_date"] != "2026-08-01" || timerGet["to_date"] != "2026-08-31" {
		t.Fatalf("GET timer failed: %d %v", code, timerGet)
	}

	// Delete timer -> 204 No Content
	code, _, _ = env.DoJSON(http.MethodDelete, "/skills/"+skillID+"/timer", nil, AuthHeader(token))
	if code != http.StatusNoContent {
		t.Fatalf("delete timer expected 204, got %d", code)
	}
}

func TestAutomaticThreeDayPurgeAndCompleteCascade(t *testing.T) {
	env := NewTestEnv(t)
	token, _ := env.RegisterUser("purge_test@example.com", "Password123!", "PurgeUser")

	// 1. Create a skill with a topic, note, and skill timer
	_, skill, _ := env.DoJSON(http.MethodPost, "/skills", map[string]any{"name": "Skill To Auto Purge"}, AuthHeader(token))
	skillID := skill["id"].(string)

	_, topic, _ := env.DoJSON(http.MethodPost, "/topics", map[string]any{"skill_id": skillID, "name": "Topic Under Skill"}, AuthHeader(token))
	topicID := topic["id"].(string)

	_, _, _ = env.DoJSON(http.MethodPost, "/topics/"+topicID+"/notes", map[string]any{"content": "Note content to be purged"}, AuthHeader(token))

	_, _, _ = env.DoJSON(http.MethodPost, "/skills/"+skillID+"/timer", map[string]any{"from_date": "2026-06-01", "to_date": "2026-06-30"}, AuthHeader(token))

	// Soft-delete skill (move to trash)
	code, _, _ := env.DoJSON(http.MethodDelete, "/skills/"+skillID+"?confirm=true", nil, AuthHeader(token))
	if code != http.StatusOK {
		t.Fatalf("delete skill failed: %d", code)
	}

	// Fast forward purge_at by 3 days (in the past)
	past := time.Now().UTC().Add(-73 * time.Hour)
	_, _ = env.DB.Exec(context.Background(), nil, `UPDATE skills SET purge_at = ? WHERE id = ?`, env.DB.FormatTime(past), skillID)

	// User requests trash (or skills list) -> "the very next moment" automatic purge runs
	code, _, rawTrash := env.DoJSON(http.MethodGet, "/skills/trash", nil, AuthHeader(token))
	if code != http.StatusOK {
		t.Fatalf("get trash failed: %d", code)
	}
	var trashList []map[string]any
	_ = json.Unmarshal(rawTrash, &trashList)
	if len(trashList) != 0 {
		t.Fatalf("expected trash to be empty after 3-day purge, got %d items", len(trashList))
	}

	// Verify complete cascading deletion in database
	var count int
	_ = env.DB.QueryRow(context.Background(), nil, `SELECT COUNT(*) FROM skills WHERE id = ?`, skillID).Scan(&count)
	if count != 0 {
		t.Fatalf("expected skill to be permanently deleted from DB, found %d", count)
	}
	_ = env.DB.QueryRow(context.Background(), nil, `SELECT COUNT(*) FROM topics WHERE id = ?`, topicID).Scan(&count)
	if count != 0 {
		t.Fatalf("expected topic to be permanently deleted from DB, found %d", count)
	}
	_ = env.DB.QueryRow(context.Background(), nil, `SELECT COUNT(*) FROM notes WHERE topic_id = ?`, topicID).Scan(&count)
	if count != 0 {
		t.Fatalf("expected note to be permanently deleted from DB, found %d", count)
	}
	_ = env.DB.QueryRow(context.Background(), nil, `SELECT COUNT(*) FROM skill_timers WHERE skill_id = ?`, skillID).Scan(&count)
	if count != 0 {
		t.Fatalf("expected timer to be permanently deleted from DB, found %d", count)
	}

	// 2. Test Goal 3-Day Auto Purge
	_, goal, _ := env.DoJSON(http.MethodPost, "/goals", map[string]any{
		"goal_name":   "Goal To Auto Purge",
		"deadline_at": time.Now().UTC().Add(48 * time.Hour).Format(time.RFC3339),
	}, AuthHeader(token))
	goalID := goal["id"].(string)

	_, _, _ = env.DoJSON(http.MethodPost, "/goals/"+goalID+"/activities", map[string]any{"activity_text": "Working hard"}, AuthHeader(token))

	// Soft-delete goal
	code, _, _ = env.DoJSON(http.MethodDelete, "/goals/"+goalID, nil, AuthHeader(token))
	if code != http.StatusOK {
		t.Fatalf("delete goal failed: %d", code)
	}

	// Fast forward purge_at by 3 days
	_, _ = env.DB.Exec(context.Background(), nil, `UPDATE goals SET purge_at = ? WHERE id = ?`, env.DB.FormatTime(past), goalID)

	// User requests trash -> automatic purge runs immediately
	code, _, rawGoalTrash := env.DoJSON(http.MethodGet, "/goals/trash", nil, AuthHeader(token))
	if code != http.StatusOK {
		t.Fatalf("get goal trash failed: %d", code)
	}
	var goalTrashList []map[string]any
	_ = json.Unmarshal(rawGoalTrash, &goalTrashList)
	if len(goalTrashList) != 0 {
		t.Fatalf("expected goal trash to be empty after 3-day purge, got %d items", len(goalTrashList))
	}

	// Verify database state
	_ = env.DB.QueryRow(context.Background(), nil, `SELECT COUNT(*) FROM goals WHERE id = ?`, goalID).Scan(&count)
	if count != 0 {
		t.Fatalf("expected goal to be permanently deleted from DB, found %d", count)
	}
	_ = env.DB.QueryRow(context.Background(), nil, `SELECT COUNT(*) FROM goal_activities WHERE goal_id = ?`, goalID).Scan(&count)
	if count != 0 {
		t.Fatalf("expected goal activities to be permanently deleted from DB, found %d", count)
	}
}
