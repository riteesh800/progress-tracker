package services

import (
	"context"
	"fmt"
	"math"
	"sort"
	"strings"
	"time"

	"github.com/google/uuid"

	"skill-progress-tracker/backend-go/internal/database"
	"skill-progress-tracker/backend-go/internal/models"
)

type NodeProgress struct {
	TopicID         uuid.UUID
	IsLeaf          bool
	CompletedLeaves int
	TotalLeaves     int
	Percent         float64
	IsCompleted     bool
}

func (np NodeProgress) Empty() bool {
	return np.TotalLeaves == 0
}

func calcPercent(completed, total int) float64 {
	if total == 0 {
		return 0.0
	}
	return math.Round((float64(completed)/float64(total))*10000) / 100
}

type leafCountPair struct {
	completed int
	total     int
}

type ProgressEngine struct {
	Topics       map[uuid.UUID]models.Topic
	Children     map[uuid.UUID][]models.Topic
	RootChildren []models.Topic
	countsCache  map[uuid.UUID]leafCountPair
	leavesCache  map[uuid.UUID][]models.Topic
}

func NewProgressEngine(topics []models.Topic) *ProgressEngine {
	e := &ProgressEngine{
		Topics:       make(map[uuid.UUID]models.Topic, len(topics)),
		Children:     make(map[uuid.UUID][]models.Topic),
		RootChildren: make([]models.Topic, 0),
		countsCache:  make(map[uuid.UUID]leafCountPair, len(topics)),
		leavesCache:  make(map[uuid.UUID][]models.Topic),
	}

	for _, t := range topics {
		e.Topics[t.ID] = t
		if t.ParentID == nil {
			e.RootChildren = append(e.RootChildren, t)
		} else {
			pid := *t.ParentID
			e.Children[pid] = append(e.Children[pid], t)
		}
	}

	sortSiblings := func(slice []models.Topic) {
		sort.Slice(slice, func(i, j int) bool {
			if slice[i].OrderIndex != slice[j].OrderIndex {
				return slice[i].OrderIndex < slice[j].OrderIndex
			}
			return slice[i].ID.String() < slice[j].ID.String()
		})
	}

	sortSiblings(e.RootChildren)
	for pid := range e.Children {
		sortSiblings(e.Children[pid])
	}

	return e
}

func (e *ProgressEngine) IsLeaf(topicID uuid.UUID) bool {
	return len(e.Children[topicID]) == 0
}

func (e *ProgressEngine) DescendantLeaves(topicID uuid.UUID, seen map[uuid.UUID]bool) []models.Topic {
	if cached, ok := e.leavesCache[topicID]; ok {
		return cached
	}
	if seen == nil {
		seen = make(map[uuid.UUID]bool)
	}
	if seen[topicID] {
		return nil
	}
	seen[topicID] = true

	node, ok := e.Topics[topicID]
	if !ok {
		return nil
	}
	kids := e.Children[topicID]
	var res []models.Topic
	if len(kids) == 0 {
		res = []models.Topic{node}
	} else {
		for _, child := range kids {
			res = append(res, e.DescendantLeaves(child.ID, seen)...)
		}
	}
	e.leavesCache[topicID] = res
	return res
}

func (e *ProgressEngine) leafCounts(topicID uuid.UUID, seen map[uuid.UUID]bool) (int, int) {
	if cached, ok := e.countsCache[topicID]; ok {
		return cached.completed, cached.total
	}
	if seen == nil {
		seen = make(map[uuid.UUID]bool)
	}
	if seen[topicID] {
		return 0, 0
	}
	seen[topicID] = true

	kids := e.Children[topicID]
	var pair leafCountPair
	if len(kids) == 0 {
		node := e.Topics[topicID]
		comp := 0
		if node.IsCompleted {
			comp = 1
		}
		pair = leafCountPair{completed: comp, total: 1}
	} else {
		comp := 0
		tot := 0
		for _, child := range kids {
			c, t := e.leafCounts(child.ID, seen)
			comp += c
			tot += t
		}
		pair = leafCountPair{completed: comp, total: tot}
	}
	e.countsCache[topicID] = pair
	return pair.completed, pair.total
}

