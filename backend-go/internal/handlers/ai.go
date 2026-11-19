package handlers

import (
	"database/sql"
	"fmt"
	"net/http"
	"strings"
	"time"

	"skill-progress-tracker/backend-go/internal/core"
	"skill-progress-tracker/backend-go/internal/database"
	"skill-progress-tracker/backend-go/internal/models"
	"skill-progress-tracker/backend-go/internal/services"

	"github.com/google/uuid"
)

func sanitizeAINodes(nodesRaw any, depth int) ([]map[string]any, *core.AppError) {
	if depth > services.MaxTreeDepth {
		return nil, core.NewAppError(http.StatusBadRequest, "invalid_tree", "Topic hierarchy exceeds the maximum depth limit.")
	}
	var list []map[string]any
	switch v := nodesRaw.(type) {
	case []map[string]any:
		list = v
	case []any:
		for _, item := range v {
			if m, ok := item.(map[string]any); ok {
				list = append(list, m)
			}
		}
	default:
		return []map[string]any{}, nil
	}

	out := make([]map[string]any, 0, len(list))
	for _, node := range list {
		rawName := ""
		if node["name"] != nil {
			rawName = fmt.Sprintf("%v", node["name"])
		}
		name := strings.TrimSpace(rawName)
		if name == "" {
			return nil, core.NewAppError(http.StatusBadRequest, "empty_topic_name", "All topics must have a non-empty name before confirming.")
		}
		if runes := []rune(name); len(runes) > services.MaxNameLen {
			name = string(runes[:services.MaxNameLen])
		}
		var desc any
		if node["description"] != nil {
			dStr := fmt.Sprintf("%v", node["description"])
			if runes := []rune(dStr); len(runes) > 2000 {
				dStr = string(runes[:2000])
			}
			desc = dStr
		}
		children, appErr := sanitizeAINodes(node["children"], depth+1)
		if appErr != nil {
			return nil, appErr
		}
		out = append(out, map[string]any{
			"name":        name,
			"description": desc,
			"children":    children,
		})
	}
	return out, nil
}

func countAILeaves(nodes []map[string]any) int {
	leaves := 0
	stack := make([]map[string]any, len(nodes))
	copy(stack, nodes)
	for len(stack) > 0 {
		node := stack[len(stack)-1]
		stack = stack[:len(stack)-1]
		kids, _ := node["children"].([]map[string]any)
		if len(kids) == 0 {
			leaves++
		} else {
			stack = append(stack, kids...)
		}
	}
	return leaves
}

func (s *Server) GenerateSyllabus(w http.ResponseWriter, r *http.Request, _ *models.User) {
	body, ok := DecodeBody[models.GenerateSyllabusIn](w, r)
	if !ok {
		return
	}
	if appErr := s.Limiter.EnforceLimit(s.Cfg, r, "ai_generate", 40, 3600); appErr != nil {
		core.WriteAppError(w, appErr)
		return
	}
	result, err := services.GenerateSyllabus(r.Context(), s.Cfg, body.Subject, body.Depth, body.Purpose)
	if err != nil {
		if appErr, ok := err.(*core.AppError); ok {
			core.WriteAppError(w, appErr)
		} else {
			core.WriteError(w, http.StatusBadGateway, "ai_error", "AI generation failed. Please try again.")
		}
		return
	}
	core.WriteJSON(w, http.StatusOK, result)
}

func (s *Server) RefineSyllabus(w http.ResponseWriter, r *http.Request, _ *models.User) {
	body, ok := DecodeBody[models.RefineSyllabusIn](w, r)
	if !ok {
		return
	}
	if appErr := s.Limiter.EnforceLimit(s.Cfg, r, "ai_refine", 60, 3600); appErr != nil {
		core.WriteAppError(w, appErr)
		return
	}
	result, err := services.RefineSyllabus(
		r.Context(),
		s.Cfg,
		body.Subject,
		body.Depth,
		body.Purpose,
		body.CurrentTree,
		body.ConversationHistory,
		body.Instruction,
	)
	if err != nil {
		if appErr, ok := err.(*core.AppError); ok {
			core.WriteAppError(w, appErr)
		} else {
			core.WriteError(w, http.StatusBadGateway, "ai_error", "AI generation failed. Please try again.")
		}
		return
	}
	core.WriteJSON(w, http.StatusOK, result)
}

