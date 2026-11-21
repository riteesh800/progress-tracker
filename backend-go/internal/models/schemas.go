package models

import (
	"encoding/json"
	"errors"
	"net/mail"
	"regexp"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/google/uuid"
)

var emailPattern = regexp.MustCompile(`^[^\s@]+@[^\s@]+\.[^\s@]+$`)

func ValidateEmail(raw string) (string, error) {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" || len(trimmed) > 320 || !emailPattern.MatchString(trimmed) {
		return "", errors.New("invalid email")
	}
	addr, err := mail.ParseAddress(trimmed)
	if err != nil || addr.Address != trimmed {
		return "", errors.New("invalid email")
	}
	return trimmed, nil
}

func ValidateTimezone(tz string) error {
	if strings.TrimSpace(tz) == "" {
		return errors.New("empty timezone")
	}
	_, err := time.LoadLocation(tz)
	return err
}

func ValidateDate(d string) (time.Time, error) {
	return time.Parse("2006-01-02", strings.TrimSpace(d))
}

type TokenPair struct {
	AccessToken  string `json:"access_token"`
	RefreshToken string `json:"refresh_token"`
	TokenType    string `json:"token_type"`
}

func NewTokenPair(access, refresh string) TokenPair {
	return TokenPair{
		AccessToken:  access,
		RefreshToken: refresh,
		TokenType:    "bearer",
	}
}

type RegisterIn struct {
	Email    string  `json:"email"`
	Password string  `json:"password"`
	Name     *string `json:"name"`
	Timezone *string `json:"timezone"`
}

func (r *RegisterIn) Validate() error {
	email, err := ValidateEmail(r.Email)
	if err != nil {
		return err
	}
	r.Email = email
	pwLen := utf8.RuneCountInString(r.Password)
	if pwLen < 8 || pwLen > 128 {
		return errors.New("password length must be 8..128")
	}
	if r.Name != nil && utf8.RuneCountInString(*r.Name) > 200 {
		return errors.New("name too long")
	}
	tz := "UTC"
	if r.Timezone != nil {
		tz = *r.Timezone
	}
	if err := ValidateTimezone(tz); err != nil {
		return err
	}
	r.Timezone = &tz
	return nil
}

type LoginIn struct {
	Email    string  `json:"email"`
	Password *string `json:"password"`
}

func (l *LoginIn) Validate() error {
	email, err := ValidateEmail(l.Email)
	if err != nil {
		return err
	}
	l.Email = email
	if l.Password == nil {
		return errors.New("password required")
	}
	return nil
}

type RefreshIn struct {
	RefreshToken *string `json:"refresh_token"`
}

func (r *RefreshIn) Validate() error {
	if r.RefreshToken == nil {
		return errors.New("refresh_token required")
	}
	return nil
}

type GoogleIn struct {
	IDToken  *string `json:"id_token"`
	Timezone *string `json:"timezone"`
}

func (g *GoogleIn) Validate() error {
	if g.IDToken == nil {
		return errors.New("id_token required")
	}
	tz := "UTC"
	if g.Timezone != nil {
		tz = *g.Timezone
	}
	if err := ValidateTimezone(tz); err != nil {
		return err
	}
	g.Timezone = &tz
	return nil
}

type ForgotPasswordIn struct {
	Email string `json:"email"`
}

func (f *ForgotPasswordIn) Validate() error {
	email, err := ValidateEmail(f.Email)
	if err != nil {
		return err
	}
	f.Email = email
	return nil
}

type VerifyResetIn struct {
	Email string  `json:"email"`
	Code  *string `json:"code"`
}

func (v *VerifyResetIn) Validate() error {
	email, err := ValidateEmail(v.Email)
	if err != nil {
		return err
	}
	v.Email = email
	if v.Code == nil {
		return errors.New("code required")
	}
	return nil
}

type ResetPasswordIn struct {
	Email       string  `json:"email"`
	Code        *string `json:"code"`
	NewPassword string  `json:"new_password"`
}

