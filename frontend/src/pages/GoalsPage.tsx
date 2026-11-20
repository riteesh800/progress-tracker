import { useState, useEffect } from "react";
import { useNavigate } from "react-router-dom";
import { useQuery, useMutation, useQueryClient } from "@tanstack/react-query";
import { listGoals, createGoal, deleteGoal, type Goal } from "../api/goals.api";
import { ApiError } from "../api/client";
import ConfirmDialog from "../components/ConfirmDialog";

const getMinDateTime = () => {
  const now = new Date();
  const pad = (n: number) => String(n).padStart(2, "0");
  return `${now.getFullYear()}-${pad(now.getMonth() + 1)}-${pad(now.getDate())}T${pad(now.getHours())}:${pad(now.getMinutes())}`;
};

export default function GoalsPage() {
  const navigate = useNavigate();
  const qc = useQueryClient();
  const q = useQuery({ queryKey: ["goals"], queryFn: listGoals });

  const [modalOpen, setModalOpen] = useState(false);
  const [minDateTime, setMinDateTime] = useState("");
  const [goalName, setGoalName] = useState("");
  const [deadlineAt, setDeadlineAt] = useState("");
  const [motivationLine, setMotivationLine] = useState("");
  const [formError, setFormError] = useState("");
  const [pendingDelete, setPendingDelete] = useState<{ id: string; name: string } | null>(null);

  const openModal = () => {
    setMinDateTime(getMinDateTime());
    setFormError("");
    setModalOpen(true);
  };

  const closeModal = () => {
    setModalOpen(false);
    setFormError("");
    setMinDateTime("");
  };

  useEffect(() => {
    if (modalOpen && !minDateTime) {
      setMinDateTime(getMinDateTime());
    }
  }, [modalOpen, minDateTime]);

  const isPastDeadline = (val: string) => {
    if (!val) return false;
    const d = new Date(val);
    if (isNaN(d.getTime())) return false;
    const now = new Date();
    now.setSeconds(0, 0);
    return d.getTime() < now.getTime() - 60 * 1000;
  };

  const createMut = useMutation({
    mutationFn: createGoal,
    onSuccess: () => {
      closeModal();
      setGoalName("");
      setDeadlineAt("");
      setMotivationLine("");
      qc.invalidateQueries({ queryKey: ["goals"] });
    },
    onError: (err) => {
      setFormError(err instanceof ApiError ? err.message : "Failed to create goal. Please check inputs.");
    },
  });

  const deleteMut = useMutation({
    mutationFn: (id: string) => deleteGoal(id),
    onSuccess: () => {
      setPendingDelete(null);
      qc.invalidateQueries({ queryKey: ["goals"] });
      qc.invalidateQueries({ queryKey: ["goals-trash"] });
    },
  });

  const handleCreateSubmit = (e: React.FormEvent) => {
    e.preventDefault();
    const trimmedName = goalName.trim();
    if (!trimmedName) {
      setFormError("Goal name is required.");
      return;
    }
    if (trimmedName.length > 100) {
      setFormError("Goal name must be 100 characters or fewer.");
      return;
    }
    if (!deadlineAt) {
      setFormError("Please select a complete deadline (date and time).");
      return;
    }
    const d = new Date(deadlineAt);
    if (isNaN(d.getTime())) {
      setFormError("Invalid deadline date and time.");
      return;
    }
    if (isPastDeadline(deadlineAt)) {
      setFormError("Deadline must be today or in the future (cannot be in the past or earlier than current time).");
      return;
    }
    if (motivationLine.trim().length > 200) {
      setFormError("Motivation line must be 200 characters or fewer.");
      return;
    }

    setFormError("");
    // Detect user's local timezone
    const userTimezone = Intl.DateTimeFormat().resolvedOptions().timeZone || "UTC";

    createMut.mutate({
      goal_name: trimmedName,
      deadline_at: d.toISOString(),
      timezone: userTimezone,
      motivation_line: motivationLine.trim() || undefined,
    });
  };

  const formatDeadlineBadge = (iso: string) => {
    const d = new Date(iso);
    if (isNaN(d.getTime())) return iso;
    return d.toLocaleString(undefined, {
      dateStyle: "medium",
      timeStyle: "short",
    });
  };

  if (q.isLoading) return <p>Loading goals…</p>;
  if (q.isError) return <p className="error">Could not load goals. Please refresh.</p>;

  return (
    <div className="goals-page-wrap">
      <div className="goals-header-bar">
        <div>
          <h1>Goals</h1>
          <p className="muted">
            Set ambitious targets, track daily consistency, and race against the live countdown clock.
          </p>
        </div>
        <button
          type="button"
          className="primary goals-create-btn"
          onClick={openModal}
        >
          + Create Goal
        </button>
      </div>

      {q.data?.length === 0 ? (
        <div className="card goals-empty-card">
          <div className="goals-empty-icon" aria-hidden>
            🎯
          </div>
          <h2>No goals yet</h2>
          <p className="muted">
            Create your first goal with a target deadline and motivation line to start tracking daily consistency!
          </p>
          <button
            type="button"
            className="primary"
            onClick={openModal}
            style={{ marginTop: "1rem" }}
          >
            Create Your First Goal
          </button>
        </div>
      ) : (
        <div className="goals-grid">
          {q.data?.map((g: Goal) => {
            const isDeadlinePassed = new Date(g.deadline_at).getTime() <= Date.now();

            return (
              <div
                key={g.id}
                className="card goal-card"
                onClick={() => navigate(`/goals/${g.id}`)}
                role="button"
                tabIndex={0}
                onKeyDown={(e) => {
                  if (e.target === e.currentTarget && (e.key === "Enter" || e.key === " ")) {
                    e.preventDefault();
                    navigate(`/goals/${g.id}`);
                  }
                }}
              >
                <div className="goal-card-header">
                  <div className="goal-card-title-wrap">
                    <span className="goal-card-name" title={g.goal_name}>
                      {g.goal_name}
                    </span>
                    {isDeadlinePassed ? (
                      <span className="goal-badge reached">🏁 Deadline Reached</span>
                    ) : (
                      <span className="goal-badge active">⏳ Active</span>
                    )}
                  </div>
                </div>

                {g.motivation_line && (
                  <p className="goal-card-motivation">
                    &ldquo;{g.motivation_line}&rdquo;
                  </p>
                )}

                <div className="goal-card-meta">
                  <div className="goal-card-meta-item">
                    <span className="muted">Deadline:</span>
                    <strong>{formatDeadlineBadge(g.deadline_at)}</strong>
                  </div>
                  <div className="goal-card-meta-item">
                    <span className="muted">Activities:</span>
                    <strong>{g.total_activities} recorded</strong>
                  </div>
                </div>

                <div className="goal-card-actions">
                  <button
                    type="button"
                    className="goal-card-del-btn"
                    title="Move to Trash"
                    onClick={(e) => {
                      e.stopPropagation();
                      setPendingDelete({ id: g.id, name: g.goal_name });
                    }}
                    onKeyDown={(e) => {
                      e.stopPropagation();
                    }}
                  >
                    Delete
                  </button>
                </div>
              </div>
            );
          })}
        </div>
      )}

      {/* Create Goal Modal */}
      {modalOpen && (
        <div
          className="goal-modal-overlay"
          onClick={closeModal}
          role="dialog"
          aria-modal="true"
        >
          <div className="card goal-modal" onClick={(e) => e.stopPropagation()}>
            <div className="goal-modal-header">
              <h2>Create New Goal</h2>
              <button
                type="button"
                className="goal-modal-close"
                onClick={closeModal}
                aria-label="Close modal"
              >
                ✕
              </button>
            </div>

            <form onSubmit={handleCreateSubmit} className="goal-form">
              {formError && <p className="error" role="alert">{formError}</p>}

              {/* Input 1: Goal name (required, max 100 chars) */}
              <div className="field">
                <label htmlFor="goal-name-input">
                  Goal Name <span className="required">*</span>
                </label>
                <input
                  id="goal-name-input"
                  type="text"
                  placeholder="e.g. Master Go & Distributed Architecture"
                  value={goalName}
                  onChange={(e) => setGoalName(e.target.value.slice(0, 100))}
                  maxLength={100}
                  required
                  autoFocus
                />
              </div>

              {/* Input 2: Deadline (complete date and time picker) */}
              <div className="field">
                <label htmlFor="goal-deadline-input">
                  Deadline (Date &amp; Time) <span className="required">*</span>
                </label>
                <input
                  id="goal-deadline-input"
                  type="datetime-local"
                  min={minDateTime || getMinDateTime()}
                  value={deadlineAt}
                  onChange={(e) => {
                    const val = e.target.value;
                    setDeadlineAt(val);
                    if (val && isPastDeadline(val)) {
                      setFormError("Deadline must be today or in the future (cannot be in the past or earlier than current time).");
                    } else if (formError.includes("Deadline")) {
                      setFormError("");
                    }
                  }}
                  required
                />
                <span className="muted field-hint">
                  Select a deadline date and time today or in the future
                </span>
              </div>

              {/* Input 3: Motivation line (optional, max 200 chars) */}
              <div className="field">
                <label htmlFor="goal-motivation-input">
                  Motivation Line <span className="muted">(Optional)</span>
                </label>
                <input
                  id="goal-motivation-input"
                  type="text"
                  placeholder="e.g. Consistency beats intensity every single day."
                  value={motivationLine}
                  onChange={(e) => setMotivationLine(e.target.value.slice(0, 200))}
                  maxLength={200}
                />
              </div>

              <div className="goal-modal-actions">
                <button
                  type="button"
                  className="secondary"
                  onClick={closeModal}
                  disabled={createMut.isPending}
                >
                  Cancel
                </button>
                <button
                  type="submit"
                  className="primary"
                  disabled={createMut.isPending || !goalName.trim() || !deadlineAt || isPastDeadline(deadlineAt)}
                >
                  {createMut.isPending ? "Creating Goal…" : "Create Goal"}
                </button>
              </div>
            </form>
          </div>
        </div>
      )}

      {/* Confirm Delete Dialog (Moves to Trash) */}
      <ConfirmDialog
        open={pendingDelete !== null}
        title="Move Goal to Trash"
        body={`Are you sure you want to delete "${pendingDelete?.name}"? It will be kept in your Trash for 3 days before being permanently removed.`}
        confirmLabel={deleteMut.isPending ? "Moving to Trash…" : "Move to Trash"}
        danger
        busy={deleteMut.isPending}
        onCancel={() => setPendingDelete(null)}
        onConfirm={() => {
          if (pendingDelete) {
            deleteMut.mutate(pendingDelete.id);
          }
        }}
      />
    </div>
  );
}
