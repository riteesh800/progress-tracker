package handlers

import (
	"context"
	"database/sql"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/google/uuid"
	"skill-progress-tracker/backend-go/internal/core"
	"skill-progress-tracker/backend-go/internal/database"
	"skill-progress-tracker/backend-go/internal/models"
	"skill-progress-tracker/backend-go/internal/services"
)

const skillTrashRetention = 72 * time.Hour

func (s *Server) purgeSkillRecord(ctx context.Context, q database.Querier, skillID uuid.UUID) error {
	fSkillID := s.DB.FormatUUID(skillID)
	if _, err := s.DB.Exec(
		ctx, q,
		`UPDATE activity_log SET topic_id = NULL WHERE topic_id IN (SELECT id FROM topics WHERE skill_id = ?)`,
		fSkillID,
	); err != nil {
		return err
	}
	if _, err := s.DB.Exec(ctx, q, `UPDATE activity_log SET skill_id = NULL WHERE skill_id = ?`, fSkillID); err != nil {
		return err
	}
	if _, err := s.DB.Exec(ctx, q, `UPDATE import_jobs SET skill_id = NULL WHERE skill_id = ?`, fSkillID); err != nil {
		return err
	}
	if _, err := s.DB.Exec(
		ctx, q,
		`DELETE FROM notes WHERE topic_id IN (SELECT id FROM topics WHERE skill_id = ?)`,
		fSkillID,
	); err != nil {
		return err
	}
	if _, err := s.DB.Exec(ctx, q, `DELETE FROM skill_timers WHERE skill_id = ?`, fSkillID); err != nil {
		return err
	}
	if _, err := s.DB.Exec(ctx, q, `DELETE FROM topics WHERE skill_id = ?`, fSkillID); err != nil {
		return err
	}
	_, err := s.DB.Exec(ctx, q, `DELETE FROM skills WHERE id = ?`, fSkillID)
	return err
}

func (s *Server) purgeExpiredSkillsForUser(ctx context.Context, userID uuid.UUID) error {
	now := time.Now().UTC()
	rows, err := s.DB.Query(
		ctx, nil,
		`SELECT id, purge_at FROM skills WHERE user_id = ? AND deleted_at IS NOT NULL AND purge_at IS NOT NULL`,
		s.DB.FormatUUID(userID),
	)
	if err != nil {
		return err
	}
	var expiredIDs []uuid.UUID
	for rows.Next() {
		var rawID string
		var rawPurge any
		if err := rows.Scan(&rawID, &rawPurge); err != nil {
			continue
		}
		id, err := database.ParseUUID(rawID)
		if err != nil {
			continue
		}
		pa, err := database.ParseNullTime(rawPurge)
		if err != nil || pa == nil {
			continue
		}
		if !now.Before(pa.UTC()) {
			expiredIDs = append(expiredIDs, id)
		}
	}
	rows.Close()

	for _, sid := range expiredIDs {
		_ = s.DB.WithTx(ctx, func(tx *sql.Tx) error {
			return s.purgeSkillRecord(ctx, tx, sid)
		})
	}
	return nil
}

func (s *Server) ListSkills(w http.ResponseWriter, r *http.Request, user *models.User) {
	ctx := r.Context()
	_ = s.purgeExpiredSkillsForUser(ctx, user.ID)
	rows, err := s.DB.Query(
		ctx, nil,
		`SELECT `+SkillColumns+` FROM skills WHERE user_id = ? AND deleted_at IS NULL ORDER BY created_at ASC`,
		s.DB.FormatUUID(user.ID),
	)
	if err != nil {
		core.WriteError(w, http.StatusInternalServerError, "internal_error", "Something went wrong. Please try again.")
		return
	}
	defer rows.Close()

	var skills []*models.Skill
	var skillIDs []uuid.UUID
	for rows.Next() {
		sk, err := ScanSkill(rows)
		if err != nil {
			core.WriteError(w, http.StatusInternalServerError, "internal_error", "Something went wrong. Please try again.")
			return
		}
		skills = append(skills, sk)
		skillIDs = append(skillIDs, sk.ID)
	}
	rows.Close()

	topicsMap, err := s.LoadSkillsTopicsMap(ctx, nil, skillIDs)
	if err != nil {
		core.WriteError(w, http.StatusInternalServerError, "internal_error", "Something went wrong. Please try again.")
		return
	}
	out := make([]models.SkillOut, 0, len(skills))
	for _, sk := range skills {
		out = append(out, SerializeSkill(sk, topicsMap[sk.ID]))
	}
	core.WriteJSON(w, http.StatusOK, out)
}