func (e *ProgressEngine) NodeProgress(topicID uuid.UUID) NodeProgress {
	completed, total := e.leafCounts(topicID, nil)
	leaf := e.IsLeaf(topicID)
	isCompleted := false
	if leaf {
		if t, ok := e.Topics[topicID]; ok {
			isCompleted = t.IsCompleted
		}
	}
	return NodeProgress{
		TopicID:         topicID,
		IsLeaf:          leaf,
		CompletedLeaves: completed,
		TotalLeaves:     total,
		Percent:         calcPercent(completed, total),
		IsCompleted:     isCompleted,
	}
}

func (e *ProgressEngine) SkillProgress() (int, int, float64) {
	total := 0
	completed := 0
	for tid, t := range e.Topics {
		if e.IsLeaf(tid) {
			total++
			if t.IsCompleted {
				completed++
			}
		}
	}
	return completed, total, calcPercent(completed, total)
}

func (e *ProgressEngine) WouldCreateCycle(topicID uuid.UUID, newParentID *uuid.UUID) bool {
	if newParentID == nil {
		return false
	}
	if topicID == *newParentID {
		return true
	}
	current := newParentID
	seen := make(map[uuid.UUID]bool)
	for current != nil {
		if *current == topicID {
			return true
		}
		if seen[*current] {
			break
		}
		seen[*current] = true
		parent, ok := e.Topics[*current]
		if !ok {
			break
		}
		current = parent.ParentID
	}
	return false
}

func (e *ProgressEngine) BuildNode(topic models.Topic, seen map[uuid.UUID]bool) models.TopicNodeOut {
	if seen == nil {
		seen = make(map[uuid.UUID]bool)
	}
	prog := e.NodeProgress(topic.ID)
	isCompleted := false
	if prog.IsLeaf {
		isCompleted = topic.IsCompleted
	}
	if seen[topic.ID] {
		return models.TopicNodeOut{
			ID:              topic.ID,
			SkillID:         topic.SkillID,
			ParentID:        topic.ParentID,
			Name:            topic.Name,
			Description:     topic.Description,
			OrderIndex:      topic.OrderIndex,
			IsLeaf:          prog.IsLeaf,
			IsCompleted:     isCompleted,
			CompletedLeaves: prog.CompletedLeaves,
			TotalLeaves:     prog.TotalLeaves,
			Percent:         prog.Percent,
			Children:        []models.TopicNodeOut{},
		}
	}
	seen[topic.ID] = true
	childTopics := e.Children[topic.ID]
	kids := make([]models.TopicNodeOut, 0, len(childTopics))
	for _, c := range childTopics {
		kids = append(kids, e.BuildNode(c, seen))
	}
	return models.TopicNodeOut{
		ID:              topic.ID,
		SkillID:         topic.SkillID,
		ParentID:        topic.ParentID,
		Name:            topic.Name,
		Description:     topic.Description,
		OrderIndex:      topic.OrderIndex,
		IsLeaf:          prog.IsLeaf,
		IsCompleted:     isCompleted,
		CompletedLeaves: prog.CompletedLeaves,
		TotalLeaves:     prog.TotalLeaves,
		Percent:         prog.Percent,
		Children:        kids,
	}
}

func (e *ProgressEngine) BuildTree() []models.TopicNodeOut {
	out := make([]models.TopicNodeOut, 0, len(e.RootChildren))
	for _, r := range e.RootChildren {
		out = append(out, e.BuildNode(r, nil))
	}
	return out
}

