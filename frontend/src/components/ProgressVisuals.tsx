function Donut({ percent, label }: { percent: number; label: string }) {
  const p = Math.max(0, Math.min(100, percent));
  const bg = `conic-gradient(#5eead4 ${p * 3.6}deg, #243040 0deg)`;
  return (
    <div className="donut-wrap">
      <div className="donut" style={{ background: bg }}><span>{label}</span></div>
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
