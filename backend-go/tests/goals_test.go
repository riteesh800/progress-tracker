package tests

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"skill-progress-tracker/backend-go/internal/models"
	"skill-progress-tracker/backend-go/internal/services"
)

func TestGoalsFullLifecycleAndIsolation(t *testing.T) {
	env := NewTestEnv(t)

	tokenA, _ := env.RegisterUser("alice_goals@example.com", "AlicePass123!", "Alice")
	tokenB, _ := env.RegisterUser("bob_goals@example.com", "BobPass123!", "Bob")

	// 1. Goal creation validation: empty name rejected
	code, errOut, _ := env.DoJSON(http.MethodPost, "/goals", map[string]any{
		"goal_name":   "",
		"deadline_at": time.Now().Add(48 * time.Hour).Format(time.RFC3339),
	}, AuthHeader(tokenA))
	if code != http.StatusUnprocessableEntity {
		t.Fatalf("expected 422 for empty goal_name, got %d %v", code, errOut)
	}

	// 2. Goal creation validation: name > 100 runes rejected
	longName := strings.Repeat("A", 101)
	code, _, _ = env.DoJSON(http.MethodPost, "/goals", map[string]any{
		"goal_name":   longName,
		"deadline_at": time.Now().Add(48 * time.Hour).Format(time.RFC3339),
	}, AuthHeader(tokenA))
	if code != http.StatusUnprocessableEntity {
		t.Fatalf("expected 422 for name > 100 chars, got %d", code)
	}

	// 3. Goal creation: valid goal with optional motivation line
	futureDeadline := time.Now().Add(30 * 24 * time.Hour).UTC().Format(time.RFC3339)
	code, goalA1, rawA1 := env.DoJSON(http.MethodPost, "/goals", map[string]any{
		"goal_name":       "Master Distributed Systems & Go 🚀",
		"deadline_at":     futureDeadline,
		"timezone":        "UTC",
		"motivation_line": "Consistency beats intensity every single day.",
	}, AuthHeader(tokenA))
	if code != http.StatusCreated {
		t.Fatalf("expected 201 created, got %d: %s", code, string(rawA1))
	}
	goalA1ID, _ := goalA1["id"].(string)
	if goalA1ID == "" {
		t.Fatalf("expected goal id, got %v", goalA1)
	}

	// 4. Bob creates a goal with the exact same name (allowed! no conflict)
	code, goalB1, _ := env.DoJSON(http.MethodPost, "/goals", map[string]any{
		"goal_name":   "Master Distributed Systems & Go 🚀",
		"deadline_at": futureDeadline,
	}, AuthHeader(tokenB))
	if code != http.StatusCreated {
		t.Fatalf("Bob should be able to create goal with same name, got %d", code)
	}
	goalB1ID, _ := goalB1["id"].(string)

	// 5. User isolation: Bob cannot view Alice's goal
	code, _, _ = env.DoJSON(http.MethodGet, "/goals/"+goalA1ID, nil, AuthHeader(tokenB))
	if code != http.StatusNotFound {
		t.Fatalf("expected 404 for cross-user goal lookup, got %d", code)
	}

	// 6. Alice lists goals (sees 1 goal)
	code, _, rawListA := env.DoJSON(http.MethodGet, "/goals", nil, AuthHeader(tokenA))
	if code != http.StatusOK {
		t.Fatalf("list goals failed: %d %s", code, string(rawListA))
	}
	var goalsListA []map[string]any
	_ = json.Unmarshal(rawListA, &goalsListA)
	if len(goalsListA) != 1 || goalsListA[0]["id"] != goalA1ID {
		t.Fatalf("expected 1 goal for Alice, got %v", goalsListA)
	}

	// 7. Activity validation: empty text rejected
	code, _, _ = env.DoJSON(http.MethodPost, "/goals/"+goalA1ID+"/activities", map[string]any{
		"activity_text": "   ",
	}, AuthHeader(tokenA))
	if code != http.StatusUnprocessableEntity {
		t.Fatalf("expected 422 for empty activity_text, got %d", code)
	}

	// 8. Activity validation: 51 chars rejected
	text51 := strings.Repeat("x", 51)
	code, _, _ = env.DoJSON(http.MethodPost, "/goals/"+goalA1ID+"/activities", map[string]any{
		"activity_text": text51,
	}, AuthHeader(tokenA))
	if code != http.StatusUnprocessableEntity {
		t.Fatalf("expected 422 for 51 chars activity text, got %d", code)
	}

	// 9. Activity validation: exactly 50 chars accepted, with emojis & symbols
	text50 := "Read Raft consensus paper section 5.2 📚✨ #golang" // exactly 50 runes or less
	code, act1, rawAct1 := env.DoJSON(http.MethodPost, "/goals/"+goalA1ID+"/activities", map[string]any{
		"activity_text": text50,
	}, AuthHeader(tokenA))
	if code != http.StatusCreated {
		t.Fatalf("expected 201 for valid activity, got %d: %s", code, string(rawAct1))
	}
	act1ID, _ := act1["id"].(string)
	if act1ID == "" {
		t.Fatalf("expected activity id, got %v", act1)
	}

	// 10. Multiple cards on the same day accepted as separate records
	code, act2, _ := env.DoJSON(http.MethodPost, "/goals/"+goalA1ID+"/activities", map[string]any{
		"activity_text": "Implemented leader election timer in Go ⚡",
	}, AuthHeader(tokenA))
	if code != http.StatusCreated {
		t.Fatalf("expected 201 for second activity on same day, got %d", code)
	}
	act2ID, _ := act2["id"].(string)
	if act2ID == act1ID {
		t.Fatalf("expected distinct activity IDs")
	}

	// 11. User isolation: Bob cannot add activities to Alice's goal
	code, _, _ = env.DoJSON(http.MethodPost, "/goals/"+goalA1ID+"/activities", map[string]any{
		"activity_text": "Bob trying to hack Alice's goal",
	}, AuthHeader(tokenB))
	if code != http.StatusNotFound {
		t.Fatalf("expected 404 for Bob adding activity to Alice's goal, got %d", code)
	}

	// 12. List activities pagination
	code, _, rawActs := env.DoJSON(http.MethodGet, "/goals/"+goalA1ID+"/activities?page=1&per_page=10", nil, AuthHeader(tokenA))
	if code != http.StatusOK {
		t.Fatalf("list activities failed: %d %s", code, string(rawActs))
	}
	var actPage map[string]any
	_ = json.Unmarshal(rawActs, &actPage)
	totalActs, _ := actPage["total"].(float64)
	if totalActs != 2 {
		t.Fatalf("expected 2 total activities, got %v", totalActs)
	}

	// 13. Progress calculation: multiple cards today gives +1.0 (not +2.0)
	code, progData, rawProg := env.DoJSON(http.MethodGet, "/goals/"+goalA1ID+"/progress", nil, AuthHeader(tokenA))
	if code != http.StatusOK {
		t.Fatalf("get goal progress failed: %d %s", code, string(rawProg))
	}
	score, _ := progData["current_score"].(float64)
	if score != 1.0 {
		t.Fatalf("expected score 1.0 for today with multiple activities, got %v", score)
	}
	workedDays, _ := progData["worked_days"].(float64)
	if workedDays != 1 {
		t.Fatalf("expected 1 worked day, got %v", workedDays)
	}

	// 14. Trash integration: Delete goal moves it to trash
	code, _, _ = env.DoJSON(http.MethodDelete, "/goals/"+goalA1ID, nil, AuthHeader(tokenA))
	if code != http.StatusOK {
		t.Fatalf("delete goal failed: %d", code)
	}

	// Alice's active list is now empty
	code, _, rawListAfterDel := env.DoJSON(http.MethodGet, "/goals", nil, AuthHeader(tokenA))
	var listAfterDel []map[string]any
	_ = json.Unmarshal(rawListAfterDel, &listAfterDel)
	if len(listAfterDel) != 0 {
		t.Fatalf("expected 0 active goals after delete, got %d", len(listAfterDel))
	}

	// Alice's trash list contains the deleted goal
	code, _, rawTrash := env.DoJSON(http.MethodGet, "/goals/trash", nil, AuthHeader(tokenA))
	if code != http.StatusOK {
		t.Fatalf("list trash failed: %d", code)
	}
	var trashList []map[string]any
	_ = json.Unmarshal(rawTrash, &trashList)
	if len(trashList) != 1 || trashList[0]["id"] != goalA1ID {
		t.Fatalf("expected goal in trash, got %v", trashList)
	}

	// Bob's trash does not see Alice's goal
	code, _, rawBobTrash := env.DoJSON(http.MethodGet, "/goals/trash", nil, AuthHeader(tokenB))
	var bobTrashList []map[string]any
	_ = json.Unmarshal(rawBobTrash, &bobTrashList)
	if len(bobTrashList) != 0 {
		t.Fatalf("Bob should not see Alice's trashed goal, got %v", bobTrashList)
	}

	// Restore goal from trash
	code, restoredGoal, rawRestore := env.DoJSON(http.MethodPost, "/goals/"+goalA1ID+"/restore", nil, AuthHeader(tokenA))
	if code != http.StatusOK {
		t.Fatalf("restore goal failed: %d %s", code, string(rawRestore))
	}
	if restoredGoal["deleted_at"] != nil {
		t.Fatalf("expected deleted_at to be cleared on restore, got %v", restoredGoal["deleted_at"])
	}

	// History is preserved: activities still exist
	code, _, rawActsAfterRestore := env.DoJSON(http.MethodGet, "/goals/"+goalA1ID+"/activities", nil, AuthHeader(tokenA))
	_ = json.Unmarshal(rawActsAfterRestore, &actPage)
	if total, _ := actPage["total"].(float64); total != 2 {
		t.Fatalf("expected 2 activities preserved after restore, got %v", total)
	}

	// Permanently delete: move to trash then purge
	_, _, _ = env.DoJSON(http.MethodDelete, "/goals/"+goalA1ID, nil, AuthHeader(tokenA))
	code, _, _ = env.DoJSON(http.MethodDelete, "/goals/"+goalA1ID+"/permanent", nil, AuthHeader(tokenA))
	if code != http.StatusOK {
		t.Fatalf("permanent delete failed: %d", code)
	}

	// Bob's goal is unaffected
	code, bobGoal, _ := env.DoJSON(http.MethodGet, "/goals/"+goalB1ID, nil, AuthHeader(tokenB))
	if code != http.StatusOK || bobGoal["goal"] == nil {
		t.Fatalf("Bob's goal should still exist, got %d %v", code, bobGoal)
	}
}

