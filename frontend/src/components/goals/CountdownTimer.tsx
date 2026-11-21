import { useEffect, useState, useMemo } from "react";

export interface CountdownTimerProps {
  deadlineAt: string;
  onDeadlineReached?: () => void;
}

export interface TimeRemaining {
  isReached: boolean;
  years: number;
  months: number;
  days: number;
  hours: number;
  minutes: number;
  seconds: number;
  formattedText: string;
}

export function formatTargetDate(deadlineIso: string): string {
  const d = new Date(deadlineIso);
  if (isNaN(d.getTime())) return "";
  const day = d.getDate();
  const monthNames = ["Jan", "Feb", "Mar", "Apr", "May", "Jun", "Jul", "Aug", "Sep", "Oct", "Nov", "Dec"];
  const month = monthNames[d.getMonth()];
  const year = d.getFullYear();
  let hours = d.getHours();
  const minutes = String(d.getMinutes()).padStart(2, "0");
  const ampm = hours >= 12 ? "pm" : "am";
  hours = hours % 12;
  hours = hours ? hours : 12;
  return `${day} ${month} ${year}, ${hours}:${minutes} ${ampm}`;
}

export function calculateRemaining(deadlineIso: string): TimeRemaining {
  const target = new Date(deadlineIso);
  const now = new Date();
  const diffMs = target.getTime() - now.getTime();

  if (diffMs <= 0 || isNaN(target.getTime())) {
    return {
      isReached: true,
      years: 0,
      months: 0,
      days: 0,
      hours: 0,
      minutes: 0,
      seconds: 0,
      formattedText: "Deadline Reached",
    };
  }

  // Calculate year, month, day difference accurately
  let years = target.getFullYear() - now.getFullYear();
  let months = target.getMonth() - now.getMonth();
  let days = target.getDate() - now.getDate();
  let hours = target.getHours() - now.getHours();
  let minutes = target.getMinutes() - now.getMinutes();
  let seconds = target.getSeconds() - now.getSeconds();

  if (seconds < 0) {
    seconds += 60;
    minutes--;
  }
  if (minutes < 0) {
    minutes += 60;
    hours--;
  }
  if (hours < 0) {
    hours += 24;
    days--;
  }
  if (days < 0) {
    // Days in previous month
    const prevMonthLastDay = new Date(target.getFullYear(), target.getMonth(), 0).getDate();
    days += prevMonthLastDay;
    months--;
  }
  if (months < 0) {
    months += 12;
    years--;
  }

  // Double check non-negativity
  if (years < 0) {
    return {
      isReached: true,
      years: 0,
      months: 0,
      days: 0,
      hours: 0,
      minutes: 0,
      seconds: 0,
      formattedText: "Deadline Reached",
    };
  }

  const pad = (n: number) => String(n).padStart(2, "0");
  const timePart = `${pad(hours)}:${pad(minutes)}:${pad(seconds)}`;

  // Adaptive format: avoid zero-valued larger units
  const parts: string[] = [];
  if (years > 0) {
    parts.push(`${years} ${years === 1 ? "Year" : "Years"}`);
  }
  if (months > 0 || years > 0) {
    if (months > 0) {
      parts.push(`${months} ${months === 1 ? "Month" : "Months"}`);
    }
  }
  if (days > 0 || months > 0 || years > 0) {
    if (days > 0) {
      parts.push(`${days} ${days === 1 ? "Day" : "Days"}`);
    }
  }
  parts.push(timePart);

  return {
    isReached: false,
    years,
    months,
    days,
    hours,
    minutes,
    seconds,
    formattedText: parts.join(" "),
  };
}

export function CountdownTimer({ deadlineAt, onDeadlineReached }: CountdownTimerProps) {
  const [remaining, setRemaining] = useState<TimeRemaining>(() => calculateRemaining(deadlineAt));

  useEffect(() => {
    // Immediate calculation on deadline prop change
    const initial = calculateRemaining(deadlineAt);
    setRemaining(initial);
    if (initial.isReached && onDeadlineReached) {
      onDeadlineReached();
    }

    const timer = setInterval(() => {
      const next = calculateRemaining(deadlineAt);
      setRemaining(next);
      if (next.isReached && onDeadlineReached) {
        onDeadlineReached();
      }
    }, 1000);

    return () => clearInterval(timer);
  }, [deadlineAt, onDeadlineReached]);

  const targetDateStr = useMemo(() => {
    const d = new Date(deadlineAt);
    if (isNaN(d.getTime())) return "";
    return d.toLocaleString(undefined, {
      dateStyle: "medium",
      timeStyle: "short",
    });
  }, [deadlineAt]);

  if (remaining.isReached) {
    return (
      <div className="goal-countdown-box reached">
        <div className="goal-countdown-header">
          <span className="goal-countdown-badge reached">🏁 Deadline Reached</span>
        </div>
        <div className="goal-countdown-display reached">
          <span className="goal-countdown-reached-text">Target time has elapsed</span>
        </div>
        <div className="goal-countdown-footer">
          <span className="muted">Deadline was {targetDateStr}</span>
        </div>
      </div>
    );
  }

  return (
    <div className="goal-countdown-box">
      <div className="goal-countdown-header">
        <span className="goal-countdown-label">⏳ Time Remaining</span>
        <span className="goal-countdown-target-pill">Target: {targetDateStr}</span>
      </div>

      <div className="goal-countdown-display">
        <div className="goal-countdown-adaptive-text">
          {remaining.formattedText}
        </div>
      </div>

      <div className="goal-countdown-footer">
        {/* Adaptive breakdown badges */}
        <div className="goal-countdown-badges">
          {remaining.years > 0 && (
            <span className="goal-countdown-unit-badge">
              <strong>{remaining.years}</strong>y
            </span>
          )}
          {(remaining.months > 0 || remaining.years > 0) && (
            <span className="goal-countdown-unit-badge">
              <strong>{remaining.months}</strong>m
            </span>
          )}
          {(remaining.days > 0 || remaining.months > 0 || remaining.years > 0) && (
            <span className="goal-countdown-unit-badge">
              <strong>{remaining.days}</strong>d
            </span>
          )}
          <span className="goal-countdown-unit-badge">
            <strong>{String(remaining.hours).padStart(2, "0")}</strong>h
          </span>
          <span className="goal-countdown-unit-badge">
            <strong>{String(remaining.minutes).padStart(2, "0")}</strong>m
          </span>
          <span className="goal-countdown-unit-badge">
            <strong>{String(remaining.seconds).padStart(2, "0")}</strong>s
          </span>
        </div>
      </div>
    </div>
  );
}
