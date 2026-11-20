interface InactivityWarningProps {
  show: boolean;
}

export function InactivityWarning({ show }: InactivityWarningProps) {
  if (!show) return null;

  return (
    <div className="goal-inactivity-warning" role="alert" aria-live="polite">
      <div className="goal-inactivity-pulse" />
      <span className="goal-inactivity-icon" aria-hidden>
        ⚠️
      </span>
      <div className="goal-inactivity-content">
        <strong>Inactivity Alert:</strong> You haven&apos;t done anything for this goal in the last 3 days. Get back on track!
      </div>
    </div>
  );
}
