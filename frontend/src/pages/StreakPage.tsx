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

type Cell = { date: string; count: number; inRange: boolean; isPadding: boolean };
type WeekData = { isGap: boolean; monthLabel: string; cells: Cell[] };

export default function StreakPage() {
  const q = useQuery({ queryKey: ["streak-calendar"], queryFn: getStreakCalendar });
  const [tip, setTip] = useState<{ text: string; x: number; y: number } | null>(null);
  const [selectedPeriod, setSelectedPeriod] = useState<string>("current");

  const years = useMemo(() => {
    if (!q.data) return [];
    const yrs = new Set<number>();
    if (q.data.start) yrs.add(parseInt(q.data.start.slice(0, 4)));
    if (q.data.end) yrs.add(parseInt(q.data.end.slice(0, 4)));
    q.data.days.forEach(d => yrs.add(parseInt(d.date.slice(0, 4))));
    yrs.add(new Date().getFullYear());
    return Array.from(yrs).sort((a, b) => b - a);
  }, [q.data]);

  const weeks = useMemo(() => {
    if (!q.data) return [];
    const counts = new Map(q.data.days.map((d) => [d.date, d.count]));
    const out: WeekData[] = [];
    
    let startYear, startMonth, endYear, endMonth;
    const now = new Date();
    
    if (selectedPeriod === "current") {
      endYear = now.getFullYear();
      endMonth = now.getMonth();
      startYear = endYear - 1;
      startMonth = endMonth + 1;
      if (startMonth > 11) {
        startMonth = 0;
        startYear++;
      }
    } else {
      const yr = parseInt(selectedPeriod);
      startYear = yr;
      startMonth = 0;
      endYear = yr;
      endMonth = 11;
    }

    let currY = startYear;
    let currM = startMonth;

    const todayStr = `${now.getFullYear()}-${String(now.getMonth()+1).padStart(2, '0')}-${String(now.getDate()).padStart(2, '0')}`;

    while (currY < endYear || (currY === endYear && currM <= endMonth)) {
      const daysInMonth = new Date(currY, currM + 1, 0).getDate();
      const firstDay = new Date(currY, currM, 1).getDay();
      const numWeeks = Math.ceil((daysInMonth + firstDay) / 7);
      const middleWeek = Math.floor(numWeeks / 2);

      let currentWeek: Cell[] = Array.from({ length: 7 }, () => ({ date: "", count: 0, inRange: false, isPadding: true }));
      let weekIndex = 0;

      for (let d = 1; d <= daysInMonth; d++) {
         const dateObj = new Date(currY, currM, d);
         const dayOfWeek = dateObj.getDay();
         
         const mm = String(currM + 1).padStart(2, '0');
         const dd = String(d).padStart(2, '0');
         const dateStr = `${currY}-${mm}-${dd}`;
         
         currentWeek[dayOfWeek] = {
             date: dateStr,
             count: counts.get(dateStr) ?? 0,
             inRange: dateStr <= todayStr,
             isPadding: false
         };
         
         if (dayOfWeek === 6 || d === daysInMonth) {
             out.push({ 
                 isGap: false, 
                 monthLabel: weekIndex === middleWeek ? MONTHS[currM] : "",
                 cells: [...currentWeek]
             });
             currentWeek = Array.from({ length: 7 }, () => ({ date: "", count: 0, inRange: false, isPadding: true }));
             weekIndex++;
         }
      }
      
      currM++;
      if (currM > 11) {
          currM = 0;
          currY++;
      }
      
      if (currY < endYear || (currY === endYear && currM <= endMonth)) {
          out.push({ 
              isGap: true, 
              monthLabel: "", 
              cells: Array.from({ length: 7 }, () => ({ date: "", count: 0, inRange: false, isPadding: true }))
          });
      }
    }
    
    return out;
  }, [q.data, selectedPeriod]);

  if (q.isPending) return <p>Loading streak…</p>;
  if (q.isError || !q.data) return <p className="error">Could not load streak history.</p>;
  const data = q.data;

  return (
    <div className="grid streak-page">
      <div className="row" style={{ justifyContent: "space-between" }}>
        <h1>Streak</h1>
        <select 
           value={selectedPeriod} 
           onChange={(e) => setSelectedPeriod(e.target.value)}
           style={{ width: "auto", minWidth: "220px" }}
        >
           <option value="current">Current (Last 12 Months)</option>
           {years.map(y => (
               <option key={y} value={String(y)}>{y}</option>
           ))}
        </select>
      </div>
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
            {weeks.map((week, wi) => (
              <span 
                key={wi} 
                className="cal-month" 
                style={{ 
                  width: week.isGap ? "4px" : undefined, 
                  textAlign: week.monthLabel ? "center" : undefined 
                }}
              >
                {week.monthLabel}
              </span>
            ))}
          </div>
          <div className="cal-grid">
            {weeks.map((week, wi) => (
              <div className="cal-week" style={week.isGap ? { width: "4px" } : undefined} key={wi}>
                {!week.isGap && week.cells.map((cell, idx) => (
                  <span
                    key={cell.date || `pad-${wi}-${idx}`}
                    className={`cal-day lvl-${!cell.isPadding && cell.inRange ? level(cell.count) : "pad"}`}
                    onMouseEnter={(e) => {
                      if (cell.isPadding || !cell.inRange) return;
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
