package handlers

import (
	"net/http"
	"strings"
	"unicode/utf8"

	"skill-progress-tracker/backend-go/internal/core"
	"skill-progress-tracker/backend-go/internal/database"
	"skill-progress-tracker/backend-go/internal/models"
)

func escapeLike(q string) string {
	r := strings.ReplaceAll(q, `\`, `\\`)
	r = strings.ReplaceAll(r, `%`, `\%`)
	r = strings.ReplaceAll(r, `_`, `\_`)
	return "%" + r + "%"
}

func truncateSnippet(s string, maxRunes int) string {
	runes := []rune(s)
	if len(runes) <= maxRunes {
		return s
	}
	return string(runes[:maxRunes])
}

func (s *Server) Search(w http.ResponseWriter, r *http.Request, user *models.User) {
	rawQ := r.URL.Query().Get("q")
	qLen := utf8.RuneCountInString(rawQ)
	if qLen < 1 || qLen > 200 {
		core.WriteValidationError(w)
		return
	}
	qClean := strings.TrimSpace(rawQ)
	if qClean == "" {
		core.WriteJSON(w, http.StatusOK, []models.SearchHit{})
		return
	}

	ctx := r.Context()
	pattern := escapeLike(qClean)
	patternLower := strings.ToLower(pattern)
	hits := make([]models.SearchHit, 0)

	// 1. Skills search
	var sQuery string
	var sArgs []any
	if s.DB.IsPostgres() {
		sQuery = `SELECT s.id, s.name, s.description FROM skills s
			WHERE s.user_id = ? AND s.deleted_at IS NULL
			  AND (to_tsvector('english', coalesce(s.name,'') || ' ' || coalesce(s.description,'')) @@ plainto_tsquery('english', ?)
			       OR s.name ILIKE ? ESCAPE '\'
			       OR coalesce(s.description,'') ILIKE ? ESCAPE '\')
			LIMIT 50`
		sArgs = []any{s.DB.FormatUUID(user.ID), qClean, pattern, pattern}
	} else {
		sQuery = `SELECT id, name, description FROM skills
			WHERE user_id = ? AND deleted_at IS NULL
			  AND (LOWER(name) LIKE ? ESCAPE '\' OR LOWER(coalesce(description,'')) LIKE ? ESCAPE '\')
			LIMIT 50`
		sArgs = []any{s.DB.FormatUUID(user.ID), patternLower, patternLower}
	}
	if sRows, err := s.DB.Query(ctx, nil, sQuery, sArgs...); err == nil {
		for sRows.Next() {
			var rawID, name string
			var descVal any
			if err := sRows.Scan(&rawID, &name, &descVal); err == nil {
				if id, err := database.ParseUUID(rawID); err == nil {
					desc := ""
					if ds := database.ParseNullString(descVal); ds != nil {
						desc = *ds
					}
					sid := id
					hits = append(hits, models.SearchHit{
						Kind:    "skill",
						ID:      id,
						Title:   name,
						Snippet: truncateSnippet(desc, 180),
						SkillID: &sid,
						TopicID: nil,
					})
				}
			}
		}
		sRows.Close()
	}

	// 2. Topics search
	var tQuery string
	var tArgs []any
	if s.DB.IsPostgres() {
		tQuery = `SELECT t.id, t.name, t.description, t.skill_id FROM topics t
			JOIN skills s ON s.id = t.skill_id
			WHERE s.user_id = ? AND s.deleted_at IS NULL
			  AND (to_tsvector('english', coalesce(t.name,'') || ' ' || coalesce(t.description,'')) @@ plainto_tsquery('english', ?)
			       OR t.name ILIKE ? ESCAPE '\'
			       OR coalesce(t.description,'') ILIKE ? ESCAPE '\')
			LIMIT 50`
		tArgs = []any{s.DB.FormatUUID(user.ID), qClean, pattern, pattern}
	} else {
		tQuery = `SELECT t.id, t.name, t.description, t.skill_id FROM topics t
			JOIN skills s ON s.id = t.skill_id
			WHERE s.user_id = ? AND s.deleted_at IS NULL
			  AND (LOWER(t.name) LIKE ? ESCAPE '\' OR LOWER(coalesce(t.description,'')) LIKE ? ESCAPE '\')
			LIMIT 50`
		tArgs = []any{s.DB.FormatUUID(user.ID), patternLower, patternLower}
	}
	if tRows, err := s.DB.Query(ctx, nil, tQuery, tArgs...); err == nil {
		for tRows.Next() {
			var rawID, name, rawSID string
			var descVal any
			if err := tRows.Scan(&rawID, &name, &descVal, &rawSID); err == nil {
				id, err1 := database.ParseUUID(rawID)
				sid, err2 := database.ParseUUID(rawSID)
				if err1 == nil && err2 == nil {
					desc := ""
					if ds := database.ParseNullString(descVal); ds != nil {
						desc = *ds
					}
					tid := id
					hits = append(hits, models.SearchHit{
						Kind:    "topic",
						ID:      id,
						Title:   name,
						Snippet: truncateSnippet(desc, 180),
						SkillID: &sid,
						TopicID: &tid,
					})
				}
			}
		}
		tRows.Close()
	}

	// 3. Notes search
	var nQuery string
	var nArgs []any
	if s.DB.IsPostgres() {
		nQuery = `SELECT n.id, n.content, n.topic_id, t.skill_id, t.name FROM notes n
			JOIN topics t ON t.id = n.topic_id
			JOIN skills s ON s.id = t.skill_id
			WHERE n.user_id = ? AND s.deleted_at IS NULL
			  AND (to_tsvector('english', coalesce(n.content,'')) @@ plainto_tsquery('english', ?)
			       OR n.content ILIKE ? ESCAPE '\')
			LIMIT 50`
		nArgs = []any{s.DB.FormatUUID(user.ID), qClean, pattern}
	} else {
		nQuery = `SELECT n.id, n.content, n.topic_id, t.skill_id, t.name FROM notes n
			JOIN topics t ON t.id = n.topic_id
			JOIN skills s ON s.id = t.skill_id
			WHERE n.user_id = ? AND s.deleted_at IS NULL
			  AND LOWER(n.content) LIKE ? ESCAPE '\'
			LIMIT 50`
		nArgs = []any{s.DB.FormatUUID(user.ID), patternLower}
	}
	if nRows, err := s.DB.Query(ctx, nil, nQuery, nArgs...); err == nil {
		for nRows.Next() {
			var rawNID, content, rawTID, rawSID, tName string
			if err := nRows.Scan(&rawNID, &content, &rawTID, &rawSID, &tName); err == nil {
				nid, err1 := database.ParseUUID(rawNID)
				tid, err2 := database.ParseUUID(rawTID)
				sid, err3 := database.ParseUUID(rawSID)
				if err1 == nil && err2 == nil && err3 == nil {
					hits = append(hits, models.SearchHit{
						Kind:    "note",
						ID:      nid,
						Title:   tName,
						Snippet: truncateSnippet(content, 180),
						SkillID: &sid,
						TopicID: &tid,
					})
				}
			}
		}
		nRows.Close()
	}

	core.WriteJSON(w, http.StatusOK, hits)
}
