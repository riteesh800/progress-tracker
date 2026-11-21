package handlers

import (
	"context"
	"database/sql"
	"net/http"
	"sort"
	"strings"
	"time"

	"github.com/google/uuid"
	"skill-progress-tracker/backend-go/internal/core"
	"skill-progress-tracker/backend-go/internal/database"
	"skill-progress-tracker/backend-go/internal/models"
)

func (s *Server) loadPersonalNoteTasks(ctx context.Context, noteIDs []uuid.UUID) (map[uuid.UUID][]models.PersonalNoteTaskOut, error) {
	result := make(map[uuid.UUID][]models.PersonalNoteTaskOut, len(noteIDs))
	if len(noteIDs) == 0 {
		return result, nil
	}
	placeholders := make([]string, len(noteIDs))
	args := make([]any, len(noteIDs))
	for i, id := range noteIDs {
		placeholders[i] = "?"
		args[i] = s.DB.FormatUUID(id)
	}
	query := `SELECT id, note_id, content, is_completed, order_index FROM personal_note_tasks WHERE note_id IN (` + strings.Join(placeholders, ",") + `) ORDER BY order_index ASC`
	rows, err := s.DB.Query(ctx, nil, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var rawID, rawNID, content string
		var isComp any
		var orderIdx int
		if err := rows.Scan(&rawID, &rawNID, &content, &isComp, &orderIdx); err != nil {
			return nil, err
		}
		id, err := database.ParseUUID(rawID)
		if err != nil {
			return nil, err
		}
		nid, err := database.ParseUUID(rawNID)
		if err != nil {
			return nil, err
		}
		result[nid] = append(result[nid], models.PersonalNoteTaskOut{
			ID:          id,
			Content:     content,
			IsCompleted: database.ParseBool(isComp),
			OrderIndex:  orderIdx,
		})
	}
	for nid := range result {
		tasks := result[nid]
		sort.SliceStable(tasks, func(i, j int) bool {
			return tasks[i].OrderIndex < tasks[j].OrderIndex
		})
		result[nid] = tasks
	}
	return result, rows.Err()
}

func (s *Server) ListPersonalNotes(w http.ResponseWriter, r *http.Request, user *models.User) {
	ctx := r.Context()
	rows, err := s.DB.Query(
		ctx, nil,
		`SELECT id, user_id, title, created_at, updated_at FROM personal_notes WHERE user_id = ? ORDER BY updated_at DESC`,
		s.DB.FormatUUID(user.ID),
	)
	if err != nil {
		core.WriteError(w, http.StatusInternalServerError, "internal_error", "Something went wrong. Please try again.")
		return
	}
	defer rows.Close()

	var notes []models.PersonalNoteOut
	var noteIDs []uuid.UUID
	for rows.Next() {
		var rawID, rawUID, title string
		var created, updated any
		if err := rows.Scan(&rawID, &rawUID, &title, &created, &updated); err != nil {
			core.WriteError(w, http.StatusInternalServerError, "internal_error", "Something went wrong. Please try again.")
			return
		}
		id, err := database.ParseUUID(rawID)
		if err != nil {
			core.WriteError(w, http.StatusInternalServerError, "internal_error", "Something went wrong. Please try again.")
			return
		}
		createdAt, _ := database.ParseTime(created)
		updatedAt, _ := database.ParseTime(updated)
		notes = append(notes, models.PersonalNoteOut{
			ID:        id,
			Title:     title,
			CreatedAt: createdAt,
			UpdatedAt: updatedAt,
			Tasks:     []models.PersonalNoteTaskOut{},
		})
		noteIDs = append(noteIDs, id)
	}
	rows.Close()

	tasksMap, err := s.loadPersonalNoteTasks(ctx, noteIDs)
	if err != nil {
		core.WriteError(w, http.StatusInternalServerError, "internal_error", "Something went wrong. Please try again.")
		return
	}
	for i := range notes {
		if tList, ok := tasksMap[notes[i].ID]; ok && tList != nil {
			notes[i].Tasks = tList
		}
	}
	if notes == nil {
		notes = []models.PersonalNoteOut{}
	}
	core.WriteJSON(w, http.StatusOK, notes)
}