func (r *ResetPasswordIn) Validate() error {
	email, err := ValidateEmail(r.Email)
	if err != nil {
		return err
	}
	r.Email = email
	if r.Code == nil {
		return errors.New("code required")
	}
	pwLen := utf8.RuneCountInString(r.NewPassword)
	if pwLen < 8 || pwLen > 128 {
		return errors.New("new_password length must be 8..128")
	}
	return nil
}

type ForcedPasswordIn struct {
	NewPassword string `json:"new_password"`
}

func (f *ForcedPasswordIn) Validate() error {
	pwLen := utf8.RuneCountInString(f.NewPassword)
	if pwLen < 8 || pwLen > 128 {
		return errors.New("new_password length must be 8..128")
	}
	return nil
}

type UserOut struct {
	ID                 uuid.UUID `json:"id"`
	Email              string    `json:"email"`
	Name               string    `json:"name"`
	Timezone           string    `json:"timezone"`
	IsActive           bool      `json:"is_active"`
	CanOpenAdmin       bool      `json:"can_open_admin"`
	MustChangePassword bool      `json:"must_change_password"`
}

type AdminUnlockIn struct {
	AccessCode string `json:"access_code"`
}

func (a *AdminUnlockIn) Validate() error {
	l := utf8.RuneCountInString(a.AccessCode)
	if l < 1 || l > 128 {
		return errors.New("access_code length must be 1..128")
	}
	return nil
}

type AdminUnlockOut struct {
	AdminToken string `json:"admin_token"`
	ExpiresIn  int    `json:"expires_in"`
}

type AdminUserOut struct {
	ID     uuid.UUID `json:"id"`
	Name   string    `json:"name"`
	Email  string    `json:"email"`
	Status string    `json:"status"`
}

type AdminPasswordIn struct {
	NewPassword string `json:"new_password"`
}

func (a *AdminPasswordIn) Validate() error {
	pwLen := utf8.RuneCountInString(a.NewPassword)
	if pwLen < 8 || pwLen > 128 {
		return errors.New("new_password length must be 8..128")
	}
	return nil
}

type UserUpdateIn struct {
	Name     *string `json:"name"`
	Timezone *string `json:"timezone"`
}

func (u *UserUpdateIn) Validate() error {
	if u.Name != nil {
		if utf8.RuneCountInString(*u.Name) > 200 {
			return errors.New("name too long")
		}
		trimmed := strings.TrimSpace(*u.Name)
		u.Name = &trimmed
	}
	if u.Timezone != nil {
		if err := ValidateTimezone(*u.Timezone); err != nil {
			return err
		}
	}
	return nil
}

type SkillIn struct {
	Name        string  `json:"name"`
	Description *string `json:"description"`
}

func (s *SkillIn) Validate() error {
	if utf8.RuneCountInString(s.Name) > 200 {
		return errors.New("name too long")
	}
	trimmed := strings.TrimSpace(s.Name)
	if trimmed == "" {
		return errors.New("skill name cannot be empty")
	}
	s.Name = trimmed
	return nil
}

type SkillUpdateIn struct {
	Name        *string `json:"name"`
	Description *string `json:"description"`
}

func (s *SkillUpdateIn) Validate() error {
	if s.Name != nil {
		if utf8.RuneCountInString(*s.Name) > 200 {
			return errors.New("name too long")
		}
		trimmed := strings.TrimSpace(*s.Name)
		if trimmed == "" {
			return errors.New("skill name cannot be empty")
		}
		s.Name = &trimmed
	}
	return nil
}

type SkillOut struct {
	ID              uuid.UUID  `json:"id"`
	Name            string     `json:"name"`
	Description     *string    `json:"description"`
	CreatedAt       time.Time  `json:"created_at"`
	CompletedLeaves int        `json:"completed_leaves"`
	TotalLeaves     int        `json:"total_leaves"`
	Percent         float64    `json:"percent"`
	Empty           bool       `json:"empty"`
	DeletedAt       *time.Time `json:"deleted_at"`
	PurgeAt         *time.Time `json:"purge_at"`
}

