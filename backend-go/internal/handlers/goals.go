package handlers

import (
	"context"
	"database/sql"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"
	"skill-progress-tracker/backend-go/internal/core"
	"skill-progress-tracker/backend-go/internal/database"
	"skill-progress-tracker/backend-go/internal/models"
	"skill-progress-tracker/backend-go/internal/services"
)

const goalTrashRetention = 72 * time.Hour

func (s *Server) ListGoals(w http.ResponseWriter, r *http.Request, user *models.User) {
	ctx := r.Context()
	rows, err := s.DB.Query(
		ctx, nil,
		`SELECT g.id, g.user_id, g.goal_name, g.deadline_at, g.timezone, g.motivation_line, g.deleted_at, g.purge_at, g.created_at, g.updated_at,
		 (SELECT COUNT(a.id) FROM goal_activities a WHERE a.goal_id = g.id) AS total_activities
		 FROM goals g
		 WHERE g.user_id = ? AND g.deleted_at IS NULL
		 ORDER BY g.created_at DESC`,
		s.DB.FormatUUID(user.ID),
	)
	if err != nil {
		core.WriteError(w, http.StatusInternalServerError, "internal_error", "Something went wrong. Please try again.")
		return
	}
	defer rows.Close()

	out := make([]models.GoalOut, 0)
	now := time.Now().UTC()
	for rows.Next() {
		var rawID, rawUID, name, tz string
		var deadline, motiv, delAt, purgeAt, created, updated any
		var totalAct int
		if err := rows.Scan(&rawID, &rawUID, &name, &deadline, &tz, &motiv, &delAt, &purgeAt, &created, &updated, &totalAct); err != nil {
			core.WriteError(w, http.StatusInternalServerError, "internal_error", "Something went wrong. Please try again.")
			return
		}
		id, err := database.ParseUUID(rawID)
		if err != nil {
			continue
		}
		deadlineAt, _ := database.ParseTime(deadline)
		deletedAt, _ := database.ParseNullTime(delAt)
		purgeAtTime, _ := database.ParseNullTime(purgeAt)
		createdAt, _ := database.ParseTime(created)
		updatedAt, _ := database.ParseTime(updated)

		isExpired := !now.Before(deadlineAt)
		out = append(out, models.GoalOut{
			ID:              id,
			GoalName:        name,
			DeadlineAt:      deadlineAt,
			Timezone:        tz,
			MotivationLine:  database.ParseNullString(motiv),
			DeletedAt:       deletedAt,
			PurgeAt:         purgeAtTime,
			CreatedAt:       createdAt,
			UpdatedAt:       updatedAt,
			IsExpired:       isExpired,
			TotalActivities: totalAct,
		})
	}

	core.WriteJSON(w, http.StatusOK, out)
}

func (s *Server) CreateGoal(w http.ResponseWriter, r *http.Request, user *models.User) {
	body, ok := DecodeBody[models.GoalIn](w, r)
	if !ok {
		return
	}
	ctx := r.Context()
	now := time.Now().UTC()

	deadlineAt, err := time.Parse(time.RFC3339, body.DeadlineAt)
	if err != nil {
		core.WriteError(w, http.StatusBadRequest, "invalid_deadline", "Invalid deadline format.")
		return
	}

	tz := user.Timezone
	if body.Timezone != nil && strings.TrimSpace(*body.Timezone) != "" {
		tz = strings.TrimSpace(*body.Timezone)
	}

	goal := &models.Goal{
		ID:             uuid.New(),
		UserID:         user.ID,
		GoalName:       body.GoalName,
		DeadlineAt:     deadlineAt,
		Timezone:       tz,
		MotivationLine: body.MotivationLine,
		CreatedAt:      now,
		UpdatedAt:      now,
	}

	_, err = s.DB.Exec(
		ctx, nil,
		`INSERT INTO goals (id, user_id, goal_name, deadline_at, timezone, motivation_line, deleted_at, purge_at, created_at, updated_at)
		 VALUES (?, ?, ?, ?, ?, ?, NULL, NULL, ?, ?)`,
		s.DB.FormatUUID(goal.ID),
		s.DB.FormatUUID(user.ID),
		goal.GoalName,
		s.DB.FormatTime(goal.DeadlineAt),
		goal.Timezone,
		goal.MotivationLine,
		s.DB.FormatTime(now),
		s.DB.FormatTime(now),
	)
	if err != nil {
		core.WriteError(w, http.StatusInternalServerError, "internal_error", "Something went wrong. Please try again.")
		return
	}

	core.WriteJSON(w, http.StatusCreated, SerializeGoal(goal, 0))
}