func (s *Server) GetPersonalNote(w http.ResponseWriter, r *http.Request, user *models.User) {
	noteID, ok := ParsePathUUID(w, r.PathValue("note_id"))
	if !ok {
		return
	}
	ctx := r.Context()
	var rawID, title string
	var createdVal, updatedVal any
	err := s.DB.QueryRow(
		ctx, nil,
		`SELECT id, title, created_at, updated_at FROM personal_notes WHERE id = ? AND user_id = ?`,
		s.DB.FormatUUID(noteID), s.DB.FormatUUID(user.ID),
	).Scan(&rawID, &title, &createdVal, &updatedVal)
	if err != nil {
		if err == sql.ErrNoRows {
			core.WriteError(w, http.StatusNotFound, "not_found", "Note not found.")
			return
		}
		core.WriteError(w, http.StatusInternalServerError, "internal_error", "Something went wrong. Please try again.")
		return
	}
	createdAt, _ := database.ParseTime(createdVal)
	updatedAt, _ := database.ParseTime(updatedVal)
	tasksMap, err := s.loadPersonalNoteTasks(ctx, []uuid.UUID{noteID})
	if err != nil {
		core.WriteError(w, http.StatusInternalServerError, "internal_error", "Something went wrong. Please try again.")
		return
	}
	tasks := tasksMap[noteID]
	if tasks == nil {
		tasks = []models.PersonalNoteTaskOut{}
	}
	core.WriteJSON(w, http.StatusOK, models.PersonalNoteOut{
		ID:        noteID,
		Title:     title,
		CreatedAt: createdAt,
		UpdatedAt: updatedAt,
		Tasks:     tasks,
	})
}

func (s *Server) CreatePersonalNote(w http.ResponseWriter, r *http.Request, user *models.User) {
	body, ok := DecodeBody[models.PersonalNoteIn](w, r)
	if !ok {
		return
	}
	title := "Untitled note"
	if body.Title != nil && strings.TrimSpace(*body.Title) != "" {
		title = strings.TrimSpace(*body.Title)
	}
	if runes := []rune(title); len(runes) > 35 {
		title = string(runes[:35])
	}
	now := time.Now().UTC()
	id := uuid.New()
	taskID := uuid.New()
	ctx := r.Context()

	err := s.DB.WithTx(ctx, func(tx *sql.Tx) error {
		if _, err := s.DB.Exec(
			ctx, tx,
			`INSERT INTO personal_notes (id, user_id, title, created_at, updated_at) VALUES (?, ?, ?, ?, ?)`,
			s.DB.FormatUUID(id), s.DB.FormatUUID(user.ID), title, s.DB.FormatTime(now), s.DB.FormatTime(now),
		); err != nil {
			return err
		}
		_, err := s.DB.Exec(
			ctx, tx,
			`INSERT INTO personal_note_tasks (id, note_id, content, is_completed, order_index, created_at, updated_at)
			 VALUES (?, ?, ?, ?, ?, ?, ?)`,
			s.DB.FormatUUID(taskID), s.DB.FormatUUID(id), "", false, 0, s.DB.FormatTime(now), s.DB.FormatTime(now),
		)
		return err
	})
	if err != nil {
		core.WriteError(w, http.StatusInternalServerError, "internal_error", "Something went wrong. Please try again.")
		return
	}
	core.WriteJSON(w, http.StatusCreated, models.PersonalNoteOut{
		ID:        id,
		Title:     title,
		CreatedAt: now,
		UpdatedAt: now,
		Tasks: []models.PersonalNoteTaskOut{
			{
				ID:          taskID,
				Content:     "",
				IsCompleted: false,
				OrderIndex:  0,
			},
		},
	})
}

