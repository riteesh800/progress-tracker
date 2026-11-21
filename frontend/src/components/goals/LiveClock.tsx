import { useEffect, useState, useRef } from "react";
import { calculateRemaining, type TimeRemaining } from "./CountdownTimer";

interface LiveClockProps {
  deadlineAt?: string;
  onDeadlineReached?: () => void;
}

export function LiveClock({ deadlineAt, onDeadlineReached }: LiveClockProps) {
  const [now, setNow] = useState(() => new Date());
  const [remaining, setRemaining] = useState<TimeRemaining>(() =>
    deadlineAt
      ? calculateRemaining(deadlineAt)
      : {
          isReached: false,
          years: 0,
          months: 0,
          days: 0,
          hours: 0,
          minutes: 0,
          seconds: 0,
          formattedText: "",
        }
  );

  const reachedFiredRef = useRef(false);

  // Recalculate remaining when deadlineAt changes
  useEffect(() => {
    if (!deadlineAt) return;
    const initial = calculateRemaining(deadlineAt);
    setRemaining(initial);
    if (initial.isReached && !reachedFiredRef.current && onDeadlineReached) {
      reachedFiredRef.current = true;
      onDeadlineReached();
    }
  }, [deadlineAt, onDeadlineReached]);

  // Synchronous 1-second live clock & countdown tick
  useEffect(() => {
    const timer = setInterval(() => {
      setNow(new Date());

      if (deadlineAt) {
        const next = calculateRemaining(deadlineAt);
        setRemaining(next);
        if (next.isReached && !reachedFiredRef.current && onDeadlineReached) {
          reachedFiredRef.current = true;
          onDeadlineReached();
        }
      }
    }, 1000);

    return () => clearInterval(timer);
  }, [deadlineAt, onDeadlineReached]);

  const hours = now.getHours();
  const minutes = now.getMinutes();
  const seconds = now.getSeconds();

  // Analog hand angles (360 degrees full circle)
  const secAngle = seconds * 6; // 360 / 60 = 6 deg/sec
  const minAngle = minutes * 6 + seconds * 0.1; // 6 deg/min + 0.1 deg/sec
  const hrAngle = (hours % 12) * 30 + minutes * 0.5; // 30 deg/hr + 0.5 deg/min

  return (
    <div className="goal-live-clock" aria-label="Live Clock with Deadline Countdown">
      <div className="goal-clock-face">
        {/* 12 Hour Tick Marks */}
        {Array.from({ length: 12 }).map((_, i) => (
          <div
            key={i}
            className="goal-clock-tick-wrap"
            style={{ transform: `rotate(${i * 30}deg)` }}
          >
            <div className={`goal-clock-tick ${i % 3 === 0 ? "major" : ""}`} />
          </div>
        ))}

        {/* Embedded Countdown Display (Image 2 placed inside Image 3 red circled zone) */}
        {deadlineAt && (
          <div className="goal-clock-embedded-countdown" aria-live="polite">
            {remaining.isReached ? (
              <div className="goal-clock-countdown-reached">
                <span className="goal-clock-reached-badge">🏁 Deadline Reached</span>
                <span className="goal-clock-reached-sub">Target time has elapsed</span>
              </div>
            ) : (
              <>
                <div className="goal-clock-countdown-text">
                  {remaining.formattedText}
                </div>

                <div className="goal-clock-countdown-badges">
                  {remaining.years > 0 && (
                    <span className="goal-clock-unit-badge">
                      <strong>{remaining.years}</strong>y
                    </span>
                  )}
                  {(remaining.months > 0 || remaining.years > 0) && (
                    <span className="goal-clock-unit-badge">
                      <strong>{remaining.months}</strong>m
                    </span>
                  )}
                  {(remaining.days > 0 || remaining.months > 0 || remaining.years > 0) && (
                    <span className="goal-clock-unit-badge">
                      <strong>{remaining.days}</strong>d
                    </span>
                  )}
                  <span className="goal-clock-unit-badge">
                    <strong>{String(remaining.hours).padStart(2, "0")}</strong>h
                  </span>
                  <span className="goal-clock-unit-badge">
                    <strong>{String(remaining.minutes).padStart(2, "0")}</strong>m
                  </span>
                  <span className="goal-clock-unit-badge">
                    <strong>{String(remaining.seconds).padStart(2, "0")}</strong>s
                  </span>
                </div>
              </>
            )}
          </div>
        )}

        {/* Hour Hand (Transparent body, visible glowing border, see-through to background content) */}
        <div
          className="goal-clock-hand hour-hand"
          style={{ transform: `rotate(${hrAngle}deg)` }}
          title={`Hour Hand (${hours % 12 || 12}h)`}
        />

        {/* Minute Hand (Transparent ice-cyan body, visible glowing border, see-through to background content) */}
        <div
          className="goal-clock-hand minute-hand"
          style={{ transform: `rotate(${minAngle}deg)` }}
          title={`Minute Hand (${minutes}m)`}
        />

        {/* Second Hand (Transparent ruby-red body, visible red border, see-through to background content) */}
        <div
          className="goal-clock-hand second-hand"
          style={{ transform: `rotate(${secAngle}deg)` }}
          title={`Second Hand (${seconds}s)`}
        />

        {/* Center Metallic Pivot Pin */}
        <div className="goal-clock-pin" aria-hidden />
      </div>
    </div>
  );
}
