import { FormEvent, useState } from "react";
import { Link } from "react-router-dom";
import { search, type SearchHit } from "../api/search.api";
import { ApiError } from "../api/client";

export default function SearchPage() {
  const [q, setQ] = useState("");
  const [hits, setHits] = useState<SearchHit[] | null>(null);
  const [error, setError] = useState("");
  const [loading, setLoading] = useState(false);

  async function onSubmit(e: FormEvent) {
    e.preventDefault();
    const query = q.trim();
    if (!query) return;
    setLoading(true);
    setError("");
    try {
      setHits(await search(query));
    } catch (err) {
      setError(err instanceof ApiError ? err.message : "Search failed");
    } finally {
      setLoading(false);
    }
  }

  return (
    <div className="grid search-page">
      <h1>Search</h1>
      <form className="search-form" onSubmit={onSubmit} role="search">
        <div className="search-bar">
          <svg className="search-icon" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2" aria-hidden="true">
            <circle cx="11" cy="11" r="7" />
            <line x1="21" y1="21" x2="16.65" y2="16.65" strokeLinecap="round" />
          </svg>
          <input
            type="text"
            className="search-input"
            value={q}
            onChange={(e) => setQ(e.target.value)}
            placeholder="Skills, topics, notes"
            autoFocus
          />
          {q && (
            <button
              type="button"
              className="search-clear-btn"
              onClick={() => {
                setQ("");
                setHits(null);
              }}
              title="Clear search"
              aria-label="Clear search"
            >
              ✕
            </button>
          )}
        </div>
      </form>

      {loading && <p className="muted">Searching…</p>}
      {error && <p className="error">{error}</p>}
      {hits && hits.length === 0 && <p className="muted">No matches.</p>}

      {hits && hits.length > 0 && (
        <div className="search-results">
          {hits.map((h) => (
            <div className="card search-hit-card" key={`${h.kind}-${h.id}`}>
              <div className="search-hit-kind muted">{h.kind}</div>
              <strong className="search-hit-title">{h.title}</strong>
              <p className="search-hit-snippet">{h.snippet}</p>
              {h.skill_id && (
                <Link to={`/skills/${h.skill_id}`} tabIndex={-1} style={{ textDecoration: "none" }}>
                  <button type="button" className="search-hit-btn">Open in tree</button>
                </Link>
              )}
            </div>
          ))}
        </div>
      )}
    </div>
  );
}
