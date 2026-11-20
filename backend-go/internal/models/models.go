package models

import (
	"time"

	"github.com/google/uuid"
)

type ActionType string

const (
	ActionTopicCompleted   ActionType = "topic_completed"
	ActionTopicUncompleted ActionType = "topic_uncompleted"
	ActionNoteAdded        ActionType = "note_added"
	ActionNoteUpdated      ActionType = "note_updated"
	ActionTopicCreated     ActionType = "topic_created"
	ActionSkillCreated     ActionType = "skill_created"
	ActionSkillCompleted   ActionType = "skill_completed"
	ActionSkillDeleted     ActionType = "skill_deleted"
	ActionPDFImported      ActionType = "pdf_imported"
	ActionAIGenerated      ActionType = "ai_generated"
)

type ImportJobStatus string

const (
	StatusUploaded        ImportJobStatus = "uploaded"
	StatusExtracting      ImportJobStatus = "extracting"
	StatusParsing         ImportJobStatus = "parsing"
	StatusReadyForPreview ImportJobStatus = "ready_for_preview"
	StatusConfirmed       ImportJobStatus = "confirmed"
	StatusFailed          ImportJobStatus = "failed"
)

type User struct {
	ID                  uuid.UUID
	Email               string
	PasswordHash        *string
	GoogleID            *string
	Name                string
	Timezone            string
	IsActive            bool
	MustChangePassword  bool
	DeletionRequestedAt *time.Time
	DeletionDeadline    *time.Time
	LastActiveAt        *time.Time
	CreatedAt           time.Time
	UpdatedAt           time.Time
}

type Skill struct {
	ID          uuid.UUID
	UserID      uuid.UUID
	Name        string
	Description *string
	DeletedAt   *time.Time
	PurgeAt     *time.Time
	CreatedAt   time.Time
	UpdatedAt   time.Time
}

type Topic struct {
	ID          uuid.UUID
	SkillID     uuid.UUID
	ParentID    *uuid.UUID
	Name        string
	Description *string
	OrderIndex  int
	IsCompleted bool
	CompletedAt *time.Time
	CreatedAt   time.Time
	UpdatedAt   time.Time
}

type Note struct {
	ID        uuid.UUID
	TopicID   uuid.UUID
	UserID    uuid.UUID
	Content   string
	CreatedAt time.Time
	UpdatedAt time.Time
}

type ActivityLog struct {
	ID                 uuid.UUID
	UserID             uuid.UUID
	ActionType         ActionType
	TopicID            *uuid.UUID
	SkillID            *uuid.UUID
	TopicNameSnapshot  *string
	SkillNameSnapshot  *string
	ParentNameSnapshot *string
	Summary            *string
	OccurredAt         time.Time
	ActivityDate       string // YYYY-MM-DD
}

type PersonalNote struct {
	ID        uuid.UUID
	UserID    uuid.UUID
	Title     string
	CreatedAt time.Time
	UpdatedAt time.Time
	Tasks     []PersonalNoteTask
}

type PersonalNoteTask struct {
	ID          uuid.UUID
	NoteID      uuid.UUID
	Content     string
	IsCompleted bool
	OrderIndex  int
	CreatedAt   time.Time
	UpdatedAt   time.Time
}

type ImportJob struct {
	ID                uuid.UUID
	UserID            uuid.UUID
	SkillID           *uuid.UUID
	OriginalFilename  string
	FileSizeBytes     int
	Status            ImportJobStatus
	ConfidenceNotes   any
	ErrorMessage      *string
	GeneratedTreeJSON map[string]any
	CreatedAt         time.Time
	UpdatedAt         time.Time
}

type RefreshToken struct {
	ID        uuid.UUID
	UserID    uuid.UUID
	TokenHash string
	ExpiresAt time.Time
	Revoked   bool
	CreatedAt time.Time
}

type PasswordResetToken struct {
	ID        uuid.UUID
	UserID    uuid.UUID
	CodeHash  string
	ExpiresAt time.Time
	Used      bool
	CreatedAt time.Time
}

type SkillTimer struct {
	ID        uuid.UUID
	UserID    uuid.UUID
	SkillID   uuid.UUID
	FromDate  string // YYYY-MM-DD
	ToDate    string // YYYY-MM-DD
	CreatedAt time.Time
	UpdatedAt time.Time
}

type Goal struct {
	ID             uuid.UUID
	UserID         uuid.UUID
	GoalName       string
	DeadlineAt     time.Time
	Timezone       string
	MotivationLine *string
	DeletedAt      *time.Time
	PurgeAt        *time.Time
	CreatedAt      time.Time
	UpdatedAt      time.Time
}

type GoalActivity struct {
	ID           uuid.UUID
	GoalID       uuid.UUID
	UserID       uuid.UUID
	ActivityText string
	ActivityDate string // YYYY-MM-DD
	CreatedAt    time.Time
}
