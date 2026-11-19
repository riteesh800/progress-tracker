package handlers

import (
	"math"
	"net/http"
	"strconv"
	"strings"

	"github.com/google/uuid"
	"skill-progress-tracker/backend-go/internal/core"
	"skill-progress-tracker/backend-go/internal/database"
	"skill-progress-tracker/backend-go/internal/models"
	"skill-progress-tracker/backend-go/internal/services"
)

func (s *Server) Dashboard(w http.ResponseWriter, r *http.Request, user *models.User) {
	ctx := r.Context()
	rows, err := s.DB.Query(
		ctx, nil,
		`SELECT `+SkillColumns+` FROM skills WHERE user_id = ? AND deleted_at IS NULL`,
		s.DB.FormatUUID(user.ID),
	)
	if err != nil {
		core.WriteError(w, http.StatusInternalServerError, "internal_error", "Something went wrong. Please try again.")
		return
	}
	var skills []*models.Skill
	var skillIDs []uuid.UUID
	for rows.Next() {
		sk, err := ScanSkill(rows)
		if err != nil {
			rows.Close()
			core.WriteError(w, http.StatusInternalServerError, "internal_error", "Something went wrong. Please try again.")
			return
		}
		skills = append(skills, sk)
		skillIDs = append(skillIDs, sk.ID)
	}
	rows.Close()

	topicsMap, err := s.LoadSkillsTopicsMap(ctx, nil, skillIDs)
	if err != nil {
		core.WriteError(w, http.StatusInternalServerError, "internal_error", "Something went wrong. Please try again.")
		return
	}

	totalLeaves := 0
	completedLeaves := 0
	for _, sk := range skills {
		comp, total, _, _ := services.ComputeSkillProgress(topicsMap[sk.ID])
		completedLeaves += comp
		totalLeaves += total
	}
	overall := 0.0
	if totalLeaves > 0 {
		overall = math.Round((float64(completedLeaves)/float64(totalLeaves))*10000.0) / 100.0
	}

	todayStr := services.UserToday(user.Timezone).Format("2006-01-02")
	current, longest, err := services.StreakForUser(ctx, s.DB, nil, user.ID, todayStr)
	if err != nil {
		core.WriteError(w, http.StatusInternalServerError, "internal_error", "Something went wrong. Please try again.")
		return
	}

	core.WriteJSON(w, http.StatusOK, models.DashboardOut{
		TotalSkills:     len(skills),
		TotalTopics:     totalLeaves,
		CompletedTopics: completedLeaves,
		OverallProgress: overall,
		CurrentStreak:   current,
		LongestStreak:   longest,
		Empty:           len(skills) == 0,
	})
}

func (s *Server) SuggestedNext(w http.ResponseWriter, r *http.Request, user *models.User) {
	ctx := r.Context()
	rows, err := s.DB.Query(
		ctx, nil,
		`SELECT `+SkillColumns+` FROM skills WHERE user_id = ? AND deleted_at IS NULL ORDER BY created_at ASC`,
		s.DB.FormatUUID(user.ID),
	)
	if err != nil {
		core.WriteError(w, http.StatusInternalServerError, "internal_error", "Something went wrong. Please try again.")
		return
	}
	var skills []*models.Skill
	var skillIDs []uuid.UUID
	for rows.Next() {
		sk, err := ScanSkill(rows)
		if err != nil {
			rows.Close()
			core.WriteError(w, http.StatusInternalServerError, "internal_error", "Something went wrong. Please try again.")
			return
		}
		skills = append(skills, sk)
		skillIDs = append(skillIDs, sk.ID)
	}
	rows.Close()

	topicsMap, err := s.LoadSkillsTopicsMap(ctx, nil, skillIDs)
	if err != nil {
		core.WriteError(w, http.StatusInternalServerError, "internal_error", "Something went wrong. Please try again.")
		return
	}

	var bestSkill *models.Skill
	bestPct := 101.0
	var firstSkill *models.Skill
	var firstLeaf *models.Topic

	for _, sk := range skills {
		engine := services.NewProgressEngine(topicsMap[sk.ID])
		comp, total, pct := engine.SkillProgress()
		if total == 0 || comp == total {
			continue
		}
		if firstLeaf == nil {
			for _, node := range engine.Preorder() {
				if engine.IsLeaf(node.ID) && !node.IsCompleted {
					nCopy := node
					firstSkill = sk
					firstLeaf = &nCopy
					break
				}
			}
		}
		if pct < bestPct {
			bestPct = pct
			bestSkill = sk
		}
	}

	if firstLeaf != nil && firstSkill != nil {
		sid := firstSkill.ID
		sname := firstSkill.Name
		tid := firstLeaf.ID
		tname := firstLeaf.Name
		core.WriteJSON(w, http.StatusOK, models.SuggestedNextOut{
			SkillID:   &sid,
			SkillName: &sname,
			TopicID:   &tid,
			TopicName: &tname,
			Reason:    "Oldest incomplete leaf topic in tree order.",
		})
		return
	}
	if bestSkill != nil {
		sid := bestSkill.ID
		sname := bestSkill.Name
		core.WriteJSON(w, http.StatusOK, models.SuggestedNextOut{
			SkillID:   &sid,
			SkillName: &sname,
			TopicID:   nil,
			TopicName: nil,
			Reason:    "Lowest completion skill that still has incomplete leaves.",
		})
		return
	}
	core.WriteJSON(w, http.StatusOK, models.SuggestedNextOut{
		SkillID:   nil,
		SkillName: nil,
		TopicID:   nil,
		TopicName: nil,
		Reason:    "Nothing left to study — nice work.",
	})
}

