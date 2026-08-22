import { useMemo, useState } from "react";
import { useQuery } from "@tanstack/react-query";
import { getStreakCalendar } from "../api/streak.api";

const MONTHS = ["Jan", "Feb", "Mar", "Apr", "May", "Jun", "Jul", "Aug", "Sep", "Oct", "Nov", "Dec"];

function level(count: number) {
  if (count <= 0) return 0;
  if (count === 1) return 1;
  if (count <= 3) return 2;
  if (count <= 6) return 3;
  return 4;
}

function formatDay(iso: string) {
  const [y, m, d] = iso.split("-").map(Number);
  const date = new Date(y, m - 1, d);
  return `${MONTHS[date.getMonth()]} ${date.getDate()}, ${date.getFullYear()}`;
}

function padSunday(iso: string) {
  const [y, m, d] = iso.split("-").map(Number);
  const date = new Date(y, m - 1, d);
  const shift = date.getDay();
  date.setDate(date.getDate() - shift);
  const mm = String(date.getMonth() + 1).padStart(2, "0");
  const dd = String(date.getDate()).padStart(2, "0");
  return `${date.getFullYear()}-${mm}-${dd}`;
}

function addDays(iso: string, n: number) {
  const [y, m, d] = iso.split("-").map(Number);
  const date = new Date(y, m - 1, d);
  date.setDate(date.getDate() + n);
  const mm = String(date.getMonth() + 1).padStart(2, "0");
  const dd = String(date.getDate()).padStart(2, "0");
  return `${date.getFullYear()}-${mm}-${dd}`;
}

export default function StreakPage() {
  const q = useQuery({ queryKey: ["streak-calendar"], queryFn: getStreakCalendar });
  const [tip, setTip] = useState<{ text: string; x: number; y: number } | null>(null);

  const weeks = useMemo(() => {
    if (!q.data) return [];
    const counts = new Map(q.data.days.map((d) => [d.date, d.count]));
    const start = padSunday(q.data.start);
    const cells: { date: string; count: number; inRange: boolean }[] = [];
    let cursor = start;
    while (cursor <= q.data.end) {
      const inRange = cursor >= q.data.start;
      cells.push({ date: cursor, count: inRange ? counts.get(cursor) ?? 0 : 0, inRange });
      cursor = addDays(cursor, 1);
    }
    const out: typeof cells[] = [];
    for (let i = 0; i < cells.length; i += 7) out.push(cells.slice(i, i + 7));
    return out;
  }, [q.data]);

  const monthLabels = useMemo(() => {
    return weeks.map((week, i) => {
      const first = week.find((c) => c.inRange);
      if (!first) return "";
      const month = Number(first.date.slice(5, 7));
      if (i === 0) return MONTHS[month - 1];
      const prev = weeks[i - 1].find((c) => c.inRange);
      if (!prev) return MONTHS[month - 1];
      return Number(prev.date.slice(5, 7)) !== month ? MONTHS[month - 1] : "";
    });
  }, [weeks]);

  if (q.isPending) return <p>Loading streak…</p>;
  if (q.isError || !q.data) return <p className="error">Could not load streak history.</p>;
  const data = q.data;

  return (
    <div className="grid streak-page">
      <h1>Streak</h1>
      <div className="card streak-card">
        <div className="streak-stats">
          <div>
            <span className="streak-stat-num">{data.total_submissions}</span>
            <span className="muted"> submissions in the past one year</span>
          </div>
          <div className="streak-stats-right muted">
            <span>Total active days: {data.total_active_days}</span>
            <span>Current streak: {data.current_streak}</span>
            <span>Max streak: {data.longest_streak}</span>
          </div>
        </div>
        <div className="cal-wrap">
          <div className="cal-months">
            {monthLabels.map((label, i) => (
              <span key={i} className="cal-month">{label}</span>
            ))}
          </div>
          <div className="cal-grid">
            {weeks.map((week, wi) => (
              <div className="cal-week" key={wi}>
                {week.map((cell) => (
                  <span
                    key={cell.date}
                    className={`cal-day lvl-${cell.inRange ? level(cell.count) : "pad"}`}
                    onMouseEnter={(e) => {
                      if (!cell.inRange) return;
                      setTip({
                        text: `${formatDay(cell.date)} — ${cell.count} submissions`,
                        x: e.clientX,
                        y: e.clientY,
                      });
                    }}
                    onMouseMove={(e) => setTip((t) => (t ? { ...t, x: e.clientX, y: e.clientY } : t))}
                    onMouseLeave={() => setTip(null)}
                  />
                ))}
              </div>
            ))}
          </div>
          <div className="cal-legend muted">
            Less
            <span className="cal-day lvl-0" />
            <span className="cal-day lvl-1" />
            <span className="cal-day lvl-2" />
            <span className="cal-day lvl-3" />
            <span className="cal-day lvl-4" />
            More
          </div>
        </div>
      </div>
      {tip && (
        <div className="cal-tip" style={{ left: tip.x + 12, top: tip.y + 12 }}>
          {tip.text}
        </div>
      )}
    </div>
  );
}