func (e *ProgressEngine) Preorder() []models.Topic {
	var out []models.Topic
	visited := make(map[uuid.UUID]bool)
	var walk func(items []models.Topic)
	walk = func(items []models.Topic) {
		for _, child := range items {
			if visited[child.ID] {
				continue
			}
			visited[child.ID] = true
			out = append(out, child)
			walk(e.Children[child.ID])
		}
	}
	walk(e.RootChildren)
	return out
}

func ScanTopicRow(scanner interface{ Scan(dest ...any) error }) (models.Topic, error) {
	var (
		idRaw          any
		skillIDRaw     any
		parentIDRaw    any
		name           string
		descRaw        any
		orderIndex     int
		isCompletedRaw any
		completedAtRaw any
		createdAtRaw   any
		updatedAtRaw   any
	)
	if err := scanner.Scan(
		&idRaw,
		&skillIDRaw,
		&parentIDRaw,
		&name,
		&descRaw,
		&orderIndex,
		&isCompletedRaw,
		&completedAtRaw,
		&createdAtRaw,
		&updatedAtRaw,
	); err != nil {
		return models.Topic{}, err
	}
	id, err := database.ParseUUID(idRaw)
	if err != nil {
		return models.Topic{}, err
	}
	skillID, err := database.ParseUUID(skillIDRaw)
	if err != nil {
		return models.Topic{}, err
	}
	parentID, _ := database.ParseNullUUID(parentIDRaw)
	completedAt, _ := database.ParseNullTime(completedAtRaw)
	createdAt, _ := database.ParseTime(createdAtRaw)
	updatedAt, _ := database.ParseTime(updatedAtRaw)

	return models.Topic{
		ID:          id,
		SkillID:     skillID,
		ParentID:    parentID,
		Name:        name,
		Description: database.ParseNullString(descRaw),
		OrderIndex:  orderIndex,
		IsCompleted: database.ParseBool(isCompletedRaw),
		CompletedAt: completedAt,
		CreatedAt:   createdAt,
		UpdatedAt:   updatedAt,
	}, nil
}

func LoadSkillTopics(ctx context.Context, db *database.DB, q database.Querier, skillID uuid.UUID) ([]models.Topic, error) {
	rows, err := db.Query(ctx, q, `
		SELECT id, skill_id, parent_id, name, description, order_index, is_completed, completed_at, created_at, updated_at
		FROM topics
		WHERE skill_id = ?
	`, db.FormatUUID(skillID))
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var topics []models.Topic
	for rows.Next() {
		t, err := ScanTopicRow(rows)
		if err != nil {
			return nil, err
		}
		topics = append(topics, t)
	}
	return topics, rows.Err()
}

