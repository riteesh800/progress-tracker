package services

import (
	"time"

	"skill-progress-tracker/backend-go/internal/models"
)

// GoalProgressResult holds calculated consistency and activity statistics for a goal.
type GoalProgressResult struct {
	Points            []models.GoalProgressPoint
	CurrentScore      float64
	WorkedDays        int
	MissedDays        int
	TotalTrackedDays  int
	IsDeadlineReached bool
	InactivityWarning bool
}

// ComputeGoalProgress calculates cumulative consistency scores and daily statistics for a goal.
//
// Calculation rules:
// - If user records at least one activity on a calendar day: +1 point.
// - If user records no activities on a completed calendar day: -1 point.
// - Multiple cards on the same day must not increase the graph more than once (+1 point).
// - Do not penalize the current day before it is completed (0 point penalty).
// - If the current day has an activity, reflect the +1 point immediately.
// - Starts from the goal's creation date.
// - Continue through the current date while active; finish at deadline date once deadline is reached.
// - Inactivity warning: True when user has not recorded progress for 3 consecutive completed days.
func ComputeGoalProgress(goal *models.Goal, activityDates map[string]int, now time.Time) GoalProgressResult {
	loc, err := time.LoadLocation(goal.Timezone)
	if err != nil {
		loc = time.UTC
	}

	nowInTZ := now.In(loc)
	todayStr := nowInTZ.Format("2006-01-02")
	deadlineInTZ := goal.DeadlineAt.In(loc)
	deadlineStr := deadlineInTZ.Format("2006-01-02")
	createdInTZ := goal.CreatedAt.In(loc)
	startStr := createdInTZ.Format("2006-01-02")

	isDeadlineReached := !now.Before(goal.DeadlineAt)

	// Determine end date for the graph:
	// While active: today
	// Once deadline reached: deadline date
	endStr := todayStr
	if isDeadlineReached {
		endStr = deadlineStr
	}

	// Guard: if startStr > endStr (e.g. deadline was set in past or clock variance), clamp endStr to startStr
	if startStr > endStr {
		endStr = startStr
	}

	// Parse start and end as midnight in goal's timezone
	startDate, _ := time.ParseInLocation("2006-01-02", startStr, loc)
	endDate, _ := time.ParseInLocation("2006-01-02", endStr, loc)

	var points []models.GoalProgressPoint
	currentScore := 0.0
	workedDays := 0
	missedDays := 0

	// Track points day by day
	curr := startDate
	for !curr.After(endDate) {
		dateStr := curr.Format("2006-01-02")
		count := activityDates[dateStr]
		hasActivity := count > 0

		isCurrentDay := (dateStr == todayStr) && !isDeadlineReached

		var dailyChange float64
		if hasActivity {
			dailyChange = 1.0
			workedDays++
		} else {
			if isCurrentDay {
				// Current unfinished day is not penalized
				dailyChange = 0.0
			} else {
				// Completed day with no activity
				dailyChange = -1.0
				missedDays++
			}
		}

		currentScore += dailyChange

		points = append(points, models.GoalProgressPoint{
			Date:         dateStr,
			HasActivity:  hasActivity,
			DailyChange:  dailyChange,
			Score:        currentScore,
			IsCurrentDay: isCurrentDay,
		})

		curr = curr.AddDate(0, 0, 1)
	}

	totalTrackedDays := workedDays + missedDays

	// 3-Day Inactivity Warning calculation:
	// - Must not show if activity recorded today
	// - Must check 3 consecutive completed days (yesterday, day - 2, day - 3)
	// - All 3 completed days must be on or after startStr
	// - If any of those 3 days has an activity, warning is false
	inactivityWarning := false
	if !isDeadlineReached {
		hasActivityToday := activityDates[todayStr] > 0
		if !hasActivityToday {
			y1 := nowInTZ.AddDate(0, 0, -1).Format("2006-01-02")
			y2 := nowInTZ.AddDate(0, 0, -2).Format("2006-01-02")
			y3 := nowInTZ.AddDate(0, 0, -3).Format("2006-01-02")

			// Check if at least 3 completed days have elapsed since goal creation
			if y3 >= startStr {
				if activityDates[y1] == 0 && activityDates[y2] == 0 && activityDates[y3] == 0 {
					inactivityWarning = true
				}
			}
		}
	}

	return GoalProgressResult{
		Points:            points,
		CurrentScore:      currentScore,
		WorkedDays:        workedDays,
		MissedDays:        missedDays,
		TotalTrackedDays:  totalTrackedDays,
		IsDeadlineReached: isDeadlineReached,
		InactivityWarning: inactivityWarning,
	}
}
