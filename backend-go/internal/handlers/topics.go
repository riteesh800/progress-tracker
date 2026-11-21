package handlers

import (
	"database/sql"
	"fmt"
	"net/http"
	"time"

	"github.com/google/uuid"
	"skill-progress-tracker/backend-go/internal/core"
	"skill-progress-tracker/backend-go/internal/database"
	"skill-progress-tracker/backend-go/internal/models"
	"skill-progress-tracker/backend-go/internal/services"
)

func (s *Server) CreateTopic(w http.ResponseWriter, r *http.Request, user *models.User) {
	body, ok := DecodeBody[models.TopicIn](w, r)
	if !ok {
		return
	}
	ctx := r.Context()
	skill, appErr := s.OwnedSkill(ctx, nil, body.ParsedSkillID, user, false)
	if appErr != nil {
		core.WriteAppError(w, appErr)
		return
	}

	var parent *models.Topic
	var parentName *string
	if body.ParsedParentID != nil {
		p, appErr := s.OwnedTopic(ctx, nil, *body.ParsedParentID, user, false)
		if appErr != nil {
			core.WriteAppError(w, appErr)
			return
		}
		if p.SkillID != skill.ID {
			core.WriteError(w, http.StatusUnprocessableEntity, "invalid_parent", "Parent topic must belong to the same skill.")
			return
		}
		parent = p
		pn := p.Name
		parentName = &pn
	}

	allTopics, err := s.LoadSkillTopics(ctx, nil, skill.ID)
	if err != nil {
		core.WriteError(w, http.StatusInternalServerError, "internal_error", "Something went wrong. Please try again.")
		return
	}

	maxOrder := -1
	for _, t := range allTopics {
		sameParent := (t.ParentID == nil && body.ParsedParentID == nil) ||
			(t.ParentID != nil && body.ParsedParentID != nil && *t.ParentID == *body.ParsedParentID)
		if sameParent && t.OrderIndex > maxOrder {
			maxOrder = t.OrderIndex
		}
	}

	now := time.Now().UTC()
	topic := models.Topic{
		ID:          uuid.New(),
		SkillID:     skill.ID,
		ParentID:    body.ParsedParentID,
		Name:        body.Name,
		Description: body.Description,
		OrderIndex:  maxOrder + 1,
		IsCompleted: false,
		CompletedAt: nil,
		CreatedAt:   now,
		UpdatedAt:   now,
	}

	targetName := skill.Name
	if parentName != nil {
		targetName = *parentName
	}
	summary := fmt.Sprintf("You added %q to %q", topic.Name, targetName)

	err = s.DB.WithTx(ctx, func(tx *sql.Tx) error {
		if parent != nil && parent.IsCompleted {
			if _, err := s.DB.Exec(
				ctx, tx,
				`UPDATE topics SET is_completed = ?, completed_at = NULL, updated_at = ? WHERE id = ?`,
				false, s.DB.FormatTime(now), s.DB.FormatUUID(parent.ID),
			); err != nil {
				return err
			}
		}
		if _, err := s.DB.Exec(
			ctx, tx,
			`INSERT INTO topics (id, skill_id, parent_id, name, description, order_index, is_completed, completed_at, created_at, updated_at)
			 VALUES (?, ?, ?, ?, ?, ?, ?, NULL, ?, ?)`,
			s.DB.FormatUUID(topic.ID),
			s.DB.FormatUUID(topic.SkillID),
			s.DB.FormatNullUUID(topic.ParentID),
			topic.Name,
			topic.Description,
			topic.OrderIndex,
			false,
			s.DB.FormatTime(now),
			s.DB.FormatTime(now),
		); err != nil {
			return err
		}
		tid := topic.ID
		sid := skill.ID
		tname := topic.Name
		sname := skill.Name
		return services.LogActivity(ctx, s.DB, tx, user, models.ActionTopicCreated, &tid, &sid, &tname, &sname, parentName, &summary)
	})
	if err != nil {
		core.WriteError(w, http.StatusInternalServerError, "internal_error", "Something went wrong. Please try again.")
		return
	}

	core.WriteJSON(w, http.StatusCreated, models.TopicNodeOut{
		ID:              topic.ID,
		SkillID:         topic.SkillID,
		ParentID:        topic.ParentID,
		Name:            topic.Name,
		Description:     topic.Description,
		OrderIndex:      topic.OrderIndex,
		IsLeaf:          true,
		IsCompleted:     false,
		CompletedLeaves: 0,
		TotalLeaves:     1,
		Percent:         0.0,
		Children:        []models.TopicNodeOut{},
	})
}

