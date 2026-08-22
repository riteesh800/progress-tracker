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
    setLoading(true);
    setError("");
    try {
      setHits(await search(q));
    } catch (err) {
      setError(err instanceof ApiError ? err.message : "Search failed");
    } finally {
      setLoading(false);
    }
  }

  return (
    <div className="grid">
      <h1>Search</h1>
      <form className="row" onSubmit={onSubmit}>
        <input value={q} onChange={(e) => setQ(e.target.value)} placeholder="Skills, topics, notes" />
        <button className="primary" type="submit">Search</button>
      </form>
      {loading && <p>Searching…</p>}
      {error && <p className="error">{error}</p>}
      {hits && hits.length === 0 && <p className="muted">No matches.</p>}
      {hits?.map((h) => (
        <div className="card" key={`${h.kind}-${h.id}`}>
          <div className="muted">{h.kind}</div>
          <strong>{h.title}</strong>
          <p>{h.snippet}</p>
          {h.skill_id && <Link to={`/skills/${h.skill_id}`}>Open in tree</Link>}
        </div>
      ))}
    </div>
  );
}