func (s *Server) GetGoal(w http.ResponseWriter, r *http.Request, user *models.User) {
	goalID, ok := ParsePathUUID(w, r.PathValue("goal_id"))
	if !ok {
		return
	}
	ctx := r.Context()
	goal, appErr := s.OwnedGoal(ctx, nil, goalID, user, false)
	if appErr != nil {
		core.WriteAppError(w, appErr)
		return
	}

	dateCounts, totalActivities, err := s.loadGoalActivityDateCounts(ctx, goal.ID)
	if err != nil {
		core.WriteError(w, http.StatusInternalServerError, "internal_error", "Something went wrong. Please try again.")
		return
	}

	now := time.Now().UTC()
	progress := services.ComputeGoalProgress(goal, dateCounts, now)

	summary := models.GoalDailySummary{
		WorkedDays:        progress.WorkedDays,
		MissedDays:        progress.MissedDays,
		TotalDays:         progress.TotalTrackedDays,
		IsDeadlineReached: progress.IsDeadlineReached,
		InactivityWarning: progress.InactivityWarning,
		CurrentScore:      progress.CurrentScore,
	}

	out := models.GoalDetailOut{
		Goal:              SerializeGoal(goal, totalActivities),
		Summary:           summary,
		InactivityWarning: progress.InactivityWarning,
		MissedDays:        progress.MissedDays,
		WorkedDays:        progress.WorkedDays,
		TotalDays:         progress.TotalTrackedDays,
		CurrentScore:      progress.CurrentScore,
		IsDeadlineReached: progress.IsDeadlineReached,
	}

	core.WriteJSON(w, http.StatusOK, out)
}

func (s *Server) DeleteGoal(w http.ResponseWriter, r *http.Request, user *models.User) {
	goalID, ok := ParsePathUUID(w, r.PathValue("goal_id"))
	if !ok {
		return
	}
	ctx := r.Context()
	goal, appErr := s.OwnedGoal(ctx, nil, goalID, user, false)
	if appErr != nil {
		core.WriteAppError(w, appErr)
		return
	}

	now := time.Now().UTC()
	purgeAt := now.Add(goalTrashRetention)

	_, err := s.DB.Exec(
		ctx, nil,
		`UPDATE goals SET deleted_at = ?, purge_at = ?, updated_at = ? WHERE id = ?`,
		s.DB.FormatTime(now), s.DB.FormatTime(purgeAt), s.DB.FormatTime(now), s.DB.FormatUUID(goal.ID),
	)
	if err != nil {
		core.WriteError(w, http.StatusInternalServerError, "internal_error", "Something went wrong. Please try again.")
		return
	}

	core.WriteJSON(w, http.StatusOK, map[string]any{"ok": true})
}

func (s *Server) ListDeletedGoals(w http.ResponseWriter, r *http.Request, user *models.User) {
	ctx := r.Context()
	rows, err := s.DB.Query(
		ctx, nil,
		`SELECT g.id, g.user_id, g.goal_name, g.deadline_at, g.timezone, g.motivation_line, g.deleted_at, g.purge_at, g.created_at, g.updated_at,
		 (SELECT COUNT(a.id) FROM goal_activities a WHERE a.goal_id = g.id) AS total_activities
		 FROM goals g
		 WHERE g.user_id = ? AND g.deleted_at IS NOT NULL
		 ORDER BY g.deleted_at DESC`,
		s.DB.FormatUUID(user.ID),
	)
	if err != nil {
		core.WriteError(w, http.StatusInternalServerError, "internal_error", "Something went wrong. Please try again.")
		return
	}
	defer rows.Close()

	out := make([]models.GoalOut, 0)
	now := time.Now().UTC()
	for rows.Next() {
		var rawID, rawUID, name, tz string
		var deadline, motiv, delAt, purgeAt, created, updated any
		var totalAct int
		if err := rows.Scan(&rawID, &rawUID, &name, &deadline, &tz, &motiv, &delAt, &purgeAt, &created, &updated, &totalAct); err != nil {
			core.WriteError(w, http.StatusInternalServerError, "internal_error", "Something went wrong. Please try again.")
			return
		}
		id, err := database.ParseUUID(rawID)
		if err != nil {
			continue
		}
		deadlineAt, _ := database.ParseTime(deadline)
		deletedAt, _ := database.ParseNullTime(delAt)
		purgeAtTime, _ := database.ParseNullTime(purgeAt)
		createdAt, _ := database.ParseTime(created)
		updatedAt, _ := database.ParseTime(updated)

		isExpired := !now.Before(deadlineAt)
		out = append(out, models.GoalOut{
			ID:              id,
			GoalName:        name,
			DeadlineAt:      deadlineAt,
			Timezone:        tz,
			MotivationLine:  database.ParseNullString(motiv),
			DeletedAt:       deletedAt,
			PurgeAt:         purgeAtTime,
			CreatedAt:       createdAt,
			UpdatedAt:       updatedAt,
			IsExpired:       isExpired,
			TotalActivities: totalAct,
		})
	}

	core.WriteJSON(w, http.StatusOK, out)
}

