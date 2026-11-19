package handlers

import (
	"context"
	"database/sql"
	"net/http"
	"strings"
	"time"

	"github.com/google/uuid"
	"skill-progress-tracker/backend-go/internal/core"
	"skill-progress-tracker/backend-go/internal/models"
	"skill-progress-tracker/backend-go/internal/services"
)

func serializeNote(n models.Note) models.NoteOut {
	return models.NoteOut{
		ID:        n.ID,
		TopicID:   n.TopicID,
		Content:   n.Content,
		CreatedAt: n.CreatedAt,
		UpdatedAt: n.UpdatedAt,
	}
}

func (s *Server) ListNotes(w http.ResponseWriter, r *http.Request, user *models.User) {
	rawTopicID := strings.TrimSpace(r.URL.Query().Get("topic_id"))
	topicID, err := uuid.Parse(rawTopicID)
	if err != nil {
		core.WriteValidationError(w)
		return
	}
	ctx := r.Context()
	if _, appErr := s.OwnedTopic(ctx, nil, topicID, user, false); appErr != nil {
		core.WriteAppError(w, appErr)
		return
	}
	rows, err := s.DB.Query(
		ctx, nil,
		`SELECT `+NoteColumns+` FROM notes WHERE topic_id = ? AND user_id = ? ORDER BY created_at DESC`,
		s.DB.FormatUUID(topicID), s.DB.FormatUUID(user.ID),
	)
	if err != nil {
		core.WriteError(w, http.StatusInternalServerError, "internal_error", "Something went wrong. Please try again.")
		return
	}
	defer rows.Close()

	out := make([]models.NoteOut, 0)
	for rows.Next() {
		n, err := ScanNote(rows)
		if err != nil {
			core.WriteError(w, http.StatusInternalServerError, "internal_error", "Something went wrong. Please try again.")
			return
		}
		out = append(out, serializeNote(n))
	}
	core.WriteJSON(w, http.StatusOK, out)
}

func (s *Server) GetNote(w http.ResponseWriter, r *http.Request, user *models.User) {
	noteID, ok := ParsePathUUID(w, r.PathValue("note_id"))
	if !ok {
		return
	}
	row := s.DB.QueryRow(
		r.Context(), nil,
		`SELECT `+NoteColumns+` FROM notes WHERE id = ? AND user_id = ?`,
		s.DB.FormatUUID(noteID), s.DB.FormatUUID(user.ID),
	)
	note, err := ScanNote(row)
	if err != nil {
		if err == sql.ErrNoRows {
			core.WriteError(w, http.StatusNotFound, "not_found", "Note not found.")
			return
		}
		core.WriteError(w, http.StatusInternalServerError, "internal_error", "Something went wrong. Please try again.")
		return
	}
	core.WriteJSON(w, http.StatusOK, serializeNote(note))
}

func (s *Server) CreateNote(w http.ResponseWriter, r *http.Request, user *models.User) {
	body, ok := DecodeBody[models.NoteIn](w, r)
	if !ok {
		return
	}
	ctx := r.Context()
	topic, appErr := s.OwnedTopic(ctx, nil, body.ParsedTopicID, user, false)
	if appErr != nil {
		core.WriteAppError(w, appErr)
		return
	}
	trimmed := strings.TrimSpace(body.Content)
	if trimmed == "" {
		core.WriteError(w, http.StatusUnprocessableEntity, "empty_note", "Note content cannot be empty.")
		return
	}

	var skillName *string
	var sn string
	if err := s.DB.QueryRow(ctx, nil, `SELECT name FROM skills WHERE id = ?`, s.DB.FormatUUID(topic.SkillID)).Scan(&sn); err == nil {
		skillName = &sn
	}

	now := time.Now().UTC()
	note := models.Note{
		ID:        uuid.New(),
		TopicID:   topic.ID,
		UserID:    user.ID,
		Content:   trimmed,
		CreatedAt: now,
		UpdatedAt: now,
	}
	tid := topic.ID
	sid := topic.SkillID
	tname := topic.Name

	err := s.DB.WithTx(ctx, func(tx *sql.Tx) error {
		if _, err := s.DB.Exec(
			ctx, tx,
			`INSERT INTO notes (id, topic_id, user_id, content, created_at, updated_at)
			 VALUES (?, ?, ?, ?, ?, ?)`,
			s.DB.FormatUUID(note.ID),
			s.DB.FormatUUID(note.TopicID),
			s.DB.FormatUUID(note.UserID),
			note.Content,
			s.DB.FormatTime(now),
			s.DB.FormatTime(now),
		); err != nil {
			return err
		}
		return services.LogActivity(ctx, s.DB, tx, user, models.ActionNoteAdded, &tid, &sid, &tname, skillName, nil, nil)
	})
	if err != nil {
		core.WriteError(w, http.StatusInternalServerError, "internal_error", "Something went wrong. Please try again.")
		return
	}
	core.WriteJSON(w, http.StatusCreated, serializeNote(note))
}