func (s *Server) ListDeletedSkills(w http.ResponseWriter, r *http.Request, user *models.User) {
	ctx := r.Context()
	_ = s.purgeExpiredSkillsForUser(ctx, user.ID)
	rows, err := s.DB.Query(
		ctx, nil,
		`SELECT `+SkillColumns+` FROM skills WHERE user_id = ? AND deleted_at IS NOT NULL ORDER BY deleted_at DESC`,
		s.DB.FormatUUID(user.ID),
	)
	if err != nil {
		core.WriteError(w, http.StatusInternalServerError, "internal_error", "Something went wrong. Please try again.")
		return
	}
	defer rows.Close()

	var skills []*models.Skill
	var skillIDs []uuid.UUID
	for rows.Next() {
		sk, err := ScanSkill(rows)
		if err != nil {
			core.WriteError(w, http.StatusInternalServerError, "internal_error", "Something went wrong. Please try again.")
			return
		}
		skills = append(skills, sk)
		skillIDs = append(skillIDs, sk.ID)
	}
	rows.Close()

	topicsMap, err := s.LoadSkillsTopicsMap(ctx, nil, skillIDs)
	if err != nil {
		core.WriteError(w, http.StatusInternalServerError, "internal_error", "Something went wrong. Please try again.")
		return
	}
	out := make([]models.SkillOut, 0, len(skills))
	for _, sk := range skills {
		out = append(out, SerializeSkill(sk, topicsMap[sk.ID]))
	}
	core.WriteJSON(w, http.StatusOK, out)
}

func (s *Server) CreateSkill(w http.ResponseWriter, r *http.Request, user *models.User) {
	body, ok := DecodeBody[models.SkillIn](w, r)
	if !ok {
		return
	}
	ctx := r.Context()
	now := time.Now().UTC()

	row := s.DB.QueryRow(
		ctx, nil,
		`SELECT `+SkillColumns+` FROM skills WHERE user_id = ? AND name = ?`,
		s.DB.FormatUUID(user.ID), body.Name,
	)
	existing, err := ScanSkill(row)
	if err == nil && existing != nil {
		if existing.DeletedAt != nil {
			if existing.PurgeAt != nil && !now.Before(existing.PurgeAt.UTC()) {
				_ = s.DB.WithTx(ctx, func(tx *sql.Tx) error {
					return s.purgeSkillRecord(ctx, tx, existing.ID)
				})
			} else {
				core.WriteError(
					w,
					http.StatusConflict,
					"skill_exists",
					"You already have a skill with this name (check your Trash if you recently deleted it).",
				)
				return
			}
		} else {
			core.WriteError(w, http.StatusConflict, "skill_exists", "You already have a skill with this name.")
			return
		}
	}

	skill := &models.Skill{
		ID:          uuid.New(),
		UserID:      user.ID,
		Name:        body.Name,
		Description: body.Description,
		CreatedAt:   now,
		UpdatedAt:   now,
	}

	err = s.DB.WithTx(ctx, func(tx *sql.Tx) error {
		_, err := s.DB.Exec(
			ctx, tx,
			`INSERT INTO skills (id, user_id, name, description, deleted_at, purge_at, created_at, updated_at)
			 VALUES (?, ?, ?, ?, NULL, NULL, ?, ?)`,
			s.DB.FormatUUID(skill.ID),
			s.DB.FormatUUID(user.ID),
			skill.Name,
			skill.Description,
			s.DB.FormatTime(now),
			s.DB.FormatTime(now),
		)
		if err != nil {
			return err
		}
		sid := skill.ID
		sname := skill.Name
		summary := fmt.Sprintf("You added the skill %q", skill.Name)
		return services.LogActivity(ctx, s.DB, tx, user, models.ActionSkillCreated, nil, &sid, nil, &sname, nil, &summary)
	})
	if err != nil {
		if database.IsUniqueViolation(err) {
			core.WriteError(w, http.StatusConflict, "skill_exists", "You already have a skill with this name.")
			return
		}
		core.WriteError(w, http.StatusInternalServerError, "internal_error", "Something went wrong. Please try again.")
		return
	}
	core.WriteJSON(w, http.StatusCreated, SerializeSkill(skill, nil))
}