func (s *Server) RestoreGoal(w http.ResponseWriter, r *http.Request, user *models.User) {
	goalID, ok := ParsePathUUID(w, r.PathValue("goal_id"))
	if !ok {
		return
	}
	ctx := r.Context()
	goal, appErr := s.OwnedGoal(ctx, nil, goalID, user, true)
	if appErr != nil {
		core.WriteAppError(w, appErr)
		return
	}
	if goal.DeletedAt == nil {
		core.WriteError(w, http.StatusBadRequest, "not_in_trash", "Goal is not in trash.")
		return
	}
	now := time.Now().UTC()
	if goal.PurgeAt != nil && !now.Before(goal.PurgeAt.UTC()) {
		core.WriteError(w, http.StatusGone, "trash_expired", "This goal's 3-day recovery window has expired.")
		return
	}

	_, err := s.DB.Exec(
		ctx, nil,
		`UPDATE goals SET deleted_at = NULL, purge_at = NULL, updated_at = ? WHERE id = ?`,
		s.DB.FormatTime(now), s.DB.FormatUUID(goal.ID),
	)
	if err != nil {
		core.WriteError(w, http.StatusInternalServerError, "internal_error", "Something went wrong. Please try again.")
		return
	}

	goal.DeletedAt = nil
	goal.PurgeAt = nil
	core.WriteJSON(w, http.StatusOK, SerializeGoal(goal, 0))
}

func (s *Server) PurgeGoal(w http.ResponseWriter, r *http.Request, user *models.User) {
	goalID, ok := ParsePathUUID(w, r.PathValue("goal_id"))
	if !ok {
		return
	}
	ctx := r.Context()
	goal, appErr := s.OwnedGoal(ctx, nil, goalID, user, true)
	if appErr != nil {
		core.WriteAppError(w, appErr)
		return
	}
	if goal.DeletedAt == nil {
		core.WriteError(w, http.StatusBadRequest, "not_in_trash", "Cannot permanently delete a goal that is not in trash.")
		return
	}

	err := s.DB.WithTx(ctx, func(tx *sql.Tx) error {
		return s.PurgeGoalRecord(ctx, tx, goal.ID)
	})
	if err != nil {
		core.WriteError(w, http.StatusInternalServerError, "internal_error", "Something went wrong. Please try again.")
		return
	}

	core.WriteJSON(w, http.StatusOK, map[string]any{"ok": true})
}

func (s *Server) AddGoalActivity(w http.ResponseWriter, r *http.Request, user *models.User) {
	goalID, ok := ParsePathUUID(w, r.PathValue("goal_id"))
	if !ok {
		return
	}
	body, ok := DecodeBody[models.GoalActivityIn](w, r)
	if !ok {
		return
	}

	ctx := r.Context()
	goal, appErr := s.OwnedGoal(ctx, nil, goalID, user, false)
	if appErr != nil {
		core.WriteAppError(w, appErr)
		return
	}

	now := time.Now().UTC()
	if !now.Before(goal.DeadlineAt) {
		core.WriteError(w, http.StatusBadRequest, "deadline_passed", "Cannot add activities after the goal deadline has passed.")
		return
	}

	// Compute activity date in goal's timezone
	activityDate := ""
	if body.ActivityDate != nil && strings.TrimSpace(*body.ActivityDate) != "" {
		activityDate = strings.TrimSpace(*body.ActivityDate)
	} else {
		loc, err := time.LoadLocation(goal.Timezone)
		if err != nil {
			loc = time.UTC
		}
		activityDate = now.In(loc).Format("2006-01-02")
	}

	activity := &models.GoalActivity{
		ID:           uuid.New(),
		GoalID:       goal.ID,
		UserID:       user.ID,
		ActivityText: body.ActivityText,
		ActivityDate: activityDate,
		CreatedAt:    now,
	}

	_, err := s.DB.Exec(
		ctx, nil,
		`INSERT INTO goal_activities (id, goal_id, user_id, activity_text, activity_date, created_at)
		 VALUES (?, ?, ?, ?, ?, ?)`,
		s.DB.FormatUUID(activity.ID),
		s.DB.FormatUUID(goal.ID),
		s.DB.FormatUUID(user.ID),
		activity.ActivityText,
		activity.ActivityDate,
		s.DB.FormatTime(now),
	)
	if err != nil {
		core.WriteError(w, http.StatusInternalServerError, "internal_error", "Something went wrong. Please try again.")
		return
	}

	core.WriteJSON(w, http.StatusCreated, models.GoalActivityOut{
		ID:           activity.ID,
		GoalID:       activity.GoalID,
		ActivityText: activity.ActivityText,
		ActivityDate: activity.ActivityDate,
		CreatedAt:    activity.CreatedAt,
	})
}