func (s *Server) ConfirmSyllabus(w http.ResponseWriter, r *http.Request, user *models.User) {
	body, ok := DecodeBody[models.ConfirmSyllabusIn](w, r)
	if !ok {
		return
	}
	if appErr := s.Limiter.EnforceLimit(s.Cfg, r, "ai_confirm", 30, 3600); appErr != nil {
		core.WriteAppError(w, appErr)
		return
	}

	topics, appErr := sanitizeAINodes(*body.Topics, 0)
	if appErr != nil {
		core.WriteAppError(w, appErr)
		return
	}
	if len(topics) == 0 {
		core.WriteError(w, http.StatusBadRequest, "empty_tree", "Cannot create a skill without topics.")
		return
	}
	totalNodes := services.CountMapNodes(topics)
	if totalNodes > services.MaxTreeNodes {
		core.WriteError(w, http.StatusBadRequest, "invalid_tree", fmt.Sprintf("Topic tree exceeds the %d topic limit.", services.MaxTreeNodes))
		return
	}

	ctx := r.Context()
	skillName := strings.TrimSpace(body.SkillName)
	if runes := []rune(skillName); len(runes) > 200 {
		skillName = string(runes[:200])
	}

	var existingID string
	err := s.DB.QueryRow(
		ctx, nil,
		`SELECT id FROM skills WHERE user_id = ? AND name = ?`,
		s.DB.FormatUUID(user.ID), skillName,
	).Scan(&existingID)
	if err == nil {
		core.WriteError(w, http.StatusConflict, "skill_exists", "You already have a skill with this name.")
		return
	}

	now := time.Now().UTC()
	skillID := uuid.New()

	err = s.DB.WithTx(ctx, func(tx *sql.Tx) error {
		if _, err := s.DB.Exec(
			ctx, tx,
			`INSERT INTO skills (id, user_id, name, description, deleted_at, purge_at, created_at, updated_at)
			 VALUES (?, ?, ?, ?, NULL, NULL, ?, ?)`,
			s.DB.FormatUUID(skillID),
			s.DB.FormatUUID(user.ID),
			skillName,
			body.SkillDescription,
			s.DB.FormatTime(now),
			s.DB.FormatTime(now),
		); err != nil {
			return err
		}
		if _, err := services.PersistTopicNodes(ctx, s.DB, tx, skillID, topics, nil); err != nil {
			return err
		}
		sid := skillID
		sname := skillName
		summary := fmt.Sprintf("You generated %q using AI", skillName)
		return services.LogActivity(ctx, s.DB, tx, user, models.ActionAIGenerated, nil, &sid, nil, &sname, nil, &summary)
	})
	if err != nil {
		if database.IsUniqueViolation(err) {
			core.WriteError(w, http.StatusConflict, "skill_exists", "You already have a skill with this name.")
			return
		}
		core.WriteError(w, http.StatusInternalServerError, "internal_error", "Something went wrong. Please try again.")
		return
	}

	totalLeaves := countAILeaves(topics)
	core.WriteJSON(w, http.StatusCreated, models.SkillOut{
		ID:              skillID,
		Name:            skillName,
		Description:     body.SkillDescription,
		CreatedAt:       now,
		CompletedLeaves: 0,
		TotalLeaves:     totalLeaves,
		Percent:         0.0,
		Empty:           totalLeaves == 0,
		DeletedAt:       nil,
		PurgeAt:         nil,
	})
}
