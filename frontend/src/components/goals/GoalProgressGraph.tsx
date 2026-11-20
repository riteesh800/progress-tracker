import React, { useId, useMemo, useState } from "react";
import type { GoalProgressPoint } from "../../api/goals.api";

interface GoalProgressGraphProps {
  points: GoalProgressPoint[];
  currentScore: number;
  workedDays: number;
  missedDays: number;
  totalTrackedDays: number;
  isDeadlineReached: boolean;
}

const MONTHS = ["Jan", "Feb", "Mar", "Apr", "May", "Jun", "Jul", "Aug", "Sep", "Oct", "Nov", "Dec"];

function formatFriendlyDate(iso: string): string {
  if (!iso || typeof iso !== "string") return "";
  const [y, m, d] = iso.split("-").map(Number);
  if (!y || !m || !d || isNaN(y) || isNaN(m) || isNaN(d)) return iso;
  const date = new Date(y, m - 1, d);
  if (isNaN(date.getTime())) return iso;
  return `${MONTHS[date.getMonth()]} ${date.getDate()}, ${date.getFullYear()}`;
}

export function GoalProgressGraph({
  points,
  currentScore,
  workedDays,
  missedDays,
  totalTrackedDays,
  isDeadlineReached,
}: GoalProgressGraphProps) {
  const uid = useId().replace(/:/g, "");
  const [hoveredIdx, setHoveredIdx] = useState<number | null>(null);

  // SVG Dimension system with guaranteed boundary containment
  const SVG_WIDTH = 1000;
  const SVG_HEIGHT = 300;
  const PADDING = { top: 32, bottom: 44, left: 52, right: 32 };
  const plotW = SVG_WIDTH - PADDING.left - PADDING.right;
  const plotH = SVG_HEIGHT - PADDING.top - PADDING.bottom;

  // Process data & scaling
  const chartData = useMemo(() => {
    if (!points || points.length === 0) {
      return { isEmpty: true, chartPoints: [], yDomainMin: 0, yDomainMax: 1, zeroY: 0, minScore: 0, maxScore: 0 };
    }

    // Check if there is at least one activity recorded
    const hasAnyActivity = points.some((p) => p.has_activity);
    if (!hasAnyActivity) {
      return { isEmpty: true, chartPoints: [], yDomainMin: 0, yDomainMax: 1, zeroY: 0, minScore: 0, maxScore: 0 };
    }

    // Anchor point: day before first point starts at 0.0
    const firstDate = points[0].date;
    const [fy, fm, fd] = firstDate.split("-").map(Number);
    const prevDateObj = !isNaN(fy) && !isNaN(fm) && !isNaN(fd) ? new Date(fy, fm - 1, fd - 1) : new Date();
    const prevDateStr = `${prevDateObj.getFullYear()}-${String(prevDateObj.getMonth() + 1).padStart(2, "0")}-${String(prevDateObj.getDate()).padStart(2, "0")}`;

    const extended = [
      {
        date: prevDateStr,
        has_activity: false,
        daily_change: 0,
        score: 0,
        is_current_day: false,
        isAnchor: true,
      },
      ...points.map((p) => ({ ...p, isAnchor: false })),
    ];

    // Find min and max score
    let minScore = 0;
    let maxScore = 0;
    for (const p of extended) {
      if (p.score < minScore) minScore = p.score;
      if (p.score > maxScore) maxScore = p.score;
    }

    // Add 18% headroom top & bottom so lines never touch boundaries
    const range = Math.max(2, maxScore - minScore);
    const yDomainMin = minScore - range * 0.18;
    const yDomainMax = maxScore + range * 0.18;

    const getY = (s: number) => {
      const normalized = (s - yDomainMin) / (yDomainMax - yDomainMin);
      const clamped = Math.max(0, Math.min(1, normalized));
      return PADDING.top + plotH - clamped * plotH;
    };

    const numPoints = extended.length;
    const getX = (i: number) => {
      if (numPoints <= 1) return PADDING.left + plotW / 2;
      return PADDING.left + (i / (numPoints - 1)) * plotW;
    };

    const chartPoints = extended.map((p, idx) => ({
      ...p,
      x: getX(idx),
      y: getY(p.score),
    }));

    const zeroY = getY(0);

    return {
      isEmpty: false,
      chartPoints,
      yDomainMin,
      yDomainMax,
      minScore,
      maxScore,
      zeroY,
    };
  }, [points]);

  if (chartData.isEmpty || chartData.chartPoints.length === 0) {
    return (
      <section className="card goal-graph-card" aria-label="Goal Daily Consistency Graph">
        <div className="goal-graph-header">
          <div className="goal-graph-title-wrap">
            <h2 className="goal-graph-title">
              <span aria-hidden>📈</span> Goal Consistency &amp; Momentum
            </h2>
            <p className="muted goal-graph-subtitle">
              Daily study consistency: +1 point per worked day, -1 point per missed completed day
            </p>
          </div>
          <div className="goal-graph-stats">
            <span className="goal-graph-stat-pill">0 Tracked Days</span>
          </div>
        </div>
        <div className="goal-graph-empty-wrap">
          <div className="goal-graph-empty">
            <span className="goal-graph-empty-icon" aria-hidden>
              🌱
            </span>
            <p className="goal-graph-empty-text">Consistency graph will appear here</p>
            <span className="muted goal-graph-empty-sub">
              Add your first activity card above to start tracking your daily consistency score.
            </span>
          </div>
        </div>
      </section>
    );
  }

  const { chartPoints, zeroY } = chartData;
  const minScore = chartData.minScore ?? 0;
  const maxScore = chartData.maxScore ?? 0;
  const numPoints = chartPoints.length;
  const latestPoint = chartPoints[chartPoints.length - 1];

  // Tooltip & Crosshair calculation
  const handlePointerMove = (e: React.PointerEvent<SVGSVGElement>) => {
    const rect = e.currentTarget.getBoundingClientRect();
    if (rect.width <= 0) return;
    const clientX = e.clientX - rect.left;
    const svgX = (clientX / rect.width) * SVG_WIDTH;

    let closest = 0;
    let minDiff = Infinity;
    for (let i = 0; i < chartPoints.length; i++) {
      const diff = Math.abs(chartPoints[i].x - svgX);
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

  const activeHover = hoveredIdx !== null ? chartPoints[hoveredIdx] : null;

  // Generate 4-5 horizontal grid line ticks
  const scoreTicks: number[] = [];
  const scoreStep = Math.max(1, Math.round((maxScore - minScore) / 4)) || 1;
  const startTick = Math.floor(minScore / scoreStep) * scoreStep;
  for (let s = startTick; s <= maxScore + scoreStep; s += scoreStep) {
    scoreTicks.push(s);
  }

  // Generate 5-7 evenly spaced date ticks along the X axis
  const dateTickIndices: number[] = [];
  const tickCount = Math.min(6, numPoints);
  const step = Math.max(1, Math.floor((numPoints - 1) / (tickCount - 1)));
  for (let i = 0; i < numPoints; i += step) {
    dateTickIndices.push(i);
  }
  if (dateTickIndices[dateTickIndices.length - 1] !== numPoints - 1) {
    dateTickIndices.push(numPoints - 1);
  }

  return (
    <section className="card goal-graph-card" aria-label="Goal Daily Consistency Graph">
      <div className="goal-graph-header">
        <div className="goal-graph-title-wrap">
          <h2 className="goal-graph-title">
            <span aria-hidden>📈</span> Goal Consistency &amp; Momentum
          </h2>
          <p className="muted goal-graph-subtitle">
            Daily score: +1 for working on this goal, -1 for missed days. Current day has no penalty until completed.
          </p>
        </div>

        <div className="goal-graph-stats">
          <div className="goal-graph-stat-pill score">
            <span className="goal-graph-pill-label">Consistency Score</span>
            <strong>{currentScore > 0 ? `+${currentScore.toFixed(0)}` : currentScore.toFixed(0)}</strong>
          </div>
          <div className="goal-graph-stat-pill worked">
            <span className="goal-graph-pill-label">Worked</span>
            <span>{workedDays} {workedDays === 1 ? "day" : "days"}</span>
          </div>
          <div className="goal-graph-stat-pill missed">
            <span className="goal-graph-pill-label">Missed</span>
            <span>{missedDays} {missedDays === 1 ? "day" : "days"}</span>
          </div>
          <div className="goal-graph-stat-pill total">
            <span className="goal-graph-pill-label">Total Tracked</span>
            <span>{totalTrackedDays} {totalTrackedDays === 1 ? "day" : "days"}</span>
          </div>
        </div>
      </div>

      <div className="goal-graph-canvas-wrap">
        <svg
          viewBox={`0 0 ${SVG_WIDTH} ${SVG_HEIGHT}`}
          className="goal-graph-svg"
          preserveAspectRatio="none"
          onPointerMove={handlePointerMove}
          onPointerDown={handlePointerMove}
          onPointerLeave={handlePointerLeave}
          onPointerCancel={handlePointerLeave}
          aria-hidden="true"
        >
          <defs>
            <clipPath id={`goal-chart-clip-${uid}`}>
              <rect x="0" y="0" width={SVG_WIDTH} height={SVG_HEIGHT} rx="10" />
            </clipPath>

            <filter id={`goal-glow-green-${uid}`} x="-30%" y="-30%" width="160%" height="160%">
              <feDropShadow dx="0" dy="0" stdDeviation="3.5" floodColor="#22c55e" floodOpacity="0.85" />
            </filter>

            <filter id={`goal-glow-red-${uid}`} x="-30%" y="-30%" width="160%" height="160%">
              <feDropShadow dx="0" dy="0" stdDeviation="2.5" floodColor="#ef4444" floodOpacity="0.8" />
            </filter>

            <filter id={`goal-glow-halo-${uid}`} x="-50%" y="-50%" width="200%" height="200%">
              <feDropShadow dx="0" dy="0" stdDeviation="4.5" floodColor="#38bdf8" floodOpacity="0.95" />
            </filter>
          </defs>

          <g clipPath={`url(#goal-chart-clip-${uid})`}>
            {/* Horizontal Grid lines */}
            {scoreTicks.map((tickVal) => {
              const yVal = PADDING.top + plotH - ((tickVal - chartData.yDomainMin) / (chartData.yDomainMax - chartData.yDomainMin)) * plotH;
              if (yVal < PADDING.top || yVal > PADDING.top + plotH) return null;
              return (
                <g key={`grid-${tickVal}`}>
                  <line
                    x1={PADDING.left}
                    y1={yVal}
                    x2={SVG_WIDTH - PADDING.right}
                    y2={yVal}
                    stroke="rgba(148, 163, 184, 0.12)"
                    strokeDasharray="4 6"
                  />
                  <text
                    x={PADDING.left - 8}
                    y={yVal + 4}
                    fill="#94a3b8"
                    fontSize="11"
                    textAnchor="end"
                    fontWeight="500"
                  >
                    {tickVal > 0 ? `+${tickVal}` : tickVal}
                  </text>
                </g>
              );
            })}

            {/* Zero Baseline Line (distinct highlight) */}
            {zeroY >= PADDING.top && zeroY <= PADDING.top + plotH && (
              <line
                x1={PADDING.left}
                y1={zeroY}
                x2={SVG_WIDTH - PADDING.right}
                y2={zeroY}
                stroke="rgba(255, 255, 255, 0.3)"
                strokeWidth="1.4"
                strokeDasharray="3 3"
              />
            )}

            {/* Date labels on X-axis */}
            {dateTickIndices.map((idx) => {
              const pt = chartPoints[idx];
              if (!pt) return null;
              const dateObj = new Date(pt.date);
              const label = !isNaN(dateObj.getTime())
                ? `${MONTHS[dateObj.getMonth()]} ${dateObj.getDate()}`
                : pt.date;

              return (
                <text
                  key={`date-tick-${idx}`}
                  x={pt.x}
                  y={SVG_HEIGHT - 12}
                  fill="#94a3b8"
                  fontSize="11"
                  textAnchor="middle"
                >
                  {label}
                </text>
              );
            })}

            {/* Line segments: Green UP (+1), Red DOWN (-1), Slate Flat (0) */}
            {chartPoints.slice(1).map((pt, i) => {
              const prev = chartPoints[i];
              const isUp = pt.score > prev.score;
              const isDown = pt.score < prev.score;

              const stroke = isUp ? "#22c55e" : isDown ? "#ef4444" : "#64748b";
              const strokeWidth = isUp ? 2.8 : isDown ? 2.0 : 1.5;
              const filter = isUp
                ? `url(#goal-glow-green-${uid})`
                : isDown
                ? `url(#goal-glow-red-${uid})`
                : undefined;

              return (
                <line
                  key={`seg-${pt.date}-${i}`}
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

            {/* Soft node dots when number of points is small */}
            {numPoints <= 40 &&
              chartPoints.slice(1).map((pt, i) => {
                const prev = chartPoints[i];
                const isUp = pt.score > prev.score;
                const isDown = pt.score < prev.score;
                const color = isUp ? "#22c55e" : isDown ? "#ef4444" : "#64748b";
                return (
                  <circle
                    key={`dot-${pt.date}-${i}`}
                    cx={pt.x}
                    cy={pt.y}
                    r={isUp ? 3.5 : 2.5}
                    fill={color}
                    filter={isUp ? `url(#goal-glow-green-${uid})` : undefined}
                  />
                );
              })}

            {/* Latest point pulsing marker */}
            <g transform={`translate(${latestPoint.x}, ${latestPoint.y})`}>
              <circle
                r="7"
                fill={latestPoint.has_activity ? "#22c55e" : "#38bdf8"}
                filter={`url(#goal-glow-halo-${uid})`}
                opacity="0.85"
              />
              <circle r="3.5" fill="#ffffff" />
            </g>

            {/* Hover Crosshair */}
            {activeHover && (
              <g>
                <line
                  x1={activeHover.x}
                  y1={PADDING.top}
                  x2={activeHover.x}
                  y2={PADDING.top + plotH}
                  stroke="rgba(56, 189, 248, 0.45)"
                  strokeWidth="1.2"
                  strokeDasharray="3 3"
                />
                <circle
                  cx={activeHover.x}
                  cy={activeHover.y}
                  r="6"
                  fill={activeHover.has_activity ? "#22c55e" : "#ef4444"}
                  stroke="#ffffff"
                  strokeWidth="2"
                  filter={`url(#goal-glow-halo-${uid})`}
                />
              </g>
            )}
          </g>
        </svg>

        {/* Hover Tooltip strictly bounded within container */}
        {activeHover && (() => {
          const tipPct = (activeHover.x / SVG_WIDTH) * 100;
          let tipPositionStyle: React.CSSProperties;
          if (tipPct < 25) {
            tipPositionStyle = { left: "16px", transform: "none" };
          } else if (tipPct > 75) {
            tipPositionStyle = { right: "16px", left: "auto", transform: "none" };
          } else {
            tipPositionStyle = { left: `${tipPct}%`, transform: "translateX(-50%)" };
          }

          return (
            <div className="goal-graph-tooltip" style={tipPositionStyle}>
              <div className="goal-graph-tip-date">{formatFriendlyDate(activeHover.date)}</div>
              <div className="goal-graph-tip-badge-wrap">
                {activeHover.isAnchor ? (
                  <span className="goal-graph-tip-badge neutral">🌱 Start Baseline</span>
                ) : activeHover.has_activity ? (
                  <span className="goal-graph-tip-badge worked">
                    ▲ +1.0 Worked on goal
                  </span>
                ) : activeHover.is_current_day ? (
                  <span className="goal-graph-tip-badge today">
                    ⏳ Today (unfinished, no penalty yet)
                  </span>
                ) : (
                  <span className="goal-graph-tip-badge missed">
                    ▼ -1.0 Missed completed day
                  </span>
                )}
              </div>
              <div className="goal-graph-tip-score">
                Cumulative Score: <strong>{activeHover.score.toFixed(0)}</strong>
              </div>
            </div>
          );
        })()}
      </div>

      <footer className="goal-graph-footer">
        <div className="goal-graph-legend">
          <span className="goal-graph-legend-item">
            <span className="goal-graph-dot green" aria-hidden /> Worked (+1)
          </span>
          <span className="goal-graph-legend-item">
            <span className="goal-graph-dot red" aria-hidden /> Missed (-1)
          </span>
          <span className="goal-graph-legend-item">
            <span className="goal-graph-dot blue" aria-hidden /> Unfinished Today (0)
          </span>
        </div>
        <span className="muted goal-graph-meta">
          {isDeadlineReached
            ? `Final summary at deadline (${totalTrackedDays} total tracked days)`
            : `Tracking live consistency across ${totalTrackedDays} days`}
        </span>
      </footer>
    </section>
  );
}