func (s *Server) GetSkill(w http.ResponseWriter, r *http.Request, user *models.User) {
	skillID, ok := ParsePathUUID(w, r.PathValue("skill_id"))
	if !ok {
		return
	}
	ctx := r.Context()
	skill, appErr := s.OwnedSkill(ctx, nil, skillID, user, false)
	if appErr != nil {
		core.WriteAppError(w, appErr)
		return
	}
	topics, err := s.LoadSkillTopics(ctx, nil, skill.ID)
	if err != nil {
		core.WriteError(w, http.StatusInternalServerError, "internal_error", "Something went wrong. Please try again.")
		return
	}
	core.WriteJSON(w, http.StatusOK, SerializeSkill(skill, topics))
}

func (s *Server) GetSkillTree(w http.ResponseWriter, r *http.Request, user *models.User) {
	skillID, ok := ParsePathUUID(w, r.PathValue("skill_id"))
	if !ok {
		return
	}
	ctx := r.Context()
	skill, appErr := s.OwnedSkill(ctx, nil, skillID, user, false)
	if appErr != nil {
		core.WriteAppError(w, appErr)
		return
	}
	topics, err := s.LoadSkillTopics(ctx, nil, skill.ID)
	if err != nil {
		core.WriteError(w, http.StatusInternalServerError, "internal_error", "Something went wrong. Please try again.")
		return
	}
	noteTopicIDs := make(map[uuid.UUID]bool)
	nRows, nErr := s.DB.Query(ctx, nil, `SELECT DISTINCT topic_id FROM notes WHERE user_id = ?`, s.DB.FormatUUID(user.ID))
	if nErr == nil {
		defer nRows.Close()
		for nRows.Next() {
			var rawTID any
			if err := nRows.Scan(&rawTID); err == nil {
				if tid, err := database.ParseUUID(rawTID); err == nil {
					noteTopicIDs[tid] = true
				}
			}
		}
	}
	tree := services.BuildTopicTreeWithNotes(topics, noteTopicIDs)
	if tree == nil {
		tree = []models.TopicNodeOut{}
	}
	core.WriteJSON(w, http.StatusOK, tree)
}

