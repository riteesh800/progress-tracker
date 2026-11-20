import { useState, useId } from "react";
import type { GoalActivity } from "../../api/goals.api";

interface ActivityCardsProps {
  activities: GoalActivity[];
  isDeadlineReached: boolean;
  onAddActivity: (text: string) => Promise<void>;
  isAdding: boolean;
  addError?: string;
  totalActivities: number;
  page: number;
  perPage: number;
  onPageChange: (newPage: number) => void;
}

export function ActivityCards({
  activities,
  isDeadlineReached,
  onAddActivity,
  isAdding,
  addError,
  totalActivities,
  page,
  perPage,
  onPageChange,
}: ActivityCardsProps) {
  const [inputText, setInputText] = useState("");
  const [localError, setLocalError] = useState("");
  const [activeModalText, setActiveModalText] = useState<{ text: string; date: string } | null>(null);

  const inputId = useId();
  const maxChars = 50;
  const remainingChars = maxChars - inputText.length;

  const handleSubmit = async (e: React.FormEvent) => {
    e.preventDefault();
    const trimmed = inputText.trim();
    if (!trimmed) {
      setLocalError("Please enter an activity description.");
      return;
    }
    if (trimmed.length > maxChars) {
      setLocalError(`Activity text must not exceed ${maxChars} characters.`);
      return;
    }

    setLocalError("");
    try {
      await onAddActivity(trimmed);
      setInputText("");
    } catch {
      // Parent handles addError
    }
  };

  const totalPages = Math.ceil(totalActivities / perPage) || 1;

  // Format date nicely (e.g. Nov 20, 2026)
  const formatCardDate = (dateStr: string) => {
    if (!dateStr) return "";
    const parts = dateStr.split("-").map(Number);
    if (parts.length === 3 && !isNaN(parts[0]) && !isNaN(parts[1]) && !isNaN(parts[2])) {
      const d = new Date(parts[0], parts[1] - 1, parts[2]);
      return d.toLocaleDateString(undefined, { month: "short", day: "numeric", year: "numeric" });
    }
    return dateStr;
  };

  return (
    <section className="goal-activity-section" aria-label="What I Did Today Section">
      <div className="goal-activity-header">
        <div className="goal-activity-title-wrap">
          <h2 className="goal-activity-heading">
            <span aria-hidden>📝</span> What I Did Today
          </h2>
          <p className="muted goal-activity-subheading">
            Record every concrete step taken toward achieving your goal ({totalActivities} recorded so far)
          </p>
        </div>
      </div>

      {/* Activity Input Bar */}
      {!isDeadlineReached ? (
        <form className="goal-activity-form" onSubmit={handleSubmit}>
          <div className="goal-activity-input-group">
            <label htmlFor={inputId} className="sr-only">
              What did you do today?
            </label>
            <input
              id={inputId}
              type="text"
              className="goal-activity-input"
              placeholder="What did you do today? (e.g. Read 2 chapters of Raft 📖)"
              value={inputText}
              onChange={(e) => {
                setInputText(e.target.value.slice(0, maxChars));
                if (localError) setLocalError("");
              }}
              maxLength={maxChars}
              disabled={isAdding}
              autoComplete="off"
            />
            <span
              className={`goal-activity-counter ${remainingChars < 10 ? "warning" : ""}`}
              aria-live="polite"
            >
              {remainingChars} / {maxChars}
            </span>
          </div>

          <button
            type="submit"
            className="primary goal-activity-submit-btn"
            disabled={isAdding || !inputText.trim()}
          >
            {isAdding ? "Recording…" : "Add Activity"}
          </button>
        </form>
      ) : (
        <div className="goal-activity-expired-banner">
          <span aria-hidden>🔒</span>
          <span>Goal deadline has been reached. New activities can no longer be added.</span>
        </div>
      )}

      {(localError || addError) && (
        <p className="error goal-activity-error" role="alert">
          {localError || addError}
        </p>
      )}

      {/* Activity Cards 7-Per-Row Grid */}
      {activities.length === 0 ? (
        <div className="goal-activity-empty-state card">
          <span className="goal-activity-empty-icon" aria-hidden>
            🎯
          </span>
          <h3>No activities recorded yet</h3>
          <p className="muted">
            Add your first action item above. Every day you work on this goal builds your momentum score!
          </p>
        </div>
      ) : (
        <div className="goal-activity-cards-wrap">
          <div className="goal-activity-grid">
            {activities.map((act) => (
              <div
                key={act.id}
                className="goal-activity-card-container"
                onClick={() => setActiveModalText({ text: act.activity_text, date: act.activity_date })}
              >
                <div
                  className="goal-activity-card"
                  title={`${act.activity_text} (${formatCardDate(act.activity_date)})`}
                >
                  <p className="goal-activity-card-text">{act.activity_text}</p>
                </div>
                <span className="goal-activity-card-date">
                  {formatCardDate(act.activity_date)}
                </span>
              </div>
            ))}
          </div>

          {/* Pagination when activities > perPage */}
          {totalPages > 1 && (
            <div className="goal-activity-pagination">
              <button
                type="button"
                className="secondary"
                disabled={page <= 1}
                onClick={() => onPageChange(page - 1)}
              >
                Previous
              </button>
              <span className="muted">
                Page {page} of {totalPages}
              </span>
              <button
                type="button"
                className="secondary"
                disabled={page >= totalPages}
                onClick={() => onPageChange(page + 1)}
              >
                Next
              </button>
            </div>
          )}
        </div>
      )}

      {/* Detail Popover Modal if text clicked */}
      {activeModalText && (
        <div
          className="goal-activity-detail-overlay"
          onClick={() => setActiveModalText(null)}
          role="dialog"
          aria-modal="true"
        >
          <div
            className="goal-activity-detail-modal card"
            onClick={(e) => e.stopPropagation()}
          >
            <h3>Activity Detail</h3>
            <p className="goal-activity-modal-text">{activeModalText.text}</p>
            <p className="muted">Recorded on: {formatCardDate(activeModalText.date)}</p>
            <button
              type="button"
              className="primary"
              onClick={() => setActiveModalText(null)}
            >
              Close
            </button>
          </div>
        </div>
      )}
    </section>
  );
}
