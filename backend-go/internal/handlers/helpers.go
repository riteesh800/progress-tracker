package handlers

import (
	"context"
	"database/sql"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/google/uuid"
	"skill-progress-tracker/backend-go/internal/config"
	"skill-progress-tracker/backend-go/internal/core"
	"skill-progress-tracker/backend-go/internal/database"
	"skill-progress-tracker/backend-go/internal/models"
	"skill-progress-tracker/backend-go/internal/services"
)

type Server struct {
	Cfg     *config.Settings
	DB      *database.DB
	Limiter *core.RateLimiter
}

func NewServer(cfg *config.Settings, db *database.DB, limiter *core.RateLimiter) *Server {
	if limiter == nil {
		limiter = core.NewRateLimiter(cfg)
	}
	return &Server{
		Cfg:     cfg,
		DB:      db,
		Limiter: limiter,
	}
}

func DecodeBody[T any, PT interface {
	*T
	Validate() error
}](w http.ResponseWriter, r *http.Request) (*T, bool) {
	defer r.Body.Close()
	body, err := io.ReadAll(io.LimitReader(r.Body, 4*1024*1024))
	if err != nil {
		core.WriteValidationError(w)
		return nil, false
	}
	val, err := models.DecodeAndValidate[T, PT](body)
	if err != nil {
		core.WriteValidationError(w)
		return nil, false
	}
	return val, true
}

func ParsePathUUID(w http.ResponseWriter, raw string) (uuid.UUID, bool) {
	id, err := uuid.Parse(strings.TrimSpace(raw))
	if err != nil {
		core.WriteValidationError(w)
		return uuid.Nil, false
	}
	return id, true
}

const SkillColumns = `id, user_id, name, description, deleted_at, purge_at, created_at, updated_at`

func ScanSkill(scanner interface{ Scan(dest ...any) error }) (*models.Skill, error) {
	var rawID, rawUID, name string
	var desc, delAt, purgeAt, created, updated any
	if err := scanner.Scan(&rawID, &rawUID, &name, &desc, &delAt, &purgeAt, &created, &updated); err != nil {
		return nil, err
	}
	id, err := database.ParseUUID(rawID)
	if err != nil {
		return nil, err
	}
	uid, err := database.ParseUUID(rawUID)
	if err != nil {
		return nil, err
	}
	deletedAt, _ := database.ParseNullTime(delAt)
	purgeAtTime, _ := database.ParseNullTime(purgeAt)
	createdAt, _ := database.ParseTime(created)
	updatedAt, _ := database.ParseTime(updated)
	return &models.Skill{
		ID:          id,
		UserID:      uid,
		Name:        name,
		Description: database.ParseNullString(desc),
		DeletedAt:   deletedAt,
		PurgeAt:     purgeAtTime,
		CreatedAt:   createdAt,
		UpdatedAt:   updatedAt,
	}, nil
}

const TopicColumns = `id, skill_id, parent_id, name, description, order_index, is_completed, completed_at, created_at, updated_at`

func ScanTopic(scanner interface{ Scan(dest ...any) error }) (models.Topic, error) {
	var rawID, rawSID, name string
	var rawPID, desc, compAt, created, updated any
	var orderIdx int
	var isComp any
	if err := scanner.Scan(&rawID, &rawSID, &rawPID, &name, &desc, &orderIdx, &isComp, &compAt, &created, &updated); err != nil {
		return models.Topic{}, err
	}
	id, err := database.ParseUUID(rawID)
	if err != nil {
		return models.Topic{}, err
	}
	sid, err := database.ParseUUID(rawSID)
	if err != nil {
		return models.Topic{}, err
	}
	pid, _ := database.ParseNullUUID(rawPID)
	completedAt, _ := database.ParseNullTime(compAt)
	createdAt, _ := database.ParseTime(created)
	updatedAt, _ := database.ParseTime(updated)
	return models.Topic{
		ID:          id,
		SkillID:     sid,
		ParentID:    pid,
		Name:        name,
		Description: database.ParseNullString(desc),
		OrderIndex:  orderIdx,
		IsCompleted: database.ParseBool(isComp),
		CompletedAt: completedAt,
		CreatedAt:   createdAt,
		UpdatedAt:   updatedAt,
	}, nil
}

const NoteColumns = `id, topic_id, user_id, content, created_at, updated_at`

func ScanNote(scanner interface{ Scan(dest ...any) error }) (models.Note, error) {
	var rawID, rawTID, rawUID, content string
	var created, updated any
	if err := scanner.Scan(&rawID, &rawTID, &rawUID, &content, &created, &updated); err != nil {
		return models.Note{}, err
	}
	id, err := database.ParseUUID(rawID)
	if err != nil {
		return models.Note{}, err
	}
	tid, err := database.ParseUUID(rawTID)
	if err != nil {
		return models.Note{}, err
	}
	uid, err := database.ParseUUID(rawUID)
	if err != nil {
		return models.Note{}, err
	}
	createdAt, _ := database.ParseTime(created)
	updatedAt, _ := database.ParseTime(updated)
	return models.Note{
		ID:        id,
		TopicID:   tid,
		UserID:    uid,
		Content:   content,
		CreatedAt: createdAt,
		UpdatedAt: updatedAt,
	}, nil
}

