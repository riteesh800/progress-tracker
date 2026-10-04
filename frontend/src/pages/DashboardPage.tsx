import { Link } from "react-router-dom";
import { useQuery } from "@tanstack/react-query";
import { getDashboard } from "../api/progress.api";
import { OverallProgressRing } from "../components/ProgressVisuals";

const steps = [
  { emoji: "📄", title: "Upload PDF", text: "Import a syllabus and review the generated topic tree." },
  { emoji: "🌿", title: "Study leaves", text: "Work leaf topics only — those drive real completion." },
  { emoji: "✅", title: "Mark complete", text: "Check a leaf when you finish it. Parents update automatically." },
  { emoji: "📝", title: "Add notes", text: "Keep topic notes beside the tree, or personal tasks in Make Note." },
  { emoji: "🔥", title: "Keep a streak", text: "Any leaf completion on a day earns that learning day." },
  { emoji: "🔎", title: "Search & organize", text: "Find topics fast and keep each skill in its own tree." },
];

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

      <section className="card dash-intro">
        <h2>What is <span className="accent-text">Skill Progress Tracker</span>?</h2>
        <p>
          This app turns a messy syllabus into a <span className="accent-text">topic tree</span>.
          You study <span className="warn-text">leaf topics</span>, mark them complete, add notes,
          and watch progress and streaks grow — without losing credit if you later uncheck a leaf.
        </p>
        <div className="dash-flow">
          <span>Upload PDF</span><em>→</em>
          <span>Review tree</span><em>→</em>
          <span>Study leaves</span><em>→</em>
          <span>Mark complete</span><em>→</em>
          <span>Track progress</span><em>→</em>
          <span>Add notes</span><em>→</em>
          <span>Keep streak</span>
        </div>
        <div className="dash-step-grid">
          {steps.map((s) => (
            <article className="dash-step" key={s.title}>
              <div className="dash-emoji" aria-hidden>{s.emoji}</div>
              <h3>{s.title}</h3>
              <p className="muted">{s.text}</p>
            </article>
          ))}
        </div>
      </section>
    </div>
  );
}