func (s *Server) UpdateSkill(w http.ResponseWriter, r *http.Request, user *models.User) {
	skillID, ok := ParsePathUUID(w, r.PathValue("skill_id"))
	if !ok {
		return
	}
	body, ok := DecodeBody[models.SkillUpdateIn](w, r)
	if !ok {
		return
	}
	ctx := r.Context()
	skill, appErr := s.OwnedSkill(ctx, nil, skillID, user, false)
	if appErr != nil {
		core.WriteAppError(w, appErr)
		return
	}
	if body.Name != nil && *body.Name != skill.Name {
		var existingID string
		err := s.DB.QueryRow(
			ctx, nil,
			`SELECT id FROM skills WHERE user_id = ? AND name = ?`,
			s.DB.FormatUUID(user.ID), *body.Name,
		).Scan(&existingID)
		if err == nil {
			core.WriteError(w, http.StatusConflict, "skill_exists", "You already have a skill with this name.")
			return
		}
		skill.Name = *body.Name
	}
	if body.Description != nil {
		skill.Description = body.Description
	}
	now := time.Now().UTC()
	_, err := s.DB.Exec(
		ctx, nil,
		`UPDATE skills SET name = ?, description = ?, updated_at = ? WHERE id = ?`,
		skill.Name, skill.Description, s.DB.FormatTime(now), s.DB.FormatUUID(skill.ID),
	)
	if err != nil {
		if database.IsUniqueViolation(err) {
			core.WriteError(w, http.StatusConflict, "skill_exists", "You already have a skill with this name.")
			return
		}
		core.WriteError(w, http.StatusInternalServerError, "internal_error", "Something went wrong. Please try again.")
		return
	}
	topics, err := s.LoadSkillTopics(ctx, nil, skill.ID)
	if err != nil {
		core.WriteError(w, http.StatusInternalServerError, "internal_error", "Something went wrong. Please try again.")
		return
	}
	core.WriteJSON(w, http.StatusOK, SerializeSkill(skill, topics))
}

func (s *Server) countSkillImpact(ctx context.Context, skillID uuid.UUID) (int, int, error) {
	var tCount, nCount int
	if err := s.DB.QueryRow(ctx, nil, `SELECT COUNT(id) FROM topics WHERE skill_id = ?`, s.DB.FormatUUID(skillID)).Scan(&tCount); err != nil {
		return 0, 0, err
	}
	if err := s.DB.QueryRow(
		ctx, nil,
		`SELECT COUNT(n.id) FROM notes n JOIN topics t ON t.id = n.topic_id WHERE t.skill_id = ?`,
		s.DB.FormatUUID(skillID),
	).Scan(&nCount); err != nil {
		return 0, 0, err
	}
	return tCount, nCount, nil
}

func (s *Server) SkillDeleteImpact(w http.ResponseWriter, r *http.Request, user *models.User) {
	skillID, ok := ParsePathUUID(w, r.PathValue("skill_id"))
	if !ok {
		return
	}
	ctx := r.Context()
	skill, appErr := s.OwnedSkill(ctx, nil, skillID, user, false)
	if appErr != nil {
		core.WriteAppError(w, appErr)
		return
	}
	tCount, nCount, err := s.countSkillImpact(ctx, skill.ID)
	if err != nil {
		core.WriteError(w, http.StatusInternalServerError, "internal_error", "Something went wrong. Please try again.")
		return
	}
	core.WriteJSON(w, http.StatusOK, models.DeleteImpactOut{
		Descendants:     tCount,
		Notes:           nCount,
		RequiresConfirm: tCount > 0 || nCount > 0,
	})
}

func (s *Server) DeleteSkill(w http.ResponseWriter, r *http.Request, user *models.User) {
	skillID, ok := ParsePathUUID(w, r.PathValue("skill_id"))
	if !ok {
		return
	}
	confirmStr := strings.ToLower(strings.TrimSpace(r.URL.Query().Get("confirm")))
	confirm := confirmStr == "true" || confirmStr == "1"

	ctx := r.Context()
	skill, appErr := s.OwnedSkill(ctx, nil, skillID, user, false)
	if appErr != nil {
		core.WriteAppError(w, appErr)
		return
	}
	tCount, nCount, err := s.countSkillImpact(ctx, skill.ID)
	if err != nil {
		core.WriteError(w, http.StatusInternalServerError, "internal_error", "Something went wrong. Please try again.")
		return
	}
	if (tCount > 0 || nCount > 0) && !confirm {
		core.WriteError(
			w,
			http.StatusConflict,
			"confirm_required",
			fmt.Sprintf("Deleting this skill removes %d topics and %d notes. Retry with confirm=true.", tCount, nCount),
		)
		return
	}

	now := time.Now().UTC()
	purgeAt := now.Add(skillTrashRetention)
	summary := fmt.Sprintf("You deleted the skill %q", skill.Name)
	sname := skill.Name

	err = s.DB.WithTx(ctx, func(tx *sql.Tx) error {
		if err := services.LogActivity(ctx, s.DB, tx, user, models.ActionSkillDeleted, nil, nil, nil, &sname, nil, &summary); err != nil {
			return err
		}
		_, err := s.DB.Exec(
			ctx, tx,
			`UPDATE skills SET deleted_at = ?, purge_at = ?, updated_at = ? WHERE id = ?`,
			s.DB.FormatTime(now), s.DB.FormatTime(purgeAt), s.DB.FormatTime(now), s.DB.FormatUUID(skill.ID),
		)
		return err
	})
	if err != nil {
		core.WriteError(w, http.StatusInternalServerError, "internal_error", "Something went wrong. Please try again.")
		return
	}
	core.WriteJSON(w, http.StatusOK, map[string]any{"ok": true})
}

