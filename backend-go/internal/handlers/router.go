package handlers

import (
	"net/http"

	"skill-progress-tracker/backend-go/internal/config"
	"skill-progress-tracker/backend-go/internal/core"
	"skill-progress-tracker/backend-go/internal/database"
	"skill-progress-tracker/backend-go/internal/middleware"
)

func NewRouter(cfg *config.Settings, db *database.DB, limiter *core.RateLimiter) http.Handler {
	s := NewServer(cfg, db, limiter)
	mux := http.NewServeMux()

	// Health check
	mux.HandleFunc("GET /health", func(w http.ResponseWriter, r *http.Request) {
		var one int
		if err := db.QueryRow(r.Context(), nil, "SELECT 1").Scan(&one); err != nil {
			core.WriteError(w, http.StatusInternalServerError, "internal_error", "Database health check failed.")
			return
		}
		core.WriteJSON(w, http.StatusOK, map[string]any{"status": "ok"})
	})

	// Auth routes
	mux.HandleFunc("POST /auth/register", s.Register)
	mux.HandleFunc("POST /auth/login", s.Login)
	mux.HandleFunc("POST /auth/refresh", s.Refresh)
	mux.HandleFunc("POST /auth/logout", s.Logout)
	mux.HandleFunc("POST /auth/google", s.GoogleAuth)
	mux.HandleFunc("POST /auth/forgot-password", s.ForgotPassword)
	mux.HandleFunc("POST /auth/verify-reset-code", s.VerifyResetCode)
	mux.HandleFunc("POST /auth/reset-password", s.ResetPassword)
	mux.HandleFunc("POST /auth/change-password", middleware.RequireAuth(cfg, db, s.ChangePassword))
	mux.HandleFunc("GET /auth/me", middleware.RequireAuth(cfg, db, s.Me))
	mux.HandleFunc("PATCH /auth/me", middleware.RequireAuth(cfg, db, s.UpdateMe))
	mux.HandleFunc("DELETE /auth/me", middleware.RequireAuth(cfg, db, s.DeleteMe))

	// Admin routes
	mux.HandleFunc("POST /admin/unlock", middleware.RequireAdminAccount(cfg, db, s.UnlockAdmin))
	mux.HandleFunc("GET /admin/users", middleware.RequireVerifiedAdmin(cfg, db, s.ListUsers))
	mux.HandleFunc("POST /admin/users/{user_id}/password", middleware.RequireVerifiedAdmin(cfg, db, s.AdminResetUserPassword))
	mux.HandleFunc("POST /admin/cleanup/expired-accounts", middleware.RequireVerifiedAdmin(cfg, db, s.CleanupExpiredAccounts))
	mux.HandleFunc("GET /admin/cron/cleanup", s.CronCleanupExpiredAccounts)
	mux.HandleFunc("POST /admin/cron/cleanup", s.CronCleanupExpiredAccounts)

	// Skills routes
	mux.HandleFunc("GET /skills", middleware.RequireAuth(cfg, db, s.ListSkills))
	mux.HandleFunc("GET /skills/trash", middleware.RequireAuth(cfg, db, s.ListDeletedSkills))
	mux.HandleFunc("POST /skills", middleware.RequireAuth(cfg, db, s.CreateSkill))
	mux.HandleFunc("GET /skills/{skill_id}", middleware.RequireAuth(cfg, db, s.GetSkill))
	mux.HandleFunc("GET /skills/{skill_id}/topics", middleware.RequireAuth(cfg, db, s.GetSkillTree))
	mux.HandleFunc("GET /skills/{skill_id}/tree", middleware.RequireAuth(cfg, db, s.GetSkillTree))
	mux.HandleFunc("PATCH /skills/{skill_id}", middleware.RequireAuth(cfg, db, s.UpdateSkill))
	mux.HandleFunc("GET /skills/{skill_id}/delete-impact", middleware.RequireAuth(cfg, db, s.SkillDeleteImpact))
	mux.HandleFunc("DELETE /skills/{skill_id}", middleware.RequireAuth(cfg, db, s.DeleteSkill))
	mux.HandleFunc("POST /skills/{skill_id}/restore", middleware.RequireAuth(cfg, db, s.RestoreSkill))
	mux.HandleFunc("DELETE /skills/{skill_id}/permanent", middleware.RequireAuth(cfg, db, s.PurgeSkill))
	mux.HandleFunc("DELETE /skills/{skill_id}/purge", middleware.RequireAuth(cfg, db, s.PurgeSkill))

	// Skill Timer routes
	mux.HandleFunc("GET /skills/{skill_id}/timer", middleware.RequireAuth(cfg, db, s.GetTimer))
	mux.HandleFunc("POST /skills/{skill_id}/timer", middleware.RequireAuth(cfg, db, s.UpsertTimer))
	mux.HandleFunc("PUT /skills/{skill_id}/timer", middleware.RequireAuth(cfg, db, s.UpsertTimer))
	mux.HandleFunc("DELETE /skills/{skill_id}/timer", middleware.RequireAuth(cfg, db, s.DeleteTimer))

	// Goals routes
	mux.HandleFunc("GET /goals", middleware.RequireAuth(cfg, db, s.ListGoals))
	mux.HandleFunc("POST /goals", middleware.RequireAuth(cfg, db, s.CreateGoal))
	mux.HandleFunc("GET /goals/trash", middleware.RequireAuth(cfg, db, s.ListDeletedGoals))
	mux.HandleFunc("GET /goals/{goal_id}", middleware.RequireAuth(cfg, db, s.GetGoal))
	mux.HandleFunc("DELETE /goals/{goal_id}", middleware.RequireAuth(cfg, db, s.DeleteGoal))
	mux.HandleFunc("POST /goals/{goal_id}/restore", middleware.RequireAuth(cfg, db, s.RestoreGoal))
	mux.HandleFunc("DELETE /goals/{goal_id}/permanent", middleware.RequireAuth(cfg, db, s.PurgeGoal))
	mux.HandleFunc("DELETE /goals/{goal_id}/purge", middleware.RequireAuth(cfg, db, s.PurgeGoal))
	mux.HandleFunc("GET /goals/{goal_id}/activities", middleware.RequireAuth(cfg, db, s.ListGoalActivities))
	mux.HandleFunc("POST /goals/{goal_id}/activities", middleware.RequireAuth(cfg, db, s.AddGoalActivity))
	mux.HandleFunc("GET /goals/{goal_id}/progress", middleware.RequireAuth(cfg, db, s.GetGoalProgress))

	// Topics routes
	mux.HandleFunc("POST /topics", middleware.RequireAuth(cfg, db, s.CreateTopic))
	mux.HandleFunc("GET /topics/{topic_id}", middleware.RequireAuth(cfg, db, s.GetTopic))
	mux.HandleFunc("GET /topics/{topic_id}/children", middleware.RequireAuth(cfg, db, s.TopicChildren))
	mux.HandleFunc("POST /topics/{topic_id}/complete", middleware.RequireAuth(cfg, db, s.CompleteTopic))

	// Notes routes
	mux.HandleFunc("GET /notes", middleware.RequireAuth(cfg, db, s.ListNotes))
	mux.HandleFunc("POST /notes", middleware.RequireAuth(cfg, db, s.CreateNote))
	mux.HandleFunc("GET /notes/{note_id}", middleware.RequireAuth(cfg, db, s.GetNote))
	mux.HandleFunc("PATCH /notes/{note_id}", middleware.RequireAuth(cfg, db, s.UpdateNote))
	mux.HandleFunc("DELETE /notes/{note_id}", middleware.RequireAuth(cfg, db, s.DeleteNote))

	// Personal Notes routes
	mux.HandleFunc("GET /personal-notes", middleware.RequireAuth(cfg, db, s.ListPersonalNotes))
	mux.HandleFunc("POST /personal-notes", middleware.RequireAuth(cfg, db, s.CreatePersonalNote))
	mux.HandleFunc("GET /personal-notes/{note_id}", middleware.RequireAuth(cfg, db, s.GetPersonalNote))
	mux.HandleFunc("PATCH /personal-notes/{note_id}", middleware.RequireAuth(cfg, db, s.UpdatePersonalNote))
	mux.HandleFunc("DELETE /personal-notes/{note_id}", middleware.RequireAuth(cfg, db, s.DeletePersonalNote))

	// Search route
	mux.HandleFunc("GET /search", middleware.RequireAuth(cfg, db, s.Search))

	// Dashboard & Activity routes
	mux.HandleFunc("GET /dashboard", middleware.RequireAuth(cfg, db, s.Dashboard))
	mux.HandleFunc("GET /dashboard/suggested-next", middleware.RequireAuth(cfg, db, s.SuggestedNext))
	mux.HandleFunc("GET /suggested-next", middleware.RequireAuth(cfg, db, s.SuggestedNext))
	mux.HandleFunc("GET /streak", middleware.RequireAuth(cfg, db, s.Streak))
	mux.HandleFunc("GET /streak/calendar", middleware.RequireAuth(cfg, db, s.StreakCalendar))
	mux.HandleFunc("GET /activity", middleware.RequireAuth(cfg, db, s.Activity))

	// PDF routes
	mux.HandleFunc("POST /pdf/upload-url", middleware.RequireAuth(cfg, db, s.CreatePDFUploadURL))
	mux.HandleFunc("PUT /pdf/storage-upload", middleware.RequireAuth(cfg, db, s.UploadToSignedStorage))
	mux.HandleFunc("POST /pdf/upload-complete", middleware.RequireAuth(cfg, db, s.CompletePDFUpload))
	mux.HandleFunc("POST /pdf/upload", middleware.RequireAuth(cfg, db, s.UploadPDF))
	mux.HandleFunc("GET /pdf/jobs/{job_id}", middleware.RequireAuth(cfg, db, s.GetJob))
	mux.HandleFunc("PATCH /pdf/jobs/{job_id}/tree", middleware.RequireAuth(cfg, db, s.PatchJob))
	mux.HandleFunc("PATCH /pdf/jobs/{job_id}", middleware.RequireAuth(cfg, db, s.PatchJob))
	mux.HandleFunc("POST /pdf/jobs/{job_id}/confirm", middleware.RequireAuth(cfg, db, s.ConfirmJob))

	// AI routes
	mux.HandleFunc("POST /ai/generate-syllabus", middleware.RequireAuth(cfg, db, s.GenerateSyllabus))
	mux.HandleFunc("POST /ai/refine-syllabus", middleware.RequireAuth(cfg, db, s.RefineSyllabus))
	mux.HandleFunc("POST /ai/confirm-syllabus", middleware.RequireAuth(cfg, db, s.ConfirmSyllabus))

	// Middleware chain matching FastAPI main.py:
	// StripApiPrefix -> GZip -> CORS -> ErrorRecoveryAndLogging -> SecurityHeaders -> mux
	var handler http.Handler = mux
	handler = middleware.SecurityHeaders(cfg, handler)
	handler = middleware.ErrorRecoveryAndLogging(handler)
	handler = middleware.CORS(cfg, handler)
	handler = middleware.GZip(handler)
	handler = middleware.StripAPIPrefix(handler)

	return handler
}
