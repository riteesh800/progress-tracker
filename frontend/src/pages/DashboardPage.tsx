import { Link } from "react-router-dom";
import { useQuery } from "@tanstack/react-query";
import { getDashboard } from "../api/progress.api";
import { OverallProgressRing } from "../components/ProgressVisuals";
import { ConsistencyGraph } from "../components/ConsistencyGraph";

export default function DashboardPage() {
  const dash = useQuery({ queryKey: ["dashboard"], queryFn: getDashboard });

  if (dash.isLoading) return <p>Loading dashboard…</p>;
  if (dash.isError) return <p className="error">Could not load dashboard.</p>;
  const d = dash.data!;

  return (
    <div className="grid dash-page">
      <header className="dash-hero">
        <div>
          <h1>Dashboard</h1>
          <p className="muted">Your home for turning a syllabus into visible, lasting progress.</p>
        </div>
      </header>

      {d.empty ? (
        <div className="card">
          <h2>No skills yet</h2>
          <p className="muted">Upload a syllabus PDF or create a skill to start tracking leaves.</p>
          <div className="row">
            <Link to="/import" tabIndex={-1} style={{ textDecoration: "none" }}>
              <button type="button">Import a PDF</button>
            </Link>
            <Link to="/skills" tabIndex={-1} style={{ textDecoration: "none" }}>
              <button type="button">Create a skill</button>
            </Link>
          </div>
        </div>
      ) : (
        <section className="dash-overview">
          <div className="card dash-progress-card">
            <h3>Overall progress</h3>
            <OverallProgressRing percent={d.overall_progress} label={`${d.overall_progress}%`} />
            <p className="muted dash-progress-caption">{d.completed_topics} completed leaf topics</p>
          </div>
          <div className="dash-stats">
            <div
              className="card dash-stat-card"
              onContextMenu={(e) => e.preventDefault()}
              onMouseDown={(e) => e.preventDefault()}
            >
              <div className="dash-stat-label">
                <span className="dash-stat-emoji" aria-hidden>👥</span>
                <span className="muted">Skills</span>
              </div>
              <div className="dash-stat">{d.total_skills}</div>
            </div>
            <div
              className="card dash-stat-card"
              onContextMenu={(e) => e.preventDefault()}
              onMouseDown={(e) => e.preventDefault()}
            >
              <div className="dash-stat-label">
                <span className="dash-stat-emoji" aria-hidden>📚</span>
                <span className="muted">Topics</span>
              </div>
              <div className="dash-stat">{d.total_topics}</div>
            </div>
            <div
              className="card dash-stat-card"
              onContextMenu={(e) => e.preventDefault()}
              onMouseDown={(e) => e.preventDefault()}
            >
              <div className="dash-stat-label">
                <span className="dash-stat-emoji" aria-hidden>✅</span>
                <span className="muted">Completed leaves</span>
              </div>
              <div className="dash-stat">{d.completed_topics}</div>
            </div>
            <div
              className="card dash-stat-card"
              onContextMenu={(e) => e.preventDefault()}
              onMouseDown={(e) => e.preventDefault()}
            >
              <div className="dash-stat-label">
                <span className="dash-stat-emoji" aria-hidden>🔥</span>
                <span className="muted">Streak</span>
              </div>
              <div className="dash-stat">{d.current_streak}</div>
              <div className="muted dash-stat-sub">best {d.longest_streak}</div>
            </div>
          </div>
        </section>
      )}

      <ConsistencyGraph />
    </div>
  );
}