func LoadUserTopicsGrouped(ctx context.Context, db *database.DB, q database.Querier, skillIDs []uuid.UUID) (map[uuid.UUID][]models.Topic, error) {
	grouped := make(map[uuid.UUID][]models.Topic, len(skillIDs))
	if len(skillIDs) == 0 {
		return grouped, nil
	}
	placeholders := make([]string, len(skillIDs))
	args := make([]any, len(skillIDs))
	for i, id := range skillIDs {
		placeholders[i] = "?"
		args[i] = db.FormatUUID(id)
	}
	sqlStr := fmt.Sprintf(`
		SELECT id, skill_id, parent_id, name, description, order_index, is_completed, completed_at, created_at, updated_at
		FROM topics
		WHERE skill_id IN (%s)
	`, strings.Join(placeholders, ", "))

	rows, err := db.Query(ctx, q, sqlStr, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	for rows.Next() {
		t, err := ScanTopicRow(rows)
		if err != nil {
			return nil, err
		}
		grouped[t.SkillID] = append(grouped[t.SkillID], t)
	}
	return grouped, rows.Err()
}

// ComputeStreak implements the 1-day grace period streak rule (Part 5):
// Streak breaks only after two or more consecutive calendar days with zero topic_completed activity.
// Missing exactly one day does not break; that day contributes nothing and the run continues.
func ComputeStreak(completionDates []string, todayStr string) (int, int) {
	seen := make(map[string]time.Time)
	for _, dStr := range completionDates {
		if len(dStr) >= 10 {
			dStr = dStr[:10]
		}
		if t, err := time.Parse("2006-01-02", dStr); err == nil {
			seen[dStr] = t
		}
	}
	if len(seen) == 0 {
		return 0, 0
	}

	unique := make([]time.Time, 0, len(seen))
	for _, t := range seen {
		unique = append(unique, t)
	}
	sort.Slice(unique, func(i, j int) bool {
		return unique[i].Before(unique[j])
	})

	type runSpan struct {
		start int
		end   int
	}
	runLength := func(start, end int) int {
		return end - start + 1
	}

	longest := 1
	runStart := 0
	var runs []runSpan
	for i := 1; i < len(unique); i++ {
		gap := int(math.Round(unique[i].Sub(unique[i-1]).Hours() / 24))
		if gap <= 2 {
			continue
		}
		runs = append(runs, runSpan{start: runStart, end: i - 1})
		if l := runLength(runStart, i-1); l > longest {
			longest = l
		}
		runStart = i
	}
	runs = append(runs, runSpan{start: runStart, end: len(unique) - 1})
	if l := runLength(runStart, len(unique)-1); l > longest {
		longest = l
	}

	lastRun := runs[len(runs)-1]
	lastLen := runLength(lastRun.start, lastRun.end)
	latest := unique[lastRun.end]

	todayTime, err := time.Parse("2006-01-02", todayStr[:10])
	if err != nil {
		now := time.Now().UTC()
		todayTime = time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, time.UTC)
	}
	daysSince := int(math.Round(todayTime.Sub(latest).Hours() / 24))

	current := 0
	if daysSince <= 2 {
		current = lastLen
	}
	return current, longest
}

func StreakForUser(ctx context.Context, db *database.DB, q database.Querier, userID uuid.UUID, todayStr string) (int, int, error) {
	rows, err := db.Query(ctx, q, `
		SELECT DISTINCT activity_date
		FROM activity_log
		WHERE user_id = ? AND action_type = ?
	`, db.FormatUUID(userID), string(models.ActionTopicCompleted))
	if err != nil {
		return 0, 0, err
	}
	defer rows.Close()

	var dates []string
	for rows.Next() {
		var raw any
		if err := rows.Scan(&raw); err != nil {
			return 0, 0, err
		}
		dStr := database.ParseDateString(raw)
		if dStr != "" {
			dates = append(dates, dStr)
		}
	}
	if err := rows.Err(); err != nil {
		return 0, 0, err
	}
	cur, longest := ComputeStreak(dates, todayStr)
	return cur, longest, nil
}

const MaxDepth = 10

func ComputeSkillProgress(topics []models.Topic) (int, int, float64, bool) {
	if len(topics) == 0 {
		return 0, 0, 0.0, true
	}
	engine := NewProgressEngine(topics)
	comp, total, pct := engine.SkillProgress()
	return comp, total, pct, total == 0
}

func BuildTopicTree(topics []models.Topic) []models.TopicNodeOut {
	engine := NewProgressEngine(topics)
	return engine.BuildTree()
}

func FindNode(nodes []models.TopicNodeOut, targetID uuid.UUID) *models.TopicNodeOut {
	for i := range nodes {
		if nodes[i].ID == targetID {
			return &nodes[i]
		}
		if len(nodes[i].Children) > 0 {
			if found := FindNode(nodes[i].Children, targetID); found != nil {
				return found
			}
		}
	}
	return nil
}

func ComputeDepth(byID map[uuid.UUID]models.Topic, topicID uuid.UUID) int {
	depth := 1
	seen := make(map[uuid.UUID]bool)
	curr, ok := byID[topicID]
	for ok && curr.ParentID != nil {
		if seen[curr.ID] {
			break
		}
		seen[curr.ID] = true
		depth++
		curr, ok = byID[*curr.ParentID]
	}
	return depth
}