func (s *Server) Streak(w http.ResponseWriter, r *http.Request, user *models.User) {
	todayStr := services.UserToday(user.Timezone).Format("2006-01-02")
	current, longest, err := services.StreakForUser(r.Context(), s.DB, nil, user.ID, todayStr)
	if err != nil {
		core.WriteError(w, http.StatusInternalServerError, "internal_error", "Something went wrong. Please try again.")
		return
	}
	core.WriteJSON(w, http.StatusOK, models.StreakOut{
		CurrentStreak: current,
		LongestStreak: longest,
	})
}

func (s *Server) StreakCalendar(w http.ResponseWriter, r *http.Request, user *models.User) {
	ctx := r.Context()
	today := services.UserToday(user.Timezone)
	start := today.AddDate(0, 0, -364)
	startStr := start.Format("2006-01-02")
	todayStr := today.Format("2006-01-02")

	rows, err := s.DB.Query(
		ctx, nil,
		`SELECT activity_date, COUNT(id)
		 FROM activity_log
		 WHERE user_id = ? AND action_type = ? AND activity_date >= ? AND activity_date <= ?
		 GROUP BY activity_date`,
		s.DB.FormatUUID(user.ID), string(models.ActionTopicCompleted), startStr, todayStr,
	)
	if err != nil {
		core.WriteError(w, http.StatusInternalServerError, "internal_error", "Something went wrong. Please try again.")
		return
	}
	countsByDate := make(map[string]int)
	for rows.Next() {
		var dVal any
		var cnt int
		if err := rows.Scan(&dVal, &cnt); err == nil {
			countsByDate[database.ParseDateString(dVal)] = cnt
		}
	}
	rows.Close()

	current, longest, err := services.StreakForUser(ctx, s.DB, nil, user.ID, todayStr)
	if err != nil {
		core.WriteError(w, http.StatusInternalServerError, "internal_error", "Something went wrong. Please try again.")
		return
	}

	days := make([]models.StreakDayOut, 0, 365)
	totalSubmissions := 0
	totalActiveDays := 0
	for i := 0; i < 365; i++ {
		d := start.AddDate(0, 0, i).Format("2006-01-02")
		cnt := countsByDate[d]
		totalSubmissions += cnt
		if cnt > 0 {
			totalActiveDays++
		}
		days = append(days, models.StreakDayOut{
			Date:  d,
			Count: cnt,
		})
	}
	tz := user.Timezone
	if tz == "" {
		tz = "UTC"
	}
	core.WriteJSON(w, http.StatusOK, models.StreakCalendarOut{
		Start:            startStr,
		End:              todayStr,
		Timezone:         tz,
		TotalSubmissions: totalSubmissions,
		TotalActiveDays:  totalActiveDays,
		CurrentStreak:    current,
		LongestStreak:    longest,
		Days:             days,
	})
}

func (s *Server) Activity(w http.ResponseWriter, r *http.Request, user *models.User) {
	q := r.URL.Query()
	limit := 20
	if rawLimit := strings.TrimSpace(q.Get("limit")); rawLimit != "" {
		parsed, err := strconv.Atoi(rawLimit)
		if err != nil || parsed < 1 || parsed > 20 {
			core.WriteValidationError(w)
			return
		}
		limit = parsed
	}

	ctx := r.Context()
	rows, err := s.DB.Query(
		ctx, nil,
		`SELECT id, action_type, topic_id, skill_id, topic_name_snapshot, skill_name_snapshot, parent_name_snapshot, summary, occurred_at, activity_date
		 FROM activity_log
		 WHERE user_id = ?
		 ORDER BY occurred_at DESC
		 LIMIT ?`,
		s.DB.FormatUUID(user.ID), limit,
	)
	if err != nil {
		core.WriteError(w, http.StatusInternalServerError, "internal_error", "Something went wrong. Please try again.")
		return
	}
	defer rows.Close()

	items := make([]models.ActivityOut, 0)
	for rows.Next() {
		var rawID, actionType string
		var rawTID, rawSID, tSnap, sSnap, pSnap, summary, occVal, dVal any
		if err := rows.Scan(&rawID, &actionType, &rawTID, &rawSID, &tSnap, &sSnap, &pSnap, &summary, &occVal, &dVal); err != nil {
			core.WriteError(w, http.StatusInternalServerError, "internal_error", "Something went wrong. Please try again.")
			return
		}
		id, err := database.ParseUUID(rawID)
		if err != nil {
			continue
		}
		tid, _ := database.ParseNullUUID(rawTID)
		sid, _ := database.ParseNullUUID(rawSID)
		occAt, _ := database.ParseTime(occVal)
		items = append(items, models.ActivityOut{
			ID:                 id,
			ActionType:         actionType,
			TopicID:            tid,
			SkillID:            sid,
			TopicNameSnapshot:  database.ParseNullString(tSnap),
			SkillNameSnapshot:  database.ParseNullString(sSnap),
			ParentNameSnapshot: database.ParseNullString(pSnap),
			Summary:            database.ParseNullString(summary),
			OccurredAt:         occAt,
			ActivityDate:       database.ParseDateString(dVal),
		})
	}
	core.WriteJSON(w, http.StatusOK, items)
}