func (s *Server) loadNoteByID(ctx context.Context, noteID uuid.UUID, user *models.User) (*models.Note, error) {
	row := s.DB.QueryRow(
		ctx, nil,
		`SELECT `+NoteColumns+` FROM notes WHERE id = ? AND user_id = ?`,
		s.DB.FormatUUID(noteID), s.DB.FormatUUID(user.ID),
	)
	n, err := ScanNote(row)
	if err != nil {
		return nil, err
	}
	return &n, nil
}

func (s *Server) UpdateNote(w http.ResponseWriter, r *http.Request, user *models.User) {
	noteID, ok := ParsePathUUID(w, r.PathValue("note_id"))
	if !ok {
		return
	}
	body, ok := DecodeBody[models.NoteUpdateIn](w, r)
	if !ok {
		return
	}
	ctx := r.Context()
	note, err := s.loadNoteByID(ctx, noteID, user)
	if err != nil {
		if err == sql.ErrNoRows {
			core.WriteError(w, http.StatusNotFound, "not_found", "Note not found.")
			return
		}
		core.WriteError(w, http.StatusInternalServerError, "internal_error", "Something went wrong. Please try again.")
		return
	}
	topic, appErr := s.OwnedTopic(ctx, nil, note.TopicID, user, false)
	if appErr != nil {
		core.WriteAppError(w, appErr)
		return
	}

	var skillName *string
	var sn string
	if err := s.DB.QueryRow(ctx, nil, `SELECT name FROM skills WHERE id = ?`, s.DB.FormatUUID(topic.SkillID)).Scan(&sn); err == nil {
		skillName = &sn
	}

	now := time.Now().UTC()
	note.Content = strings.TrimSpace(body.Content)
	note.UpdatedAt = now
	tid := topic.ID
	sid := topic.SkillID
	tname := topic.Name

	err = s.DB.WithTx(ctx, func(tx *sql.Tx) error {
		if _, err := s.DB.Exec(
			ctx, tx,
			`UPDATE notes SET content = ?, updated_at = ? WHERE id = ?`,
			note.Content, s.DB.FormatTime(now), s.DB.FormatUUID(note.ID),
		); err != nil {
			return err
		}
		return services.LogActivity(ctx, s.DB, tx, user, models.ActionNoteUpdated, &tid, &sid, &tname, skillName, nil, nil)
	})
	if err != nil {
		core.WriteError(w, http.StatusInternalServerError, "internal_error", "Something went wrong. Please try again.")
		return
	}
	core.WriteJSON(w, http.StatusOK, serializeNote(*note))
}

func (s *Server) DeleteNote(w http.ResponseWriter, r *http.Request, user *models.User) {
	noteID, ok := ParsePathUUID(w, r.PathValue("note_id"))
	if !ok {
		return
	}
	ctx := r.Context()
	note, err := s.loadNoteByID(ctx, noteID, user)
	if err != nil {
		if err == sql.ErrNoRows {
			core.WriteError(w, http.StatusNotFound, "not_found", "Note not found.")
			return
		}
		core.WriteError(w, http.StatusInternalServerError, "internal_error", "Something went wrong. Please try again.")
		return
	}
	_, err = s.DB.Exec(ctx, nil, `DELETE FROM notes WHERE id = ?`, s.DB.FormatUUID(note.ID))
	if err != nil {
		core.WriteError(w, http.StatusInternalServerError, "internal_error", "Something went wrong. Please try again.")
		return
	}
	core.WriteJSON(w, http.StatusOK, map[string]any{"ok": true})
}