func (s *Server) ListGoalActivities(w http.ResponseWriter, r *http.Request, user *models.User) {
	goalID, ok := ParsePathUUID(w, r.PathValue("goal_id"))
	if !ok {
		return
	}
	ctx := r.Context()
	goal, appErr := s.OwnedGoal(ctx, nil, goalID, user, false)
	if appErr != nil {
		core.WriteAppError(w, appErr)
		return
	}

	page := 1
	if pStr := r.URL.Query().Get("page"); pStr != "" {
		if p, err := strconv.Atoi(pStr); err == nil && p > 0 {
			page = p
		}
	}
	perPage := 50
	if ppStr := r.URL.Query().Get("per_page"); ppStr != "" {
		if pp, err := strconv.Atoi(ppStr); err == nil && pp > 0 && pp <= 200 {
			perPage = pp
		}
	}
	offset := (page - 1) * perPage

	var total int
	if err := s.DB.QueryRow(
		ctx, nil,
		`SELECT COUNT(id) FROM goal_activities WHERE goal_id = ?`,
		s.DB.FormatUUID(goal.ID),
	).Scan(&total); err != nil {
		core.WriteError(w, http.StatusInternalServerError, "internal_error", "Something went wrong. Please try again.")
		return
	}

	rows, err := s.DB.Query(
		ctx, nil,
		`SELECT `+GoalActivityColumns+` FROM goal_activities
		 WHERE goal_id = ?
		 ORDER BY created_at DESC, id DESC
		 LIMIT ? OFFSET ?`,
		s.DB.FormatUUID(goal.ID), perPage, offset,
	)
	if err != nil {
		core.WriteError(w, http.StatusInternalServerError, "internal_error", "Something went wrong. Please try again.")
		return
	}
	defer rows.Close()

	activities := make([]models.GoalActivityOut, 0)
	for rows.Next() {
		act, err := ScanGoalActivity(rows)
		if err != nil {
			core.WriteError(w, http.StatusInternalServerError, "internal_error", "Something went wrong. Please try again.")
			return
		}
		activities = append(activities, models.GoalActivityOut{
			ID:           act.ID,
			GoalID:       act.GoalID,
			ActivityText: act.ActivityText,
			ActivityDate: act.ActivityDate,
			CreatedAt:    act.CreatedAt,
		})
	}

	core.WriteJSON(w, http.StatusOK, models.GoalActivitiesPageOut{
		Activities: activities,
		Total:      total,
		Page:       page,
		PerPage:    perPage,
	})
}

func (s *Server) GetGoalProgress(w http.ResponseWriter, r *http.Request, user *models.User) {
	goalID, ok := ParsePathUUID(w, r.PathValue("goal_id"))
	if !ok {
		return
	}
	ctx := r.Context()
	goal, appErr := s.OwnedGoal(ctx, nil, goalID, user, false)
	if appErr != nil {
		core.WriteAppError(w, appErr)
		return
	}

	dateCounts, _, err := s.loadGoalActivityDateCounts(ctx, goal.ID)
	if err != nil {
		core.WriteError(w, http.StatusInternalServerError, "internal_error", "Something went wrong. Please try again.")
		return
	}

	now := time.Now().UTC()
	progress := services.ComputeGoalProgress(goal, dateCounts, now)

	core.WriteJSON(w, http.StatusOK, models.GoalProgressOut{
		GoalID:            goal.ID,
		Points:            progress.Points,
		CurrentScore:      progress.CurrentScore,
		WorkedDays:        progress.WorkedDays,
		MissedDays:        progress.MissedDays,
		TotalTrackedDays:  progress.TotalTrackedDays,
		IsDeadlineReached: progress.IsDeadlineReached,
		InactivityWarning: progress.InactivityWarning,
	})
}

func (s *Server) loadGoalActivityDateCounts(ctx context.Context, goalID uuid.UUID) (map[string]int, int, error) {
	rows, err := s.DB.Query(
		ctx, nil,
		`SELECT activity_date, COUNT(id) FROM goal_activities WHERE goal_id = ? GROUP BY activity_date`,
		s.DB.FormatUUID(goalID),
	)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()

	counts := make(map[string]int)
	total := 0
	for rows.Next() {
		var dateStr string
		var count int
		if err := rows.Scan(&dateStr, &count); err != nil {
			return nil, 0, err
		}
		if len(dateStr) > 10 {
			dateStr = dateStr[:10]
		}
		counts[dateStr] = count
		total += count
	}
	return counts, total, nil
}