const ImportJobColumns = `id, user_id, skill_id, original_filename, file_size_bytes, status, confidence_notes, error_message, generated_tree_json, created_at, updated_at`

func ScanImportJob(scanner interface{ Scan(dest ...any) error }) (*models.ImportJob, error) {
	var rawID, rawUID, origFilename, statusStr string
	var rawSID, confNotes, errMsg, genTree, created, updated any
	var fileSize int
	if err := scanner.Scan(&rawID, &rawUID, &rawSID, &origFilename, &fileSize, &statusStr, &confNotes, &errMsg, &genTree, &created, &updated); err != nil {
		return nil, err
	}
	id, err := database.ParseUUID(rawID)
	if err != nil {
		return nil, err
	}
	uid, err := database.ParseUUID(rawUID)
	if err != nil {
		return nil, err
	}
	sid, _ := database.ParseNullUUID(rawSID)
	createdAt, _ := database.ParseTime(created)
	updatedAt, _ := database.ParseTime(updated)
	return &models.ImportJob{
		ID:                id,
		UserID:            uid,
		SkillID:           sid,
		OriginalFilename:  origFilename,
		FileSizeBytes:     fileSize,
		Status:            models.ImportJobStatus(statusStr),
		ConfidenceNotes:   database.ParseJSONAny(confNotes),
		ErrorMessage:      database.ParseNullString(errMsg),
		GeneratedTreeJSON: database.ParseJSONMap(genTree),
		CreatedAt:         createdAt,
		UpdatedAt:         updatedAt,
	}, nil
}

func (s *Server) OwnedSkill(ctx context.Context, q database.Querier, skillID uuid.UUID, user *models.User, allowDeleted bool) (*models.Skill, *core.AppError) {
	query := `SELECT ` + SkillColumns + ` FROM skills WHERE id = ? AND user_id = ?`
	if !allowDeleted {
		query += ` AND deleted_at IS NULL`
	}
	row := s.DB.QueryRow(ctx, q, query, s.DB.FormatUUID(skillID), s.DB.FormatUUID(user.ID))
	skill, err := ScanSkill(row)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, core.NewAppError(http.StatusNotFound, "not_found", "Skill not found.")
		}
		return nil, core.NewAppError(http.StatusInternalServerError, "internal_error", "Something went wrong. Please try again.")
	}
	return skill, nil
}

func (s *Server) OwnedTopic(ctx context.Context, q database.Querier, topicID uuid.UUID, user *models.User, allowDeleted bool) (*models.Topic, *core.AppError) {
	query := `SELECT t.id, t.skill_id, t.parent_id, t.name, t.description, t.order_index, t.is_completed, t.completed_at, t.created_at, t.updated_at
		FROM topics t
		JOIN skills s ON s.id = t.skill_id
		WHERE t.id = ? AND s.user_id = ?`
	if !allowDeleted {
		query += ` AND s.deleted_at IS NULL`
	}
	row := s.DB.QueryRow(ctx, q, query, s.DB.FormatUUID(topicID), s.DB.FormatUUID(user.ID))
	topic, err := ScanTopic(row)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, core.NewAppError(http.StatusNotFound, "not_found", "Topic not found.")
		}
		return nil, core.NewAppError(http.StatusInternalServerError, "internal_error", "Something went wrong. Please try again.")
	}
	return &topic, nil
}

func (s *Server) LoadSkillTopics(ctx context.Context, q database.Querier, skillID uuid.UUID) ([]models.Topic, error) {
	rows, err := s.DB.Query(ctx, q, `SELECT `+TopicColumns+` FROM topics WHERE skill_id = ?`, s.DB.FormatUUID(skillID))
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var list []models.Topic
	for rows.Next() {
		t, err := ScanTopic(rows)
		if err != nil {
			return nil, err
		}
		list = append(list, t)
	}
	return list, rows.Err()
}

func (s *Server) LoadSkillsTopicsMap(ctx context.Context, q database.Querier, skillIDs []uuid.UUID) (map[uuid.UUID][]models.Topic, error) {
	result := make(map[uuid.UUID][]models.Topic, len(skillIDs))
	if len(skillIDs) == 0 {
		return result, nil
	}
	placeholders := make([]string, len(skillIDs))
	args := make([]any, len(skillIDs))
	for i, id := range skillIDs {
		placeholders[i] = "?"
		args[i] = s.DB.FormatUUID(id)
	}
	query := `SELECT ` + TopicColumns + ` FROM topics WHERE skill_id IN (` + strings.Join(placeholders, ",") + `)`
	rows, err := s.DB.Query(ctx, q, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		t, err := ScanTopic(rows)
		if err != nil {
			return nil, err
		}
		result[t.SkillID] = append(result[t.SkillID], t)
	}
	return result, rows.Err()
}