func (s *Server) UpdatePersonalNote(w http.ResponseWriter, r *http.Request, user *models.User) {
	noteID, ok := ParsePathUUID(w, r.PathValue("note_id"))
	if !ok {
		return
	}
	body, ok := DecodeBody[models.PersonalNoteUpdateIn](w, r)
	if !ok {
		return
	}
	ctx := r.Context()

	var rawID, title string
	var createdVal, updatedVal any
	err := s.DB.QueryRow(
		ctx, nil,
		`SELECT id, title, created_at, updated_at FROM personal_notes WHERE id = ? AND user_id = ?`,
		s.DB.FormatUUID(noteID), s.DB.FormatUUID(user.ID),
	).Scan(&rawID, &title, &createdVal, &updatedVal)
	if err != nil {
		if err == sql.ErrNoRows {
			core.WriteError(w, http.StatusNotFound, "not_found", "Note not found.")
			return
		}
		core.WriteError(w, http.StatusInternalServerError, "internal_error", "Something went wrong. Please try again.")
		return
	}
	createdAt, _ := database.ParseTime(createdVal)

	if body.Title != nil {
		t := strings.TrimSpace(*body.Title)
		if t == "" {
			t = "Untitled note"
		}
		if runes := []rune(t); len(runes) > 35 {
			t = string(runes[:35])
		}
		title = t
	}
	now := time.Now().UTC()
	var tasksOut []models.PersonalNoteTaskOut

	err = s.DB.WithTx(ctx, func(tx *sql.Tx) error {
		if _, err := s.DB.Exec(
			ctx, tx,
			`UPDATE personal_notes SET title = ?, updated_at = ? WHERE id = ?`,
			title, s.DB.FormatTime(now), s.DB.FormatUUID(noteID),
		); err != nil {
			return err
		}
		if body.Tasks != nil {
			if _, err := s.DB.Exec(ctx, tx, `DELETE FROM personal_note_tasks WHERE note_id = ?`, s.DB.FormatUUID(noteID)); err != nil {
				return err
			}
			tasksOut = make([]models.PersonalNoteTaskOut, 0, len(*body.Tasks))
			for idx, taskIn := range *body.Tasks {
				tid := uuid.New()
				if _, err := s.DB.Exec(
					ctx, tx,
					`INSERT INTO personal_note_tasks (id, note_id, content, is_completed, order_index, created_at, updated_at)
					 VALUES (?, ?, ?, ?, ?, ?, ?)`,
					s.DB.FormatUUID(tid),
					s.DB.FormatUUID(noteID),
					taskIn.Content,
					taskIn.IsCompleted,
					idx,
					s.DB.FormatTime(now),
					s.DB.FormatTime(now),
				); err != nil {
					return err
				}
				tasksOut = append(tasksOut, models.PersonalNoteTaskOut{
					ID:          tid,
					Content:     taskIn.Content,
					IsCompleted: taskIn.IsCompleted,
					OrderIndex:  idx,
				})
			}
		}
		return nil
	})
	if err != nil {
		core.WriteError(w, http.StatusInternalServerError, "internal_error", "Something went wrong. Please try again.")
		return
	}
	if body.Tasks == nil {
		tMap, err := s.loadPersonalNoteTasks(ctx, []uuid.UUID{noteID})
		if err == nil {
			tasksOut = tMap[noteID]
		}
	}
	if tasksOut == nil {
		tasksOut = []models.PersonalNoteTaskOut{}
	}
	core.WriteJSON(w, http.StatusOK, models.PersonalNoteOut{
		ID:        noteID,
		Title:     title,
		CreatedAt: createdAt,
		UpdatedAt: now,
		Tasks:     tasksOut,
	})
}

func (s *Server) DeletePersonalNote(w http.ResponseWriter, r *http.Request, user *models.User) {
	noteID, ok := ParsePathUUID(w, r.PathValue("note_id"))
	if !ok {
		return
	}
	ctx := r.Context()
	var rawID string
	err := s.DB.QueryRow(
		ctx, nil,
		`SELECT id FROM personal_notes WHERE id = ? AND user_id = ?`,
		s.DB.FormatUUID(noteID), s.DB.FormatUUID(user.ID),
	).Scan(&rawID)
	if err != nil {
		if err == sql.ErrNoRows {
			core.WriteError(w, http.StatusNotFound, "not_found", "Note not found.")
			return
		}
		core.WriteError(w, http.StatusInternalServerError, "internal_error", "Something went wrong. Please try again.")
		return
	}
	err = s.DB.WithTx(ctx, func(tx *sql.Tx) error {
		_, _ = s.DB.Exec(ctx, tx, `DELETE FROM personal_note_tasks WHERE note_id = ?`, s.DB.FormatUUID(noteID))
		_, err := s.DB.Exec(ctx, tx, `DELETE FROM personal_notes WHERE id = ? AND user_id = ?`, s.DB.FormatUUID(noteID), s.DB.FormatUUID(user.ID))
		return err
	})
	if err != nil {
		core.WriteError(w, http.StatusInternalServerError, "internal_error", "Something went wrong. Please try again.")
		return
	}
	core.WriteJSON(w, http.StatusOK, map[string]any{"ok": true})
}
