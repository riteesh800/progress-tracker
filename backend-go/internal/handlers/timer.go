package handlers

import (
	"database/sql"
	"net/http"
	"time"

	"github.com/google/uuid"
	"skill-progress-tracker/backend-go/internal/core"
	"skill-progress-tracker/backend-go/internal/database"
	"skill-progress-tracker/backend-go/internal/models"
)

func (s *Server) GetTimer(w http.ResponseWriter, r *http.Request, user *models.User) {
	skillID, ok := ParsePathUUID(w, r.PathValue("skill_id"))
	if !ok {
		return
	}
	ctx := r.Context()
	if _, appErr := s.OwnedSkill(ctx, nil, skillID, user, false); appErr != nil {
		core.WriteAppError(w, appErr)
		return
	}
	var rawID, rawSID string
	var fromVal, toVal any
	err := s.DB.QueryRow(
		ctx, nil,
		`SELECT id, skill_id, from_date, to_date FROM skill_timers WHERE user_id = ? AND skill_id = ?`,
		s.DB.FormatUUID(user.ID), s.DB.FormatUUID(skillID),
	).Scan(&rawID, &rawSID, &fromVal, &toVal)
	if err != nil {
		if err == sql.ErrNoRows {
			core.WriteJSON(w, http.StatusOK, nil)
			return
		}
		core.WriteError(w, http.StatusInternalServerError, "internal_error", "Something went wrong. Please try again.")
		return
	}
	id, err := database.ParseUUID(rawID)
	if err != nil {
		core.WriteError(w, http.StatusInternalServerError, "internal_error", "Something went wrong. Please try again.")
		return
	}
	sid, err := database.ParseUUID(rawSID)
	if err != nil {
		core.WriteError(w, http.StatusInternalServerError, "internal_error", "Something went wrong. Please try again.")
		return
	}
	core.WriteJSON(w, http.StatusOK, models.SkillTimerOut{
		ID:       id,
		SkillID:  sid,
		FromDate: database.ParseDateString(fromVal),
		ToDate:   database.ParseDateString(toVal),
	})
}

func (s *Server) UpsertTimer(w http.ResponseWriter, r *http.Request, user *models.User) {
	skillID, ok := ParsePathUUID(w, r.PathValue("skill_id"))
	if !ok {
		return
	}
	body, ok := DecodeBody[models.SkillTimerIn](w, r)
	if !ok {
		return
	}
	if body.FromDate >= body.ToDate {
		core.WriteError(w, http.StatusBadRequest, "invalid_dates", "to_date must be strictly after from_date")
		return
	}
	ctx := r.Context()
	if _, appErr := s.OwnedSkill(ctx, nil, skillID, user, false); appErr != nil {
		core.WriteAppError(w, appErr)
		return
	}

	now := time.Now().UTC()
	var rawID string
	err := s.DB.QueryRow(
		ctx, nil,
		`SELECT id FROM skill_timers WHERE user_id = ? AND skill_id = ?`,
		s.DB.FormatUUID(user.ID), s.DB.FormatUUID(skillID),
	).Scan(&rawID)
	if err == nil {
		id, parseErr := database.ParseUUID(rawID)
		if parseErr != nil {
			core.WriteError(w, http.StatusInternalServerError, "internal_error", "Something went wrong. Please try again.")
			return
		}
		_, err = s.DB.Exec(
			ctx, nil,
			`UPDATE skill_timers SET from_date = ?, to_date = ?, updated_at = ? WHERE id = ?`,
			body.FromDate, body.ToDate, s.DB.FormatTime(now), s.DB.FormatUUID(id),
		)
		if err != nil {
			core.WriteError(w, http.StatusInternalServerError, "internal_error", "Something went wrong. Please try again.")
			return
		}
		core.WriteJSON(w, http.StatusOK, models.SkillTimerOut{
			ID:       id,
			SkillID:  skillID,
			FromDate: body.FromDate,
			ToDate:   body.ToDate,
		})
		return
	}

	newID := uuid.New()
	_, err = s.DB.Exec(
		ctx, nil,
		`INSERT INTO skill_timers (id, user_id, skill_id, from_date, to_date, created_at, updated_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?)`,
		s.DB.FormatUUID(newID),
		s.DB.FormatUUID(user.ID),
		s.DB.FormatUUID(skillID),
		body.FromDate,
		body.ToDate,
		s.DB.FormatTime(now),
		s.DB.FormatTime(now),
	)
	if err != nil {
		core.WriteError(w, http.StatusInternalServerError, "internal_error", "Something went wrong. Please try again.")
		return
	}
	core.WriteJSON(w, http.StatusOK, models.SkillTimerOut{
		ID:       newID,
		SkillID:  skillID,
		FromDate: body.FromDate,
		ToDate:   body.ToDate,
	})
}

func (s *Server) DeleteTimer(w http.ResponseWriter, r *http.Request, user *models.User) {
	skillID, ok := ParsePathUUID(w, r.PathValue("skill_id"))
	if !ok {
		return
	}
	ctx := r.Context()
	if _, appErr := s.OwnedSkill(ctx, nil, skillID, user, false); appErr != nil {
		core.WriteAppError(w, appErr)
		return
	}
	_, err := s.DB.Exec(
		ctx, nil,
		`DELETE FROM skill_timers WHERE user_id = ? AND skill_id = ?`,
		s.DB.FormatUUID(user.ID), s.DB.FormatUUID(skillID),
	)
	if err != nil {
		core.WriteError(w, http.StatusInternalServerError, "internal_error", "Something went wrong. Please try again.")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