func (s *Server) GetTopic(w http.ResponseWriter, r *http.Request, user *models.User) {
	topicID, ok := ParsePathUUID(w, r.PathValue("topic_id"))
	if !ok {
		return
	}
	ctx := r.Context()
	topic, appErr := s.OwnedTopic(ctx, nil, topicID, user, false)
	if appErr != nil {
		core.WriteAppError(w, appErr)
		return
	}
	topics, err := s.LoadSkillTopics(ctx, nil, topic.SkillID)
	if err != nil {
		core.WriteError(w, http.StatusInternalServerError, "internal_error", "Something went wrong. Please try again.")
		return
	}
	noteTopicIDs := make(map[uuid.UUID]bool)
	if nRows, nErr := s.DB.Query(ctx, nil, `SELECT DISTINCT topic_id FROM notes WHERE user_id = ?`, s.DB.FormatUUID(user.ID)); nErr == nil {
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
	engine := services.NewProgressEngine(topics)
	engine.NoteTopicIDs = noteTopicIDs
	node := engine.BuildNode(*topic, nil)
	core.WriteJSON(w, http.StatusOK, node)
}

func (s *Server) TopicChildren(w http.ResponseWriter, r *http.Request, user *models.User) {
	topicID, ok := ParsePathUUID(w, r.PathValue("topic_id"))
	if !ok {
		return
	}
	ctx := r.Context()
	topic, appErr := s.OwnedTopic(ctx, nil, topicID, user, false)
	if appErr != nil {
		core.WriteAppError(w, appErr)
		return
	}
	topics, err := s.LoadSkillTopics(ctx, nil, topic.SkillID)
	if err != nil {
		core.WriteError(w, http.StatusInternalServerError, "internal_error", "Something went wrong. Please try again.")
		return
	}
	noteTopicIDs := make(map[uuid.UUID]bool)
	if nRows, nErr := s.DB.Query(ctx, nil, `SELECT DISTINCT topic_id FROM notes WHERE user_id = ?`, s.DB.FormatUUID(user.ID)); nErr == nil {
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
	engine := services.NewProgressEngine(topics)
	engine.NoteTopicIDs = noteTopicIDs
	children := engine.Children[topic.ID]
	out := make([]models.TopicNodeOut, 0, len(children))
	for _, c := range children {
		out = append(out, engine.BuildNode(c, nil))
	}
	core.WriteJSON(w, http.StatusOK, out)
}

func (s *Server) CompleteTopic(w http.ResponseWriter, r *http.Request, user *models.User) {
	topicID, ok := ParsePathUUID(w, r.PathValue("topic_id"))
	if !ok {
		return
	}
	body, ok := DecodeBody[models.TopicCompleteIn](w, r)
	if !ok {
		return
	}
	ctx := r.Context()
	topic, appErr := s.OwnedTopic(ctx, nil, topicID, user, false)
	if appErr != nil {
		core.WriteAppError(w, appErr)
		return
	}
	topics, err := s.LoadSkillTopics(ctx, nil, topic.SkillID)
	if err != nil {
		core.WriteError(w, http.StatusInternalServerError, "internal_error", "Something went wrong. Please try again.")
		return
	}
	noteTopicIDs := make(map[uuid.UUID]bool)
	if nRows, nErr := s.DB.Query(ctx, nil, `SELECT DISTINCT topic_id FROM notes WHERE user_id = ?`, s.DB.FormatUUID(user.ID)); nErr == nil {
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
	engine := services.NewProgressEngine(topics)
	engine.NoteTopicIDs = noteTopicIDs
	if !engine.IsLeaf(topic.ID) {
		core.WriteError(w, http.StatusUnprocessableEntity, "not_a_leaf", "Only leaf topics can be marked complete.")
		return
	}

	completed := *body.Completed
	if completed == topic.IsCompleted {
		node := engine.BuildNode(engine.Topics[topic.ID], nil)
		core.WriteJSON(w, http.StatusOK, node)
		return
	}

	skillName := "Unknown skill"
	var sn string
	if err := s.DB.QueryRow(ctx, nil, `SELECT name FROM skills WHERE id = ?`, s.DB.FormatUUID(topic.SkillID)).Scan(&sn); err == nil {
		skillName = sn
	}

	now := time.Now().UTC()
	var compAt *time.Time
	if completed {
		compAt = &now
	}

	for i := range topics {
		if topics[i].ID == topic.ID {
			topics[i].IsCompleted = completed
			topics[i].CompletedAt = compAt
			break
		}
	}
	engineAfter := services.NewProgressEngine(topics)
	engineAfter.NoteTopicIDs = noteTopicIDs
	compLeaves, totalLeaves, _ := engineAfter.SkillProgress()

	err = s.DB.WithTx(ctx, func(tx *sql.Tx) error {
		if _, err := s.DB.Exec(
			ctx, tx,
			`UPDATE topics SET is_completed = ?, completed_at = ?, updated_at = ? WHERE id = ?`,
			completed, s.DB.FormatNullTime(compAt), s.DB.FormatTime(now), s.DB.FormatUUID(topic.ID),
		); err != nil {
			return err
		}
		tid := topic.ID
		sid := topic.SkillID
		tname := topic.Name
		sname := skillName
		if completed {
			summary := fmt.Sprintf("You completed %q in %q", topic.Name, skillName)
			if err := services.LogActivity(ctx, s.DB, tx, user, models.ActionTopicCompleted, &tid, &sid, &tname, &sname, nil, &summary); err != nil {
				return err
			}
			if totalLeaves > 0 && compLeaves == totalLeaves {
				skillSummary := fmt.Sprintf("You completed the skill %q", skillName)
				if err := services.LogActivity(ctx, s.DB, tx, user, models.ActionSkillCompleted, nil, &sid, nil, &sname, nil, &skillSummary); err != nil {
					return err
				}
			}
		} else {
			summary := fmt.Sprintf("You unmarked %q in %q", topic.Name, skillName)
			if err := services.LogActivity(ctx, s.DB, tx, user, models.ActionTopicUncompleted, &tid, &sid, &tname, &sname, nil, &summary); err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		core.WriteError(w, http.StatusInternalServerError, "internal_error", "Something went wrong. Please try again.")
		return
	}

	node := engineAfter.BuildNode(engineAfter.Topics[topic.ID], nil)
	core.WriteJSON(w, http.StatusOK, node)
}