func (s *Server) RestoreSkill(w http.ResponseWriter, r *http.Request, user *models.User) {
	skillID, ok := ParsePathUUID(w, r.PathValue("skill_id"))
	if !ok {
		return
	}
	ctx := r.Context()
	skill, appErr := s.OwnedSkill(ctx, nil, skillID, user, true)
	if appErr != nil {
		core.WriteAppError(w, appErr)
		return
	}
	if skill.DeletedAt == nil {
		core.WriteError(w, http.StatusBadRequest, "not_in_trash", "Skill is not in trash.")
		return
	}
	now := time.Now().UTC()
	if skill.PurgeAt != nil && !now.Before(skill.PurgeAt.UTC()) {
		core.WriteError(w, http.StatusGone, "trash_expired", "This skill's 3-day recovery window has expired.")
		return
	}
	_, err := s.DB.Exec(
		ctx, nil,
		`UPDATE skills SET deleted_at = NULL, purge_at = NULL, updated_at = ? WHERE id = ?`,
		s.DB.FormatTime(now), s.DB.FormatUUID(skill.ID),
	)
	if err != nil {
		core.WriteError(w, http.StatusInternalServerError, "internal_error", "Something went wrong. Please try again.")
		return
	}
	skill.DeletedAt = nil
	skill.PurgeAt = nil
	topics, err := s.LoadSkillTopics(ctx, nil, skill.ID)
	if err != nil {
		core.WriteError(w, http.StatusInternalServerError, "internal_error", "Something went wrong. Please try again.")
		return
	}
	core.WriteJSON(w, http.StatusOK, SerializeSkill(skill, topics))
}

func (s *Server) PurgeSkill(w http.ResponseWriter, r *http.Request, user *models.User) {
	skillID, ok := ParsePathUUID(w, r.PathValue("skill_id"))
	if !ok {
		return
	}
	ctx := r.Context()
	skill, appErr := s.OwnedSkill(ctx, nil, skillID, user, true)
	if appErr != nil {
		core.WriteAppError(w, appErr)
		return
	}
	if skill.DeletedAt == nil {
		core.WriteError(w, http.StatusBadRequest, "not_in_trash", "Cannot permanently delete a skill that is not in trash.")
		return
	}
	sname := skill.Name
	summary := fmt.Sprintf("You permanently deleted the skill %q", skill.Name)
	err := s.DB.WithTx(ctx, func(tx *sql.Tx) error {
		if err := services.LogActivity(ctx, s.DB, tx, user, models.ActionSkillDeleted, nil, nil, nil, &sname, nil, &summary); err != nil {
			return err
		}
		return s.purgeSkillRecord(ctx, tx, skill.ID)
	})
	if err != nil {
		core.WriteError(w, http.StatusInternalServerError, "internal_error", "Something went wrong. Please try again.")
		return
	}
	core.WriteJSON(w, http.StatusOK, map[string]any{"ok": true})
}