type TopicIn struct {
	SkillID     string  `json:"skill_id"`
	ParentID    *string `json:"parent_id"`
	Name        string  `json:"name"`
	Description *string `json:"description"`

	ParsedSkillID  uuid.UUID  `json:"-"`
	ParsedParentID *uuid.UUID `json:"-"`
}

func (t *TopicIn) Validate() error {
	sid, err := uuid.Parse(strings.TrimSpace(t.SkillID))
	if err != nil {
		return err
	}
	t.ParsedSkillID = sid
	if t.ParentID != nil && strings.TrimSpace(*t.ParentID) != "" {
		pid, err := uuid.Parse(strings.TrimSpace(*t.ParentID))
		if err != nil {
			return err
		}
		t.ParsedParentID = &pid
	}
	if utf8.RuneCountInString(t.Name) > 500 {
		return errors.New("topic name too long")
	}
	trimmed := strings.TrimSpace(t.Name)
	if trimmed == "" {
		return errors.New("topic name cannot be empty")
	}
	t.Name = trimmed
	return nil
}

type TopicCompleteIn struct {
	Completed *bool `json:"completed"`
}

func (t *TopicCompleteIn) Validate() error {
	if t.Completed == nil {
		return errors.New("completed required")
	}
	return nil
}

type TopicNodeOut struct {
	ID              uuid.UUID      `json:"id"`
	SkillID         uuid.UUID      `json:"skill_id"`
	ParentID        *uuid.UUID     `json:"parent_id"`
	Name            string         `json:"name"`
	Description     *string        `json:"description"`
	OrderIndex      int            `json:"order_index"`
	IsLeaf          bool           `json:"is_leaf"`
	IsCompleted     bool           `json:"is_completed"`
	CompletedLeaves int            `json:"completed_leaves"`
	TotalLeaves     int            `json:"total_leaves"`
	Percent         float64        `json:"percent"`
	HasNotes        bool           `json:"has_notes"`
	Children        []TopicNodeOut `json:"children"`
}

type DeleteImpactOut struct {
	Descendants     int  `json:"descendants"`
	Notes           int  `json:"notes"`
	RequiresConfirm bool `json:"requires_confirm"`
}

type NoteIn struct {
	TopicID       string    `json:"topic_id"`
	Content       string    `json:"content"`
	ParsedTopicID uuid.UUID `json:"-"`
}

func (n *NoteIn) Validate() error {
	tid, err := uuid.Parse(strings.TrimSpace(n.TopicID))
	if err != nil {
		return err
	}
	n.ParsedTopicID = tid
	l := utf8.RuneCountInString(n.Content)
	if l < 1 || l > 20000 {
		return errors.New("note content length must be 1..20000")
	}
	return nil
}

type NoteUpdateIn struct {
	Content string `json:"content"`
}

func (n *NoteUpdateIn) Validate() error {
	l := utf8.RuneCountInString(n.Content)
	if l < 1 || l > 20000 {
		return errors.New("note content length must be 1..20000")
	}
	trimmed := strings.TrimSpace(n.Content)
	if trimmed == "" {
		return errors.New("note content cannot be empty")
	}
	n.Content = trimmed
	return nil
}