func TestGoalConsistencyAndInactivityWarningRules(t *testing.T) {
	loc := time.UTC
	now := time.Date(2026, 11, 20, 12, 0, 0, 0, loc)

	// Goal created 5 days ago: 2026-11-15
	created := time.Date(2026, 11, 15, 10, 0, 0, 0, loc)
	deadline := time.Date(2026, 12, 15, 10, 0, 0, 0, loc)

	goal := &models.Goal{
		ID:         uuid.New(),
		GoalName:   "Algorithms",
		DeadlineAt: deadline,
		Timezone:   "UTC",
		CreatedAt:  created,
	}

	// Scenario 1: User had activity on 2026-11-15 and 2026-11-16, then missed 17, 18, 19 (3 completed days missed!).
	// Today (2026-11-20) has no activity yet.
	activityDates := map[string]int{
		"2026-11-15": 1,
		"2026-11-16": 2, // 2 cards, counts once
		// 17: 0
		// 18: 0
		// 19: 0
		// 20 (today): 0 (unfinished, no penalty)
	}

	res := services.ComputeGoalProgress(goal, activityDates, now)

	// Worked days: 2 (15th, 16th)
	if res.WorkedDays != 2 {
		t.Fatalf("expected 2 worked days, got %d", res.WorkedDays)
	}
	// Missed days: 3 (17th, 18th, 19th) - 20th is unfinished today, not counted as missed
	if res.MissedDays != 3 {
		t.Fatalf("expected 3 missed days, got %d", res.MissedDays)
	}
	// Cumulative score:
	// Day 15: +1 (score = 1)
	// Day 16: +1 (score = 2)
	// Day 17: -1 (score = 1)
	// Day 18: -1 (score = 0)
	// Day 19: -1 (score = -1)
	// Day 20 (today, no activity): 0 (score = -1)
	if res.CurrentScore != -1.0 {
		t.Fatalf("expected score -1.0, got %v", res.CurrentScore)
	}
	// Inactivity warning MUST be true (3 consecutive completed days missed: 17, 18, 19)
	if !res.InactivityWarning {
		t.Fatalf("expected InactivityWarning to be true for 3 missed completed days")
	}

	// Scenario 2: User records activity TODAY on the 20th!
	// This immediately clears the warning and adds +1.0 to today
	activityDates["2026-11-20"] = 1
	resToday := services.ComputeGoalProgress(goal, activityDates, now)
	if resToday.InactivityWarning {
		t.Fatalf("InactivityWarning must be cleared immediately when activity recorded today")
	}
	if resToday.WorkedDays != 3 {
		t.Fatalf("expected 3 worked days with today's activity, got %d", resToday.WorkedDays)
	}
	if resToday.CurrentScore != 0.0 { // -1 + 1 = 0
		t.Fatalf("expected score 0.0, got %v", resToday.CurrentScore)
	}

	// Scenario 3: Goal created only 2 days ago (2026-11-18)
	// Missed 18 and 19 (only 2 completed missed days).
	// Inactivity warning MUST NOT show yet!
	goalYoung := &models.Goal{
		ID:         uuid.New(),
		GoalName:   "Young Goal",
		DeadlineAt: deadline,
		Timezone:   "UTC",
		CreatedAt:  time.Date(2026, 11, 18, 10, 0, 0, 0, loc),
	}
	activityDatesYoung := map[string]int{}
	resYoung := services.ComputeGoalProgress(goalYoung, activityDatesYoung, now)
	if resYoung.InactivityWarning {
		t.Fatalf("InactivityWarning must not show before 3 consecutive missed days have elapsed since creation")
	}
}

