package services

import (
	"context"
	"strings"
	"time"

	"github.com/google/uuid"

	"skill-progress-tracker/backend-go/internal/database"
	"skill-progress-tracker/backend-go/internal/models"
)

func SafeZoneInfo(tzName string) *time.Location {
	cleaned := strings.TrimSpace(tzName)
	if cleaned == "" {
		return time.UTC
	}
	loc, err := time.LoadLocation(cleaned)
	if err != nil {
		return time.UTC
	}
	return loc
}

func ActivityDateFor(occurredAt time.Time, tzName string) string {
	loc := SafeZoneInfo(tzName)
	return occurredAt.In(loc).Format("2006-01-02")
}

func MakeActivity(
	userID uuid.UUID,
	actionType models.ActionType,
	occurredAt time.Time,
	timezoneName string,
	topicID *uuid.UUID,
	skillID *uuid.UUID,
	topicNameSnapshot *string,
	skillNameSnapshot *string,
	parentNameSnapshot *string,
	summary *string,
) models.ActivityLog {
	if occurredAt.IsZero() {
		occurredAt = time.Now().UTC()
	}
	return models.ActivityLog{
		ID:                 uuid.New(),
		UserID:             userID,
		ActionType:         actionType,
		TopicID:            topicID,
		SkillID:            skillID,
		TopicNameSnapshot:  topicNameSnapshot,
		SkillNameSnapshot:  skillNameSnapshot,
		ParentNameSnapshot: parentNameSnapshot,
		Summary:            summary,
		OccurredAt:         occurredAt.UTC(),
		ActivityDate:       ActivityDateFor(occurredAt, timezoneName),
	}
}

func InsertActivity(ctx context.Context, db *database.DB, q database.Querier, act models.ActivityLog) error {
	if act.ID == uuid.Nil {
		act.ID = uuid.New()
	}
	if act.OccurredAt.IsZero() {
		act.OccurredAt = time.Now().UTC()
	}
	_, err := db.Exec(ctx, q, `
		INSERT INTO activity_log (
			id, user_id, action_type, topic_id, skill_id,
			topic_name_snapshot, skill_name_snapshot, parent_name_snapshot,
			summary, occurred_at, activity_date
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`,
		db.FormatUUID(act.ID),
		db.FormatUUID(act.UserID),
		string(act.ActionType),
		db.FormatNullUUID(act.TopicID),
		db.FormatNullUUID(act.SkillID),
		act.TopicNameSnapshot,
		act.SkillNameSnapshot,
		act.ParentNameSnapshot,
		act.Summary,
		db.FormatTime(act.OccurredAt),
		act.ActivityDate,
	)
	return err
}

func UserToday(tzName string) time.Time {
	loc := SafeZoneInfo(tzName)
	now := time.Now().UTC().In(loc)
	return time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, time.UTC)
}

func LogActivity(
	ctx context.Context,
	db *database.DB,
	q database.Querier,
	user *models.User,
	actionType models.ActionType,
	topicID *uuid.UUID,
	skillID *uuid.UUID,
	topicNameSnapshot *string,
	skillNameSnapshot *string,
	parentNameSnapshot *string,
	summary *string,
) error {
	tz := "UTC"
	if user != nil && user.Timezone != "" {
		tz = user.Timezone
	}
	var uid uuid.UUID
	if user != nil {
		uid = user.ID
	}
	act := MakeActivity(
		uid,
		actionType,
		time.Now().UTC(),
		tz,
		topicID,
		skillID,
		topicNameSnapshot,
		skillNameSnapshot,
		parentNameSnapshot,
		summary,
	)
	return InsertActivity(ctx, db, q, act)
}