type NoteOut struct {
	ID        uuid.UUID `json:"id"`
	TopicID   uuid.UUID `json:"topic_id"`
	Content   string    `json:"content"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

type StreakOut struct {
	CurrentStreak int `json:"current_streak"`
	LongestStreak int `json:"longest_streak"`
}

type StreakDayOut struct {
	Date  string `json:"date"`
	Count int    `json:"count"`
}

type StreakCalendarOut struct {
	Start            string         `json:"start"`
	End              string         `json:"end"`
	Timezone         string         `json:"timezone"`
	TotalSubmissions int            `json:"total_submissions"`
	TotalActiveDays  int            `json:"total_active_days"`
	CurrentStreak    int            `json:"current_streak"`
	LongestStreak    int            `json:"longest_streak"`
	Days             []StreakDayOut `json:"days"`
}

type SearchHit struct {
	Kind    string     `json:"kind"`
	ID      uuid.UUID  `json:"id"`
	Title   string     `json:"title"`
	Snippet string     `json:"snippet"`
	SkillID *uuid.UUID `json:"skill_id"`
	TopicID *uuid.UUID `json:"topic_id"`
}

type ActivityOut struct {
	ID                 uuid.UUID  `json:"id"`
	ActionType         string     `json:"action_type"`
	TopicID            *uuid.UUID `json:"topic_id"`
	SkillID            *uuid.UUID `json:"skill_id"`
	TopicNameSnapshot  *string    `json:"topic_name_snapshot"`
	SkillNameSnapshot  *string    `json:"skill_name_snapshot"`
	ParentNameSnapshot *string    `json:"parent_name_snapshot"`
	Summary            *string    `json:"summary"`
	OccurredAt         time.Time  `json:"occurred_at"`
	ActivityDate       string     `json:"activity_date"`
}

type PersonalNoteTaskIn struct {
	Content     string `json:"content"`
	IsCompleted bool   `json:"is_completed"`
}

type PersonalNoteTaskOut struct {
	ID          uuid.UUID `json:"id"`
	Content     string    `json:"content"`
	IsCompleted bool      `json:"is_completed"`
	OrderIndex  int       `json:"order_index"`
}

type PersonalNoteIn struct {
	Title *string `json:"title"`
}

func (p *PersonalNoteIn) Validate() error {
	if p.Title != nil && utf8.RuneCountInString(*p.Title) > 35 {
		return errors.New("title too long")
	}
	return nil
}

type PersonalNoteUpdateIn struct {
	Title *string               `json:"title"`
	Tasks *[]PersonalNoteTaskIn `json:"tasks"`
}

func (p *PersonalNoteUpdateIn) Validate() error {
	if p.Title != nil && utf8.RuneCountInString(*p.Title) > 35 {
		return errors.New("title too long")
	}
	return nil
}

type PersonalNoteOut struct {
	ID        uuid.UUID             `json:"id"`
	Title     string                `json:"title"`
	CreatedAt time.Time             `json:"created_at"`
	UpdatedAt time.Time             `json:"updated_at"`
	Tasks     []PersonalNoteTaskOut `json:"tasks"`
}

type DashboardOut struct {
	TotalSkills     int     `json:"total_skills"`
	TotalTopics     int     `json:"total_topics"`
	CompletedTopics int     `json:"completed_topics"`
	OverallProgress float64 `json:"overall_progress"`
	CurrentStreak   int     `json:"current_streak"`
	LongestStreak   int     `json:"longest_streak"`
	Empty           bool    `json:"empty"`
}

type SuggestedNextOut struct {
	SkillID   *uuid.UUID `json:"skill_id"`
	SkillName *string    `json:"skill_name"`
	TopicID   *uuid.UUID `json:"topic_id"`
	TopicName *string    `json:"topic_name"`
	Reason    string     `json:"reason"`
}

type ImportJobOut struct {
	ID                uuid.UUID      `json:"id"`
	Status            string         `json:"status"`
	OriginalFilename  string         `json:"original_filename"`
	FileSizeBytes     int            `json:"file_size_bytes"`
	SkillID           *uuid.UUID     `json:"skill_id"`
	GeneratedTreeJSON map[string]any `json:"generated_tree_json"`
	ConfidenceNotes   any            `json:"confidence_notes"`
	ErrorMessage      *string        `json:"error_message"`
}

type TreePatchIn struct {
	GeneratedTreeJSON map[string]any `json:"generated_tree_json"`
}

func (t *TreePatchIn) Validate() error {
	if t.GeneratedTreeJSON == nil {
		return errors.New("generated_tree_json required")
	}
	return nil
}

type ConfirmImportIn struct {
	SkillName        *string `json:"skill_name"`
	SkillDescription *string `json:"skill_description"`
}

func (c *ConfirmImportIn) Validate() error {
	if c.SkillName != nil {
		l := utf8.RuneCountInString(*c.SkillName)
		if l < 1 || l > 200 {
			return errors.New("skill_name must be 1..200 chars")
		}
	}
	return nil
}

type PDFUploadURLIn struct {
	Filename      string `json:"filename"`
	ContentType   string `json:"content_type"`
	FileSizeBytes int64  `json:"file_size_bytes"`
}

func (p *PDFUploadURLIn) Validate() error {
	p.Filename = strings.TrimSpace(p.Filename)
	if p.Filename == "" {
		return errors.New("filename required")
	}
	if p.FileSizeBytes <= 0 {
		return errors.New("file_size_bytes must be positive")
	}
	return nil
}

type PDFUploadURLOut struct {
	UploadURL       string `json:"upload_url"`
	ObjectKey       string `json:"object_key"`
	UploadToken     string `json:"upload_token"`
	StorageProvider string `json:"storage_provider"`
	ExpiresIn       int    `json:"expires_in"`
	MaxBytes        int64  `json:"max_bytes"`
}

type PDFUploadCompleteIn struct {
	ObjectKey        string `json:"object_key"`
	OriginalFilename string `json:"original_filename"`
	FileSizeBytes    int64  `json:"file_size_bytes"`
	UploadToken      string `json:"upload_token"`
}

func (p *PDFUploadCompleteIn) Validate() error {
	p.ObjectKey = strings.TrimSpace(p.ObjectKey)
	if p.ObjectKey == "" {
		return errors.New("object_key required")
	}
	p.OriginalFilename = strings.TrimSpace(p.OriginalFilename)
	if p.OriginalFilename == "" {
		p.OriginalFilename = "upload.pdf"
	}
	if p.FileSizeBytes <= 0 {
		return errors.New("file_size_bytes must be positive")
	}
	p.UploadToken = strings.TrimSpace(p.UploadToken)
	if p.UploadToken == "" {
		return errors.New("upload_token required")
	}
	return nil
}

type SkillTimerIn struct {
	FromDate string `json:"from_date"`
	ToDate   string `json:"to_date"`
}

func (s *SkillTimerIn) Validate() error {
	from, err := ValidateDate(s.FromDate)
	if err != nil {
		return err
	}
	to, err := ValidateDate(s.ToDate)
	if err != nil {
		return err
	}
	s.FromDate = from.Format("2006-01-02")
	s.ToDate = to.Format("2006-01-02")
	return nil
}

type SkillTimerOut struct {
	ID       uuid.UUID `json:"id"`
	SkillID  uuid.UUID `json:"skill_id"`
	FromDate string    `json:"from_date"`
	ToDate   string    `json:"to_date"`
}

var allowedDepths = map[string]bool{
	"easy":          true,
	"medium":        true,
	"intermediate":  true,
	"comprehensive": true,
}

var allowedPurposes = map[string]bool{
	"interview": true,
	"quiz":      true,
	"a_to_z":    true,
}

type GenerateSyllabusIn struct {
	Subject string `json:"subject"`
	Depth   string `json:"depth"`
	Purpose string `json:"purpose"`
}

func (g *GenerateSyllabusIn) Validate() error {
	if utf8.RuneCountInString(g.Subject) > 300 {
		return errors.New("subject too long")
	}
	g.Subject = strings.TrimSpace(g.Subject)
	if g.Subject == "" {
		return errors.New("subject cannot be empty")
	}
	g.Depth = strings.ToLower(strings.TrimSpace(g.Depth))
	if !allowedDepths[g.Depth] {
		return errors.New("invalid depth")
	}
	g.Purpose = strings.ToLower(strings.TrimSpace(g.Purpose))
	if !allowedPurposes[g.Purpose] {
		return errors.New("invalid purpose")
	}
	return nil
}

type TopicNodeAI struct {
	Name     string        `json:"name"`
	Children []TopicNodeAI `json:"children"`
}

type GenerateSyllabusOut struct {
	SkillName string        `json:"skill_name"`
	Topics    []TopicNodeAI `json:"topics"`
}

type RefineSyllabusIn struct {
	Subject             string              `json:"subject"`
	Depth               string              `json:"depth"`
	Purpose             string              `json:"purpose"`
	CurrentTree         map[string]any      `json:"current_tree"`
	ConversationHistory []map[string]string `json:"conversation_history"`
	Instruction         string              `json:"instruction"`
}

func (r *RefineSyllabusIn) Validate() error {
	if utf8.RuneCountInString(r.Subject) > 300 {
		return errors.New("subject too long")
	}
	r.Subject = strings.TrimSpace(r.Subject)
	if r.Subject == "" {
		return errors.New("subject cannot be empty")
	}
	r.Depth = strings.ToLower(strings.TrimSpace(r.Depth))
	if !allowedDepths[r.Depth] {
		return errors.New("invalid depth")
	}
	r.Purpose = strings.ToLower(strings.TrimSpace(r.Purpose))
	if !allowedPurposes[r.Purpose] {
		return errors.New("invalid purpose")
	}
	if r.CurrentTree == nil {
		return errors.New("current_tree required")
	}
	if utf8.RuneCountInString(r.Instruction) > 2000 {
		return errors.New("instruction too long")
	}
	r.Instruction = strings.TrimSpace(r.Instruction)
	if r.Instruction == "" {
		return errors.New("instruction cannot be empty")
	}
	return nil
}

type ConfirmSyllabusIn struct {
	SkillName        string            `json:"skill_name"`
	SkillDescription *string           `json:"skill_description"`
	Topics           *[]map[string]any `json:"topics"`
}

func (c *ConfirmSyllabusIn) Validate() error {
	if utf8.RuneCountInString(c.SkillName) > 200 {
		return errors.New("skill_name too long")
	}
	c.SkillName = strings.TrimSpace(c.SkillName)
	if c.SkillName == "" {
		return errors.New("skill_name cannot be empty")
	}
	if c.Topics == nil {
		return errors.New("topics required")
	}
	return nil
}

type GoalIn struct {
	GoalName       string  `json:"goal_name"`
	DeadlineAt     string  `json:"deadline_at"`
	Timezone       *string `json:"timezone"`
	MotivationLine *string `json:"motivation_line"`
}

func (g *GoalIn) Validate() error {
	g.GoalName = strings.TrimSpace(g.GoalName)
	if g.GoalName == "" {
		return errors.New("goal_name cannot be empty")
	}
	if utf8.RuneCountInString(g.GoalName) > 100 {
		return errors.New("goal_name exceeds 100 characters")
	}

	tz := "UTC"
	if g.Timezone != nil && strings.TrimSpace(*g.Timezone) != "" {
		tz = strings.TrimSpace(*g.Timezone)
	}
	loc, err := time.LoadLocation(tz)
	if err != nil {
		return errors.New("invalid timezone")
	}
	g.Timezone = &tz

	g.DeadlineAt = strings.TrimSpace(g.DeadlineAt)
	if g.DeadlineAt == "" {
		return errors.New("deadline_at is required")
	}

	// Try standard date-time formats
	var parsedTime time.Time
	formats := []string{
		time.RFC3339Nano,
		time.RFC3339,
		"2006-01-02T15:04:05",
		"2006-01-02T15:04",
		"2006-01-02 15:04:05",
		"2006-01-02 15:04",
	}
	parsed := false
	for _, f := range formats {
		if strings.Contains(f, "Z") || strings.Contains(f, "07:00") {
			t, err := time.Parse(f, g.DeadlineAt)
			if err == nil {
				parsedTime = t
				parsed = true
				break
			}
		} else {
			t, err := time.ParseInLocation(f, g.DeadlineAt, loc)
			if err == nil {
				parsedTime = t
				parsed = true
				break
			}
		}
	}
	if !parsed {
		return errors.New("invalid deadline format, expected date and time")
	}
	g.DeadlineAt = parsedTime.UTC().Format(time.RFC3339)

	if g.MotivationLine != nil {
		trimmed := strings.TrimSpace(*g.MotivationLine)
		if trimmed == "" {
			g.MotivationLine = nil
		} else {
			if utf8.RuneCountInString(trimmed) > 200 {
				return errors.New("motivation_line exceeds 200 characters")
			}
			g.MotivationLine = &trimmed
		}
	}
	return nil
}

type GoalOut struct {
	ID              uuid.UUID  `json:"id"`
	GoalName        string     `json:"goal_name"`
	DeadlineAt      time.Time  `json:"deadline_at"`
	Timezone        string     `json:"timezone"`
	MotivationLine  *string    `json:"motivation_line,omitempty"`
	DeletedAt       *time.Time `json:"deleted_at,omitempty"`
	PurgeAt         *time.Time `json:"purge_at,omitempty"`
	CreatedAt       time.Time  `json:"created_at"`
	UpdatedAt       time.Time  `json:"updated_at"`
	IsExpired       bool       `json:"is_expired"`
	TotalActivities int        `json:"total_activities"`
}

type GoalDailySummary struct {
	WorkedDays        int     `json:"worked_days"`
	MissedDays        int     `json:"missed_days"`
	TotalDays         int     `json:"total_days"`
	IsDeadlineReached bool    `json:"is_deadline_reached"`
	InactivityWarning bool    `json:"inactivity_warning"`
	CurrentScore      float64 `json:"current_score"`
}

type GoalDetailOut struct {
	Goal              GoalOut          `json:"goal"`
	Summary           GoalDailySummary `json:"summary"`
	InactivityWarning bool             `json:"inactivity_warning"`
	MissedDays        int              `json:"missed_days"`
	WorkedDays        int              `json:"worked_days"`
	TotalDays         int              `json:"total_days"`
	CurrentScore      float64          `json:"current_score"`
	IsDeadlineReached bool             `json:"is_deadline_reached"`
}

type GoalActivityIn struct {
	ActivityText string  `json:"activity_text"`
	ActivityDate *string `json:"activity_date"`
}

func (a *GoalActivityIn) Validate() error {
	trimmed := strings.TrimSpace(a.ActivityText)
	if trimmed == "" {
		return errors.New("activity text cannot be empty or only spaces")
	}
	if utf8.RuneCountInString(trimmed) > 50 {
		return errors.New("activity text exceeds 50 characters")
	}
	a.ActivityText = trimmed

	if a.ActivityDate != nil {
		trimmedDate := strings.TrimSpace(*a.ActivityDate)
		if trimmedDate != "" {
			_, err := time.Parse("2006-01-02", trimmedDate)
			if err != nil {
				return errors.New("invalid activity_date format, expected YYYY-MM-DD")
			}
			a.ActivityDate = &trimmedDate
		} else {
			a.ActivityDate = nil
		}
	}
	return nil
}

type GoalActivityOut struct {
	ID           uuid.UUID `json:"id"`
	GoalID       uuid.UUID `json:"goal_id"`
	ActivityText string    `json:"activity_text"`
	ActivityDate string    `json:"activity_date"`
	CreatedAt    time.Time `json:"created_at"`
}

type GoalActivitiesPageOut struct {
	Activities []GoalActivityOut `json:"activities"`
	Total      int               `json:"total"`
	Page       int               `json:"page"`
	PerPage    int               `json:"per_page"`
}

type GoalProgressPoint struct {
	Date         string  `json:"date"`
	HasActivity  bool    `json:"has_activity"`
	DailyChange  float64 `json:"daily_change"`
	Score        float64 `json:"score"`
	IsCurrentDay bool    `json:"is_current_day"`
}

type GoalProgressOut struct {
	GoalID            uuid.UUID           `json:"goal_id"`
	Points            []GoalProgressPoint `json:"points"`
	CurrentScore      float64             `json:"current_score"`
	WorkedDays        int                 `json:"worked_days"`
	MissedDays        int                 `json:"missed_days"`
	TotalTrackedDays  int                 `json:"total_tracked_days"`
	IsDeadlineReached bool                `json:"is_deadline_reached"`
	InactivityWarning bool                `json:"inactivity_warning"`
}

func DecodeAndValidate[T any, PT interface {
	*T
	Validate() error
}](body []byte) (*T, error) {
	var target T
	if err := json.Unmarshal(body, &target); err != nil {
		return nil, err
	}
	if err := PT(&target).Validate(); err != nil {
		return nil, err
	}
	return &target, nil
}