func TestDeadlineExpirationEnforcement(t *testing.T) {
	env := NewTestEnv(t)
	token, _ := env.RegisterUser("charlie_deadline@example.com", "CharliePass123!", "Charlie")

	// Create goal with deadline in the past: 2 hours ago
	pastDeadline := time.Now().Add(-2 * time.Hour).UTC().Format(time.RFC3339)
	code, goalOut, _ := env.DoJSON(http.MethodPost, "/goals", map[string]any{
		"goal_name":   "Past Goal",
		"deadline_at": pastDeadline,
	}, AuthHeader(token))
	if code != http.StatusCreated {
		t.Fatalf("expected 201, got %d", code)
	}
	goalID := goalOut["id"].(string)

	// Attempting to add an activity to an expired goal must be rejected (400)
	code, errOut, _ := env.DoJSON(http.MethodPost, "/goals/"+goalID+"/activities", map[string]any{
		"activity_text": "Late activity submission",
	}, AuthHeader(token))
	if code != http.StatusBadRequest || errOut["code"] != "deadline_passed" {
		t.Fatalf("expected 400 deadline_passed, got %d %v", code, errOut)
	}

	// Detail endpoint marks deadline as reached
	code, detail, _ := env.DoJSON(http.MethodGet, "/goals/"+goalID, nil, AuthHeader(token))
	if code != http.StatusOK {
		t.Fatalf("expected 200, got %d", code)
	}
	isReached, _ := detail["is_deadline_reached"].(bool)
	if !isReached {
		t.Fatalf("expected is_deadline_reached = true, got %v", detail)
	}
}
