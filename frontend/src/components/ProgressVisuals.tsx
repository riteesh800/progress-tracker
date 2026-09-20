import { useEffect, useId, useState } from "react";

function Donut({ percent, label }: { percent: number; label: string }) {
  const p = Math.max(0, Math.min(100, percent));
  const bg = `conic-gradient(#5eead4 ${p * 3.6}deg, #243040 0deg)`;
  return (
    <div className="donut-wrap">
      <div className="donut" style={{ background: bg }}><span>{label}</span></div>
    </div>
  );
}

/** Dashboard overall-progress ring. Same 168px footprint as the previous donut. */
export function OverallProgressRing({ percent, label }: { percent: number; label: string }) {
  const p = Math.max(0, Math.min(100, percent));
  const gid = useId().replace(/:/g, "");
  const size = 168;
  const stroke = 11;
  const r = (size - stroke * 2) / 2;
  const c = 2 * Math.PI * r;
  const [drawn, setDrawn] = useState(0);

  useEffect(() => {
    const reduce = window.matchMedia("(prefers-reduced-motion: reduce)").matches;
    if (reduce) {
      setDrawn(p);
      return;
    }
    setDrawn(0);
    const frame = requestAnimationFrame(() => setDrawn(p));
    return () => cancelAnimationFrame(frame);
  }, [p]);

  const numeric = label.replace(/%/g, "").trim();
  const offset = c * (1 - drawn / 100);

  return (
    <div
      className="progress-ring"
      role="img"
      aria-label={`Overall progress ${label}`}
    >
      <svg viewBox={`0 0 ${size} ${size}`} aria-hidden>
        <defs>
          <linearGradient id={`prg-${gid}`} x1="0%" y1="0%" x2="100%" y2="100%">
            <stop offset="0%" stopColor="#5eead4" />
            <stop offset="55%" stopColor="#38bdf8" />
            <stop offset="100%" stopColor="#818cf8" />
          </linearGradient>
          <filter id={`glow-${gid}`} x="-20%" y="-20%" width="140%" height="140%">
            <feGaussianBlur stdDeviation="2.4" result="blur" />
            <feMerge>
              <feMergeNode in="blur" />
              <feMergeNode in="SourceGraphic" />
            </feMerge>
          </filter>
        </defs>
        <circle
          className="progress-ring-core"
          cx={size / 2}
          cy={size / 2}
          r={r - stroke * 0.9}
          fill="rgba(15, 20, 25, 0.45)"
          stroke="rgba(94, 234, 212, 0.14)"
          strokeWidth="1"
        />
        <circle
          className="progress-ring-track"
          cx={size / 2}
          cy={size / 2}
          r={r}
          fill="none"
          strokeWidth={stroke}
        />
        <circle
          className="progress-ring-arc"
          cx={size / 2}
          cy={size / 2}
          r={r}
          fill="none"
          stroke={`url(#prg-${gid})`}
          strokeWidth={stroke}
          strokeLinecap="round"
          strokeDasharray={c}
          strokeDashoffset={offset}
          filter={`url(#glow-${gid})`}
          transform={`rotate(-90 ${size / 2} ${size / 2})`}
        />
      </svg>
      <div className="progress-ring-label">
        <span className="progress-ring-value">
          {numeric}
          <span className="progress-ring-pct">%</span>
        </span>
      </div>
    </div>
  );
}

export function LineBar({ completed, total }: { completed: number; total: number }) {
  const pct = total === 0 ? 0 : (completed / total) * 100;
  return (
    <div className="line-bar">
      <div className="bar"><span style={{ width: `${pct}%` }} /></div>
      <span className="muted">{completed}/{total}</span>
    </div>
  );
}

export default Donut;
