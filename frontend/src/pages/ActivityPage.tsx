import { useQuery } from "@tanstack/react-query";
import { getActivity } from "../api/streak.api";
import { formatActivity } from "../lib/activityText";

export default function ActivityPage() {
  const q = useQuery({ queryKey: ["activity"], queryFn: getActivity });

  if (q.isLoading) return <p>Loading activity…</p>;
  if (q.isError) return <p className="error">Could not load recent activity.</p>;
  const items = q.data ?? [];

  return (
    <div className="grid activity-page">
      <h1>Recent activity</h1>
      <p className="muted">Latest 20 actions, newest first. Older history is kept for streaks and audit.</p>
      {items.length === 0 ? (
        <div className="card">No activity yet. Create a skill or complete a leaf topic to get started.</div>
      ) : (
        <ol className="activity-list">
          {items.map((item) => (
            <li key={item.id} className="card activity-item">
              <div>{formatActivity(item)}</div>
              <div className="muted activity-time">{new Date(item.occurred_at).toLocaleString()}</div>
            </li>
          ))}
        </ol>
      )}
    </div>
  );
}
