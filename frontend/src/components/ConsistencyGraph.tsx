import React, { useId, useMemo, useState } from "react";
import { useQuery } from "@tanstack/react-query";
import { getStreakCalendar } from "../api/streak.api";

const MONTHS = ["Jan", "Feb", "Mar", "Apr", "May", "Jun", "Jul", "Aug", "Sep", "Oct", "Nov", "Dec"];

function formatFriendlyDate(iso: string): string {
  if (!iso || typeof iso !== "string") return "";
  const [y, m, d] = iso.split("-").map(Number);
  if (!y || !m || !d || isNaN(y) || isNaN(m) || isNaN(d)) return iso;
  const date = new Date(y, m - 1, d);
  if (isNaN(date.getTime())) return iso;
  return `${MONTHS[date.getMonth()]} ${date.getDate()}, ${date.getFullYear()}`;
}

interface RawDayPoint {
  date: string;
  count: number;
  score: number;
  prevScore: number;
  type: "up" | "down" | "flat" | "start";
}

interface ChartPoint extends RawDayPoint {
  x: number;
  y: number;
}

export function ConsistencyGraph() {
  const q = useQuery({
    queryKey: ["streak-calendar"],
    queryFn: getStreakCalendar,
  });

  const uid = useId().replace(/:/g, "");
  const [hoveredIdx, setHoveredIdx] = useState<number | null>(null);

  // SVG Dimension system with guaranteed boundary containment
  const SVG_WIDTH = 1000;
  const SVG_HEIGHT = 260;
  const PADDING = { top: 32, bottom: 36, left: 32, right: 32 };
  const plotW = SVG_WIDTH - PADDING.left - PADDING.right;
  const plotH = SVG_HEIGHT - PADDING.top - PADDING.bottom;

  // Process data:
  // 1. Max last 1 year (365 days)
  // 2. Start from first active submission day
  // 3. Score calculation:
  //    - If count > 0: +1.0 (UP, light shining green)
  //    - If count === 0: -0.5 (DOWN, thin shining red, clamped at 0)
  //    - 1 submission and 100 submissions are treated the same (+1.0)
  const processed = useMemo(() => {
    if (!q.data || !q.data.days || q.data.days.length === 0) {
      return { isEmpty: true, points: [], currentScore: 0, activeDaysCount: 0, totalDaysCount: 0, peakScore: 0 };
    }

    // Only last 365 days (max 1 year)
    const allDays = q.data.days.slice(-365);
    const firstActiveIdx = allDays.findIndex((d) => d.count > 0);

    // If 0 submissions across the year -> new account / empty state
    if (firstActiveIdx === -1) {
      return { isEmpty: true, points: [], currentScore: 0, activeDaysCount: 0, totalDaysCount: 0, peakScore: 0 };
    }

    const relevantDays = allDays.slice(firstActiveIdx);
    const rawPoints: RawDayPoint[] = [];

    // Anchor point at start: baseline 0 before first submission
    const firstDate = relevantDays[0].date || "";
    const [fy, fm, fd] = firstDate.split("-").map(Number);
    const prevDateObj = (!isNaN(fy) && !isNaN(fm) && !isNaN(fd)) ? new Date(fy, fm - 1, fd - 1) : new Date();
    const prevDateStr = `${prevDateObj.getFullYear()}-${String(prevDateObj.getMonth() + 1).padStart(2, "0")}-${String(prevDateObj.getDate()).padStart(2, "0")}`;

    rawPoints.push({
      date: prevDateStr,
      count: 0,
      score: 0,
      prevScore: 0,
      type: "start",
    });

    let currentScore = 0;
    let peakScore = 0;
    let activeDaysCount = 0;

    for (let i = 0; i < relevantDays.length; i++) {
      const day = relevantDays[i];
      const prevScore = currentScore;
      let type: "up" | "down" | "flat" = "flat";

      if (day.count > 0) {
        // Daily submission: goes up by +1.0
        currentScore += 1.0;
        activeDaysCount++;
        type = "up";
      } else {
        // Missed day: brings it down by -0.5 (double the down days of up to cancel)
        currentScore = Math.max(0, currentScore - 0.5);
        type = currentScore < prevScore ? "down" : "flat";
      }

      if (currentScore > peakScore) peakScore = currentScore;

      rawPoints.push({
        date: day.date,
        count: day.count,
        score: currentScore,
        prevScore,
        type,
      });
    }

    // Mathematical scaling & boundary clamping
    const numPoints = rawPoints.length;
    const maxScore = Math.max(1, peakScore);
    // Headroom of 14% ensures the curve never touches the ceiling
    const yDomainMax = maxScore * 1.14;
    const yDomainMin = 0;

    const getY = (s: number) => {
      const normalized = (s - yDomainMin) / (yDomainMax - yDomainMin);
      const clamped = Math.max(0, Math.min(1, normalized));
      return PADDING.top + plotH - clamped * plotH;
    };

    const getX = (i: number) => {
      if (numPoints <= 1) return PADDING.left + plotW / 2;
      return PADDING.left + (i / (numPoints - 1)) * plotW;
    };

    const points: ChartPoint[] = rawPoints.map((p, idx) => ({
      ...p,
      x: getX(idx),
      y: getY(p.score),
    }));

    return {
      isEmpty: false,
      points,
      currentScore,
      peakScore,
      activeDaysCount,
      totalDaysCount: relevantDays.length,
    };
  }, [q.data]);

  // Loading skeleton
  if (q.isLoading) {
    return (
      <section className="card dash-consistency-card" aria-label="Consistency Graph">
        <div className="dash-graph-header">
          <div className="dash-graph-title-wrap">
            <h2 className="dash-graph-title">
              <span className="dash-graph-icon" aria-hidden>📈</span> Account Consistency
            </h2>
            <p className="muted dash-graph-subtitle">Loading daily momentum…</p>
          </div>
        </div>
        <div className="dash-graph-canvas-wrap dash-graph-skeleton" />
      </section>
    );
  }

  // Error fallback
  if (q.isError) {
    return (
      <section className="card dash-consistency-card" aria-label="Consistency Graph">
        <div className="dash-graph-header">
          <div className="dash-graph-title-wrap">
            <h2 className="dash-graph-title">
              <span className="dash-graph-icon" aria-hidden>📈</span> Account Consistency
            </h2>
            <p className="error dash-graph-subtitle">Could not load consistency data.</p>
          </div>
        </div>
      </section>
    );
  }

  // 0 submissions (new account) -> Plain text in middle: "it shows your progress"
  if (processed.isEmpty || processed.points.length === 0) {
    return (
      <section className="card dash-consistency-card" aria-label="Account Consistency">
        <div className="dash-graph-header">
          <div className="dash-graph-title-wrap">
            <h2 className="dash-graph-title">
              <span className="dash-graph-icon" aria-hidden>📈</span> Account Consistency
            </h2>
            <p className="muted dash-graph-subtitle">
              Daily study momentum over the last year
            </p>
          </div>
          <div className="dash-graph-stats">
            <span className="dash-graph-pill">0 Submissions</span>
          </div>
        </div>
        <div className="dash-graph-canvas-wrap dash-graph-empty-wrap">
          <div className="dash-graph-empty">
            <p className="dash-graph-empty-text">it shows your progress</p>
            <span className="muted dash-graph-empty-sub">
              Complete leaf topics daily to build your consistency line.
            </span>
          </div>
        </div>
      </section>
    );
  }

  const { points, currentScore, peakScore, activeDaysCount, totalDaysCount } = processed;
  const numPoints = points.length;
  const baselineY = PADDING.top + plotH;
  const latestPoint = points[points.length - 1];

  // Build Area Path under the curve
  let areaPath = `M ${points[0].x} ${baselineY}`;
  for (const pt of points) {
    areaPath += ` L ${pt.x} ${pt.y}`;
  }
  areaPath += ` L ${latestPoint.x} ${baselineY} Z`;

  // Handle pointer interactions for tooltip & crosshair
  const handlePointerMove = (e: React.PointerEvent<SVGSVGElement>) => {
    const rect = e.currentTarget.getBoundingClientRect();
    if (rect.width <= 0) return;
    const clientX = e.clientX - rect.left;
    const svgX = (clientX / rect.width) * SVG_WIDTH;

    let closest = 0;
    let minDiff = Infinity;
    for (let i = 0; i < points.length; i++) {
      const diff = Math.abs(points[i].x - svgX);
      if (diff < minDiff) {
        minDiff = diff;
        closest = i;
      }
    }
    setHoveredIdx(closest);
  };

  const handlePointerLeave = () => {
    setHoveredIdx(null);
  };

  const activeHover = hoveredIdx !== null ? points[hoveredIdx] : null;

  return (
    <section className="card dash-consistency-card" aria-label="Account Consistency Graph">
      {/* Header with Title and Momentum Badges */}
      <div className="dash-graph-header">
        <div className="dash-graph-title-wrap">
          <h2 className="dash-graph-title">
            <span className="dash-graph-icon" aria-hidden>📈</span> Account Consistency &amp; Momentum
          </h2>
          <p className="muted dash-graph-subtitle">
            Daily study consistency over the last year • Daily submissions push upward, missed days bring it down
          </p>
        </div>

        <div className="dash-graph-stats">
          <div className="dash-graph-pill accent">
            <span className="dash-graph-pill-label">Momentum Score</span>
            <strong>{currentScore.toFixed(1)}</strong>
          </div>
          <div className="dash-graph-pill">
            <span className="dash-graph-pill-label">Active Days</span>
            <span>{activeDaysCount} of {totalDaysCount}</span>
          </div>
          {peakScore > 0 && (
            <div className="dash-graph-pill">
              <span className="dash-graph-pill-label">Peak</span>
              <span>{peakScore.toFixed(1)}</span>
            </div>
          )}
        </div>
      </div>

      {/* Graph Visual Area */}
      <div className="dash-graph-canvas-wrap">
        <svg
          viewBox={`0 0 ${SVG_WIDTH} ${SVG_HEIGHT}`}
          className="dash-graph-svg"
          preserveAspectRatio="none"
          onPointerMove={handlePointerMove}
          onPointerDown={handlePointerMove}
          onPointerLeave={handlePointerLeave}
          onPointerCancel={handlePointerLeave}
          aria-hidden="true"
        >
          <defs>
            {/* SVG Clip Path to guarantee nothing ever spills outside */}
            <clipPath id={`chart-bounds-${uid}`}>
              <rect x="0" y="0" width={SVG_WIDTH} height={SVG_HEIGHT} rx="12" />
            </clipPath>

            {/* Light shining green filter */}
            <filter id={`glow-green-${uid}`} x="-30%" y="-30%" width="160%" height="160%">
              <feDropShadow dx="0" dy="0" stdDeviation="3.5" floodColor="#22c55e" floodOpacity="0.85" />
            </filter>

            {/* Red shining thin line filter */}
            <filter id={`glow-red-${uid}`} x="-30%" y="-30%" width="160%" height="160%">
              <feDropShadow dx="0" dy="0" stdDeviation="2.5" floodColor="#ef4444" floodOpacity="0.8" />
            </filter>

            {/* Hover halo filter */}
            <filter id={`glow-halo-${uid}`} x="-50%" y="-50%" width="200%" height="200%">
              <feDropShadow dx="0" dy="0" stdDeviation="4.5" floodColor="#5eead4" floodOpacity="0.95" />
            </filter>

            {/* Ambient area gradient under the curve */}
            <linearGradient id={`area-grad-${uid}`} x1="0%" y1="0%" x2="0%" y2="100%">
              <stop offset="0%" stopColor="#22c55e" stopOpacity="0.22" />
              <stop offset="60%" stopColor="#10b981" stopOpacity="0.08" />
              <stop offset="100%" stopColor="#0f172a" stopOpacity="0.0" />
            </linearGradient>

            {/* Subtle grid pattern */}
            <linearGradient id={`grid-line-grad-${uid}`} x1="0%" y1="0%" x2="100%" y2="0%">
              <stop offset="0%" stopColor="rgba(255,255,255,0.02)" />
              <stop offset="50%" stopColor="rgba(255,255,255,0.08)" />
              <stop offset="100%" stopColor="rgba(255,255,255,0.02)" />
            </linearGradient>
          </defs>

          <g clipPath={`url(#chart-bounds-${uid})`}>
            {/* Background Grid Guide Lines */}
            <line
              x1={PADDING.left}
              y1={PADDING.top}
              x2={SVG_WIDTH - PADDING.right}
              y2={PADDING.top}
              stroke={`url(#grid-line-grad-${uid})`}
              strokeDasharray="4 6"
            />
            <line
              x1={PADDING.left}
              y1={PADDING.top + plotH / 2}
              x2={SVG_WIDTH - PADDING.right}
              y2={PADDING.top + plotH / 2}
              stroke={`url(#grid-line-grad-${uid})`}
              strokeDasharray="4 6"
            />
            {/* Baseline 0 Line */}
            <line
              x1={PADDING.left}
              y1={baselineY}
              x2={SVG_WIDTH - PADDING.right}
              y2={baselineY}
              stroke="rgba(148, 163, 184, 0.22)"
              strokeWidth="1.2"
              strokeDasharray="3 3"
            />

            {/* Subtle Area Fill under curve */}
            <path d={areaPath} fill={`url(#area-grad-${uid})`} />

            {/* Dynamic Graph Segments: Green UP, Thin Red DOWN, Slate FLAT */}
            {points.slice(1).map((pt, i) => {
              const prev = points[i];
              const isUp = pt.type === "up";
              const isDown = pt.type === "down";

              const stroke = isUp ? "#22c55e" : isDown ? "#ef4444" : "#475569";
              // Up is shining light green; Down is shining thin red line
              const strokeWidth = isUp ? 2.8 : isDown ? 1.6 : 1.4;
              const filter = isUp
                ? `url(#glow-green-${uid})`
                : isDown
                ? `url(#glow-red-${uid})`
                : undefined;

              return (
                <line
                  key={`${pt.date}-${i}`}
                  x1={prev.x}
                  y1={prev.y}
                  x2={pt.x}
                  y2={pt.y}
                  stroke={stroke}
                  strokeWidth={strokeWidth}
                  strokeLinecap="round"
                  filter={filter}
                />
              );
            })}

            {/* If points count is small, render soft node dots on the line */}
            {numPoints <= 35 &&
              points.slice(1).map((pt, i) => {
                const isUp = pt.type === "up";
                const isDown = pt.type === "down";
                const color = isUp ? "#22c55e" : isDown ? "#ef4444" : "#475569";
                return (
                  <circle
                    key={`dot-${pt.date}-${i}`}
                    cx={pt.x}
                    cy={pt.y}
                    r={isUp ? 3.2 : 2.4}
                    fill={color}
                    filter={isUp ? `url(#glow-green-${uid})` : undefined}
                  />
                );
              })}

            {/* Latest point pulsing indicator */}
            <g transform={`translate(${latestPoint.x}, ${latestPoint.y})`}>
              <circle
                r="7"
                fill={latestPoint.type === "up" ? "#22c55e" : latestPoint.type === "down" ? "#ef4444" : "#94a3b8"}
                filter={`url(#glow-halo-${uid})`}
                opacity="0.8"
              />
              <circle r="3.5" fill="#ffffff" />
            </g>

            {/* Hover Crosshair & Focused Point */}
            {activeHover && (
              <g>
                <line
                  x1={activeHover.x}
                  y1={PADDING.top}
                  x2={activeHover.x}
                  y2={baselineY}
                  stroke="rgba(94, 234, 212, 0.45)"
                  strokeWidth="1.2"
                  strokeDasharray="3 3"
                />
                <circle
                  cx={activeHover.x}
                  cy={activeHover.y}
                  r="6"
                  fill={activeHover.type === "up" ? "#22c55e" : activeHover.type === "down" ? "#ef4444" : "#94a3b8"}
                  stroke="#ffffff"
                  strokeWidth="2"
                  filter={`url(#glow-halo-${uid})`}
                />
              </g>
            )}
          </g>
        </svg>

        {/* Interactive Floating Tooltip (strictly bounded inside card) */}
        {activeHover && (() => {
          const tipPct = (activeHover.x / SVG_WIDTH) * 100;
          let tipPositionStyle: React.CSSProperties;
          if (tipPct < 25) {
            tipPositionStyle = { left: "14px", transform: "none" };
          } else if (tipPct > 75) {
            tipPositionStyle = { right: "14px", left: "auto", transform: "none" };
          } else {
            tipPositionStyle = { left: `${tipPct}%`, transform: "translateX(-50%)" };
          }

          return (
            <div className="dash-graph-tooltip" style={tipPositionStyle}>
              <div className="dash-graph-tip-date">{formatFriendlyDate(activeHover.date)}</div>
              <div className="dash-graph-tip-row">
                {activeHover.type === "up" && (
                  <span className="dash-graph-tip-badge up">
                    ▲ +1.0 Daily push • {activeHover.count} {activeHover.count === 1 ? "submission" : "submissions"}
                  </span>
                )}
                {activeHover.type === "down" && (
                  <span className="dash-graph-tip-badge down">
                    ▼ -0.5 Missed day • No submissions
                  </span>
                )}
                {activeHover.type === "start" && (
                  <span className="dash-graph-tip-badge start">🌱 Journey baseline (0.0)</span>
                )}
                {activeHover.type === "flat" && (
                  <span className="dash-graph-tip-badge flat">— Floor at 0.0</span>
                )}
              </div>
              <div className="dash-graph-tip-score">
                Score: <strong>{activeHover.score.toFixed(1)}</strong>
              </div>
            </div>
          );
        })()}
      </div>

      {/* Legend & Timeline metadata */}
      <footer className="dash-graph-footer">
        <div className="dash-graph-legend">
          <span className="dash-graph-legend-item">
            <span className="dash-graph-dot green" aria-hidden /> Daily submission (+1.0 Up)
          </span>
          <span className="dash-graph-legend-item">
            <span className="dash-graph-dot red" aria-hidden /> Missed day (-0.5 Down)
          </span>
        </div>
        <span className="muted dash-graph-meta">
          Showing last 1 year timeline ({totalDaysCount} {totalDaysCount === 1 ? "day" : "days"} tracked)
        </span>
      </footer>
    </section>
  );
}

