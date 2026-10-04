import { useEffect, useState, useMemo } from "react";
import { useQuery, useMutation, useQueryClient } from "@tanstack/react-query";
import { getSkillTimer, setSkillTimer, deleteSkillTimer, SkillTimer } from "../api/timer.api";

export default function SkillTimerCard({ skillId }: { skillId: string }) {
  const qc = useQueryClient();
  const queryKey = ["skillTimer", skillId];
  const { data: timer, isPending } = useQuery({
    queryKey,
    queryFn: () => getSkillTimer(skillId),
    staleTime: 5 * 60 * 1000,
  });

  const [from, setFrom] = useState("");
  const [to, setTo] = useState("");
  const [errorMsg, setErrorMsg] = useState("");

  const setMut = useMutation({
    mutationFn: (args: { from: string; to: string }) =>
      setSkillTimer(skillId, { from_date: args.from, to_date: args.to }),
    onSuccess: (data) => {
      qc.setQueryData(queryKey, data);
      setErrorMsg("");
      setFrom("");
      setTo("");
    },
    onError: (err: any) => {
      setErrorMsg(err?.message || "Failed to set timer");
      setTimeout(() => setErrorMsg(""), 2000);
    },
  });

  const resetMut = useMutation({
    mutationFn: () => deleteSkillTimer(skillId),
    onSuccess: () => {
      qc.setQueryData(queryKey, null);
    },
  });

  // Re-render tick for time progress
  const [now, setNow] = useState(new Date());
  useEffect(() => {
    const iv = setInterval(() => {
      setNow(new Date());
    }, 60000); // update every minute
    return () => clearInterval(iv);
  }, []);

  const progressData = useMemo(() => {
    if (!timer) return null;
    
    // Normalize `now` to midnight for precise calendar day matching
    const today = new Date(now.getFullYear(), now.getMonth(), now.getDate());
    
    const startDate = new Date(timer.from_date + "T00:00:00");
    const endDate = new Date(timer.to_date + "T00:00:00");
    
    const isWaiting = today < startDate;
    const calcDate = isWaiting ? startDate : today;

    let percent = 0;
    let elapsedDaysInt = 0;
    let totalDaysInt = 0;
    const totalMs = endDate.getTime() - startDate.getTime();
    if (totalMs > 0) {
      const elapsedMs = today.getTime() - startDate.getTime();
      percent = Math.max(0, Math.min(100, (elapsedMs / totalMs) * 100));
      totalDaysInt = Math.round(totalMs / (1000 * 60 * 60 * 24));
      elapsedDaysInt = Math.round(elapsedMs / (1000 * 60 * 60 * 24));
      elapsedDaysInt = Math.max(0, Math.min(elapsedDaysInt, totalDaysInt));
    }

    let months = 0;
    let days = 0;
    let isFinished = false;

    if (today >= endDate) {
      isFinished = true;
    } else {
      months = (endDate.getFullYear() - calcDate.getFullYear()) * 12;
      months -= calcDate.getMonth();
      months += endDate.getMonth();
      
      days = endDate.getDate() - calcDate.getDate();
      if (days < 0) {
        months -= 1;
        const prevMonth = new Date(endDate.getFullYear(), endDate.getMonth(), 0);
        days += prevMonth.getDate();
      }
    }

    return { percent, months, days, isFinished, elapsedDaysInt, totalDaysInt, isWaiting };
  }, [timer, now]);

  if (isPending) return <div className="card timer-card">Loading...</div>;

  if (timer && progressData) {
    const { percent, months, days, isFinished, elapsedDaysInt, totalDaysInt, isWaiting } = progressData;
    let remainingText = "";
    if (isFinished) {
      remainingText = "Goal completed";
    } else {
      const parts = [];
      if (months > 0) parts.push(`${months} month${months > 1 ? "s" : ""}`);
      if (days > 0) parts.push(`${days} day${days > 1 ? "s" : ""}`);
      if (parts.length === 0) parts.push("0 days");
      remainingText = parts.join(" ") + " remaining";
    }

    const formatDate = (dateStr: string) => {
      const parts = dateStr.split("-");
      if (parts.length === 3) return `${parts[2]}-${parts[1]}-${parts[0]}`;
      return dateStr;
    };

    return (
      <div className="card timer-card" style={{ flex: 1, minWidth: "300px", position: "relative", paddingBottom: "1.5rem" }}>
        <button
          className="primary"
          style={{ position: "absolute", top: "0.5rem", right: "0.5rem", padding: "0.4rem 0.8rem", fontSize: "0.95rem" }}
          onClick={() => resetMut.mutate()}
          disabled={resetMut.isPending}
        >
          Reset
        </button>
        <div style={{ marginTop: "4.5rem", display: "flex", flexDirection: "column", gap: "1rem" }}>
          <div 
            style={{ 
              position: "relative", 
              height: "12px", 
              background: "#0b1218", 
              borderRadius: "99px", 
              overflow: "hidden",
              width: "calc(100% - 2cm)",
              margin: "0 auto",
              boxShadow: "inset 0 2px 4px rgba(0,0,0,0.5), 0 0 8px rgba(0,0,0,0.3)"
            }}
            title={`${elapsedDaysInt}/${totalDaysInt} days`}
          >
            <div style={{ height: "100%", width: `${percent}%`, background: "linear-gradient(90deg, var(--accent), var(--accent-2))", transition: "width 0.5s ease", boxShadow: "0 0 10px var(--accent)" }} />
          </div>
          <p style={{ 
            margin: "1rem 0 0 0", 
            textAlign: "center", 
            fontSize: "1.2rem",
            fontWeight: "bold",
            background: "linear-gradient(to bottom, #ffffff, #aaaaaa)",
            WebkitBackgroundClip: "text",
            color: "transparent",
            textShadow: "0px 2px 4px rgba(0,0,0,0.8), 0px 0px 10px rgba(255,255,255,0.2)"
          }}>
            {remainingText}
          </p>
        </div>
        {isWaiting && (
          <span className="muted" style={{ position: "absolute", bottom: "0.4rem", left: "0.6rem", fontSize: "0.75rem" }}>
            timer will start from {formatDate(timer.from_date)}
          </span>
        )}
        <span className="muted" style={{ position: "absolute", bottom: "0.4rem", right: "0.6rem", fontSize: "0.75rem" }}>
          {formatDate(timer.from_date)} to {formatDate(timer.to_date)}
        </span>
      </div>
    );
  }

  // Set Timer state
  const handleSet = (e: React.FormEvent) => {
    e.preventDefault();
    if (!from || !to) return;
    const fDate = new Date(from);
    const tDate = new Date(to);
    
    if (isNaN(fDate.getTime()) || isNaN(tDate.getTime())) {
      setErrorMsg("invalid time");
      setTimeout(() => setErrorMsg(""), 2000);
      return;
    }

    const fYear = fDate.getFullYear();
    const tYear = tDate.getFullYear();
    
    if (fYear < 2026 || fYear > 2100 || tYear < 2026 || tYear > 2100) {
      setErrorMsg("invalid time");
      setTimeout(() => setErrorMsg(""), 2000);
      return;
    }

    // current date before from? Prompt originally said "If current date < From date" but likely meant "If From date < current date" based on "make value must be > today"
    const today = new Date(now.getFullYear(), now.getMonth(), now.getDate());
    const fDateNoTime = new Date(fDate.getFullYear(), fDate.getMonth(), fDate.getDate());
    
    if (fDateNoTime < today) {
      setErrorMsg("enter valid time there only");
      setTimeout(() => setErrorMsg(""), 2000);
      return;
    }

    if (fDate >= tDate) {
      setErrorMsg("To date must be strictly after From date");
      setTimeout(() => setErrorMsg(""), 2000);
      return;
    }
    
    setMut.mutate({ from, to });
  };

  const todayStr = `${now.getFullYear()}-${String(now.getMonth() + 1).padStart(2, '0')}-${String(now.getDate()).padStart(2, '0')}`;
  const tomorrow = new Date(now.getFullYear(), now.getMonth(), now.getDate() + 1);
  const tomorrowStr = `${tomorrow.getFullYear()}-${String(tomorrow.getMonth() + 1).padStart(2, '0')}-${String(tomorrow.getDate()).padStart(2, '0')}`;

  return (
    <div className="card timer-card" style={{ flex: 1, minWidth: "300px", display: "flex", flexDirection: "column", gap: "0.5rem" }}>
      <h3 style={{ margin: "0 0 0.5rem 0", fontSize: "1rem" }}>Skill Completion Timeline</h3>
      <form onSubmit={handleSet} style={{ display: "flex", flexDirection: "column", gap: "0.8rem" }}>
        <div style={{ display: "flex", gap: "0.5rem", alignItems: "center" }}>
          <label style={{ width: "40px" }} className="muted">From:</label>
          <input type="date" value={from} min={todayStr} max="2100-12-31" onChange={(e) => setFrom(e.target.value)} required />
        </div>
        <div style={{ display: "flex", gap: "0.5rem", alignItems: "center" }}>
          <label style={{ width: "40px" }} className="muted">To:</label>
          <input type="date" value={to} min={tomorrowStr} max="2100-12-31" onChange={(e) => setTo(e.target.value)} required />
        </div>
        {errorMsg && <p className="error" style={{ margin: 0, fontSize: "0.85rem" }}>{errorMsg}</p>}
        <button className="primary" type="submit" disabled={setMut.isPending} style={{ marginTop: "0.5rem" }}>
          {setMut.isPending ? "Setting..." : "Set"}
        </button>
      </form>
    </div>
  );
}