func SerializeSkill(skill *models.Skill, topics []models.Topic) models.SkillOut {
	comp, total, pct, empty := services.ComputeSkillProgress(topics)
	return models.SkillOut{
		ID:              skill.ID,
		Name:            skill.Name,
		Description:     skill.Description,
		CreatedAt:       skill.CreatedAt,
		CompletedLeaves: comp,
		TotalLeaves:     total,
		Percent:         pct,
		Empty:           empty,
		DeletedAt:       skill.DeletedAt,
		PurgeAt:         skill.PurgeAt,
	}
}

func MarshalJSONString(v any) any {
	if v == nil {
		return nil
	}
	b, err := json.Marshal(v)
	if err != nil {
		return nil
	}
	return string(b)
}

const GoalColumns = `id, user_id, goal_name, deadline_at, timezone, motivation_line, deleted_at, purge_at, created_at, updated_at`

func ScanGoal(scanner interface{ Scan(dest ...any) error }) (*models.Goal, error) {
	var rawID, rawUID, name, tz string
	var deadline, motiv, delAt, purgeAt, created, updated any
	if err := scanner.Scan(&rawID, &rawUID, &name, &deadline, &tz, &motiv, &delAt, &purgeAt, &created, &updated); err != nil {
		return nil, err
	}
	id, err := database.ParseUUID(rawID)
	if err != nil {
		return nil, err
	}
	uid, err := database.ParseUUID(rawUID)
	if err != nil {
		return nil, err
	}
	deadlineAt, err := database.ParseTime(deadline)
	if err != nil {
		return nil, err
	}
	deletedAt, _ := database.ParseNullTime(delAt)
	purgeAtTime, _ := database.ParseNullTime(purgeAt)
	createdAt, _ := database.ParseTime(created)
	updatedAt, _ := database.ParseTime(updated)
	return &models.Goal{
		ID:             id,
		UserID:         uid,
		GoalName:       name,
		DeadlineAt:     deadlineAt,
		Timezone:       tz,
		MotivationLine: database.ParseNullString(motiv),
		DeletedAt:      deletedAt,
		PurgeAt:        purgeAtTime,
		CreatedAt:      createdAt,
		UpdatedAt:      updatedAt,
	}, nil
}

const GoalActivityColumns = `id, goal_id, user_id, activity_text, activity_date, created_at`

func ScanGoalActivity(scanner interface{ Scan(dest ...any) error }) (*models.GoalActivity, error) {
	var rawID, rawGID, rawUID, text, date string
	var created any
	if err := scanner.Scan(&rawID, &rawGID, &rawUID, &text, &date, &created); err != nil {
		return nil, err
	}
	id, err := database.ParseUUID(rawID)
	if err != nil {
		return nil, err
	}
	gid, err := database.ParseUUID(rawGID)
	if err != nil {
		return nil, err
	}
	uid, err := database.ParseUUID(rawUID)
	if err != nil {
		return nil, err
	}
	if len(date) > 10 {
		date = date[:10]
	}
	createdAt, _ := database.ParseTime(created)
	return &models.GoalActivity{
		ID:           id,
		GoalID:       gid,
		UserID:       uid,
		ActivityText: text,
		ActivityDate: date,
		CreatedAt:    createdAt,
	}, nil
}

func (s *Server) OwnedGoal(ctx context.Context, q database.Querier, goalID uuid.UUID, user *models.User, allowDeleted bool) (*models.Goal, *core.AppError) {
	query := `SELECT ` + GoalColumns + ` FROM goals WHERE id = ? AND user_id = ?`
	if !allowDeleted {
		query += ` AND deleted_at IS NULL`
	}
	row := s.DB.QueryRow(ctx, q, query, s.DB.FormatUUID(goalID), s.DB.FormatUUID(user.ID))
	goal, err := ScanGoal(row)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, core.NewAppError(http.StatusNotFound, "not_found", "Goal not found.")
		}
		return nil, core.NewAppError(http.StatusInternalServerError, "internal_error", "Something went wrong. Please try again.")
	}
	return goal, nil
}

func (s *Server) PurgeGoalRecord(ctx context.Context, q database.Querier, goalID uuid.UUID) error {
	if _, err := s.DB.Exec(ctx, q, `DELETE FROM goal_activities WHERE goal_id = ?`, s.DB.FormatUUID(goalID)); err != nil {
		return err
	}
	_, err := s.DB.Exec(ctx, q, `DELETE FROM goals WHERE id = ?`, s.DB.FormatUUID(goalID))
	return err
}

func SerializeGoal(goal *models.Goal, totalActivities int) models.GoalOut {
	now := time.Now().UTC()
	isExpired := !now.Before(goal.DeadlineAt)
	return models.GoalOut{
		ID:              goal.ID,
		GoalName:        goal.GoalName,
		DeadlineAt:      goal.DeadlineAt,
		Timezone:        goal.Timezone,
		MotivationLine:  goal.MotivationLine,
		DeletedAt:       goal.DeletedAt,
		PurgeAt:         goal.PurgeAt,
		CreatedAt:       goal.CreatedAt,
		UpdatedAt:       goal.UpdatedAt,
		IsExpired:       isExpired,
		TotalActivities: totalActivities,
	}
}
