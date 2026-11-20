import { useEffect, useState } from "react";

export function LiveClock() {
  const [now, setNow] = useState(() => new Date());

  useEffect(() => {
    const timer = setInterval(() => {
      setNow(new Date());
    }, 1000);

    return () => clearInterval(timer);
  }, []);

  const hours = now.getHours();
  const minutes = now.getMinutes();
  const seconds = now.getSeconds();

  // Analog hand angles (360 degrees full circle)
  const secAngle = seconds * 6; // 360 / 60 = 6 deg/sec
  const minAngle = minutes * 6 + seconds * 0.1; // 6 deg/min + 0.1 deg/sec
  const hrAngle = (hours % 12) * 30 + minutes * 0.5; // 30 deg/hr + 0.5 deg/min

  const digitalTime = now.toLocaleTimeString(undefined, {
    hour: "2-digit",
    minute: "2-digit",
    second: "2-digit",
    hour12: true,
  });

  const dateString = now.toLocaleDateString(undefined, {
    weekday: "short",
    month: "short",
    day: "numeric",
    year: "numeric",
  });

  return (
    <div className="goal-live-clock" aria-label="Current Local Time Clock">
      <div className="goal-clock-face">
        {/* Hour tick marks */}
        {Array.from({ length: 12 }).map((_, i) => (
          <div
            key={i}
            className={`goal-clock-tick ${i % 3 === 0 ? "major" : ""}`}
            style={{ transform: `rotate(${i * 30}deg)` }}
          />
        ))}

        {/* Hour Hand */}
        <div
          className="goal-clock-hand hour-hand"
          style={{ transform: `translateX(-50%) rotate(${hrAngle}deg)` }}
        />

        {/* Minute Hand */}
        <div
          className="goal-clock-hand minute-hand"
          style={{ transform: `translateX(-50%) rotate(${minAngle}deg)` }}
        />

        {/* Second Hand */}
        <div
          className="goal-clock-hand second-hand"
          style={{ transform: `translateX(-50%) rotate(${secAngle}deg)` }}
        />

        {/* Center Pivot Pin */}
        <div className="goal-clock-pin" />
      </div>

      <div className="goal-clock-readout">
        <span className="goal-clock-digital">{digitalTime}</span>
        <span className="goal-clock-date">{dateString}</span>
      </div>
    </div>
  );
}
