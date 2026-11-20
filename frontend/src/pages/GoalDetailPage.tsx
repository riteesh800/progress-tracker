import { useState, useCallback } from "react";
import { useParams, Link, useNavigate } from "react-router-dom";
import { useQuery, useMutation, useQueryClient } from "@tanstack/react-query";
import {
  getGoal,
  listGoalActivities,
  getGoalProgress,
  addGoalActivity,
  deleteGoal,
} from "../api/goals.api";
import { LiveClock } from "../components/goals/LiveClock";
import { CountdownTimer } from "../components/goals/CountdownTimer";
import { InactivityWarning } from "../components/goals/InactivityWarning";
import { ActivityCards } from "../components/goals/ActivityCards";
import { GoalProgressGraph } from "../components/goals/GoalProgressGraph";
import ConfirmDialog from "../components/ConfirmDialog";
import { ApiError } from "../api/client";

export default function GoalDetailPage() {
  const { id } = useParams<{ id: string }>();
  const navigate = useNavigate();
  const qc = useQueryClient();

  const [page, setPage] = useState(1);
  const perPage = 50;
  const [addError, setAddError] = useState("");
  const [deleteConfirmOpen, setDeleteConfirmOpen] = useState(false);

  // Queries
  const goalQuery = useQuery({
    queryKey: ["goal", id],
    queryFn: () => (id ? getGoal(id) : Promise.reject("no id")),
    enabled: Boolean(id),
  });

  const activitiesQuery = useQuery({
    queryKey: ["goal-activities", id, page],
    queryFn: () => (id ? listGoalActivities(id, page, perPage) : Promise.reject("no id")),
    enabled: Boolean(id),
  });

  const progressQuery = useQuery({
    queryKey: ["goal-progress", id],
    queryFn: () => (id ? getGoalProgress(id) : Promise.reject("no id")),
    enabled: Boolean(id),
  });

  // Mutations
  const addActivityMut = useMutation({
    mutationFn: (text: string) => {
      if (!id) throw new Error("Missing goal id");
      return addGoalActivity(id, text);
    },
    onSuccess: () => {
      setAddError("");
      qc.invalidateQueries({ queryKey: ["goal", id] });
      qc.invalidateQueries({ queryKey: ["goal-activities", id] });
      qc.invalidateQueries({ queryKey: ["goal-progress", id] });
      qc.invalidateQueries({ queryKey: ["goals"] });
    },
    onError: (err) => {
      setAddError(err instanceof ApiError ? err.message : "Failed to add activity. Try again.");
    },
  });

  const deleteMut = useMutation({
    mutationFn: () => {
      if (!id) throw new Error("Missing goal id");
      return deleteGoal(id);
    },
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: ["goals"] });
      qc.invalidateQueries({ queryKey: ["goals-trash"] });
      navigate("/goals");
    },
  });

  const handleDeadlineReached = useCallback(() => {
    qc.invalidateQueries({ queryKey: ["goal", id] });
    qc.invalidateQueries({ queryKey: ["goal-progress", id] });
  }, [qc, id]);

  if (goalQuery.isLoading) return <p>Loading goal details…</p>;
  if (goalQuery.isError || !goalQuery.data) {
    return (
      <div className="card">
        <h2>Goal Not Found</h2>
        <p className="muted">This goal could not be found or you do not have permission to view it.</p>
        <Link to="/goals" className="button primary" style={{ marginTop: "1rem", display: "inline-block" }}>
          ← Back to Goals
        </Link>
      </div>
    );
  }

  const { goal, summary, inactivity_warning, is_deadline_reached } = goalQuery.data;

  // Format deadline date for header
  const formattedDeadline = new Date(goal.deadline_at).toLocaleString(undefined, {
    dateStyle: "full",
    timeStyle: "short",
  });

  const activities = activitiesQuery.data?.activities || [];
  const totalActivities = activitiesQuery.data?.total || 0;
  const progressData = progressQuery.data;

  return (
    <div className="goal-detail-page">
      {/* Top Navigation & Breadcrumb */}
      <div className="goal-detail-nav">
        <Link to="/goals" className="goal-back-link">
          ← Back to Goals
        </Link>
        <button
          type="button"
          className="secondary goal-delete-top-btn"
          onClick={() => setDeleteConfirmOpen(true)}
          title="Move goal to Trash"
        >
          🗑️ Move to Trash
        </button>
      </div>

      {/* Main Goal Heading */}
      <header className="goal-detail-header">
        <div className="goal-detail-heading-wrap">
          <h1 className="goal-detail-title">{goal.goal_name}</h1>
          {goal.motivation_line && (
            <p className="goal-detail-motivation">
              &ldquo;{goal.motivation_line}&rdquo;
            </p>
          )}
        </div>

        <div className="goal-detail-deadline-chip">
          <span className="muted">Target Deadline:</span>
          <strong>{formattedDeadline}</strong>
          <span className="goal-detail-tz">({goal.timezone})</span>
        </div>
      </header>

      {/* Three-Day Inactivity Warning */}
      <InactivityWarning show={inactivity_warning} />

      {/* Deadline Reached Completion Summary Banner (Section 7) */}
      {is_deadline_reached && (
        <div className="card goal-completion-banner" role="status">
          <div className="goal-completion-header">
            <span className="goal-completion-icon" aria-hidden>
              🏆
            </span>
            <div>
              <h2>Your deadline has been reached!</h2>
              <p className="muted">Here is your complete study consistency summary for this goal.</p>
            </div>
          </div>

          <div className="goal-completion-stats-grid">
            <div className="goal-completion-stat-card worked">
              <span className="goal-stat-label">Days Worked</span>
              <strong className="goal-stat-val">{summary.worked_days}</strong>
              <span className="goal-stat-sub">distinct calendar days</span>
            </div>
            <div className="goal-completion-stat-card missed">
              <span className="goal-stat-label">Days Missed</span>
              <strong className="goal-stat-val">{summary.missed_days}</strong>
              <span className="goal-stat-sub">elapsed days with no activity</span>
            </div>
            <div className="goal-completion-stat-card total">
              <span className="goal-stat-label">Total Tracked Days</span>
              <strong className="goal-stat-val">{summary.total_days}</strong>
              <span className="goal-stat-sub">full timeline duration</span>
            </div>
            <div className="goal-completion-stat-card score">
              <span className="goal-stat-label">Final Consistency Score</span>
              <strong className="goal-stat-val">
                {summary.current_score > 0
                  ? `+${summary.current_score.toFixed(0)}`
                  : summary.current_score.toFixed(0)}
              </strong>
              <span className="goal-stat-sub">cumulative momentum</span>
            </div>
          </div>
        </div>
      )}

      {/* Centerpiece: Real Live Working Clock and Countdown Timer */}
      <section className="card goal-time-centerpiece" aria-label="Live Clock and Target Countdown">
        <div className="goal-time-clock-area">
          <div className="goal-time-section-label">
            <span>Local Computer Time</span>
          </div>
          <LiveClock />
        </div>

        <div className="goal-time-divider" aria-hidden />

        <div className="goal-time-countdown-area">
          <div className="goal-time-section-label">
            <span>Countdown to Deadline</span>
          </div>
          <CountdownTimer
            deadlineAt={goal.deadline_at}
            onDeadlineReached={handleDeadlineReached}
          />
        </div>
      </section>

      {/* Daily Activity Cards: "What I Did Today" (Section 5) */}
      <ActivityCards
        activities={activities}
        isDeadlineReached={is_deadline_reached}
        onAddActivity={async (text) => {
          await addActivityMut.mutateAsync(text);
        }}
        isAdding={addActivityMut.isPending}
        addError={addError}
        totalActivities={totalActivities}
        page={page}
        perPage={perPage}
        onPageChange={setPage}
      />

      {/* Goal Progress Graph: Daily Consistency (Section 6) */}
      <GoalProgressGraph
        points={progressData?.points || []}
        currentScore={progressData?.current_score || 0}
        workedDays={progressData?.worked_days || 0}
        missedDays={progressData?.missed_days || 0}
        totalTrackedDays={progressData?.total_tracked_days || 0}
        isDeadlineReached={is_deadline_reached}
      />

      {/* Confirm Move to Trash Dialog */}
      <ConfirmDialog
        open={deleteConfirmOpen}
        title="Move Goal to Trash"
        body={`Are you sure you want to delete "${goal.goal_name}"? It will be held in Trash for 3 days before permanent deletion.`}
        confirmLabel={deleteMut.isPending ? "Moving to Trash…" : "Move to Trash"}
        danger
        busy={deleteMut.isPending}
        onCancel={() => setDeleteConfirmOpen(false)}
        onConfirm={() => deleteMut.mutate()}
      />
    </div>
  );
}
