import { useState } from "react";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { getTrashSkills, restoreSkill, permanentlyDeleteSkill } from "../api/skills.api";
import { getTrashGoals, restoreGoal, permanentlyDeleteGoal } from "../api/goals.api";
import { ApiError } from "../api/client";
import Donut from "../components/ProgressVisuals";
import ConfirmDialog from "../components/ConfirmDialog";
import { uniqueSkillColor } from "../lib/skillColors";

function formatExpiration(deleted_at?: string | null, purge_at?: string | null) {
  if (!deleted_at || !purge_at) return "Deleted recently";
  const purgeDate = new Date(purge_at);
  const now = new Date();
  const diffMs = purgeDate.getTime() - now.getTime();

  if (diffMs <= 0) return "Expired";

  const diffHours = Math.floor(diffMs / (1000 * 60 * 60));
  const diffDays = Math.floor(diffHours / 24);

  if (diffDays > 0) {
    const remainingHours = diffHours % 24;
    return `Permanently deleted in ${diffDays} day${diffDays > 1 ? "s" : ""} ${
      remainingHours > 0 ? remainingHours + " hour" + (remainingHours > 1 ? "s" : "") : ""
    }`;
  }
  return `Permanently deleted in ${diffHours} hour${diffHours > 1 ? "s" : ""}`;
}

export default function TrashPage() {
  const qc = useQueryClient();
  const skillsQuery = useQuery({ queryKey: ["skills-trash"], queryFn: getTrashSkills });
  const goalsQuery = useQuery({ queryKey: ["goals-trash"], queryFn: getTrashGoals });

  const [pendingDelete, setPendingDelete] = useState<{ id: string; name: string; type: "skill" | "goal" } | null>(
    null
  );
  const [deleting, setDeleting] = useState(false);
  const [error, setError] = useState("");

  const restoreSkillMut = useMutation({
    mutationFn: (id: string) => restoreSkill(id),
    onSuccess: () => {
      setError("");
      qc.invalidateQueries({ queryKey: ["skills-trash"] });
      qc.invalidateQueries({ queryKey: ["skills"] });
      qc.invalidateQueries({ queryKey: ["dashboard"] });
      qc.invalidateQueries({ queryKey: ["activity"] });
      qc.invalidateQueries({ queryKey: ["streak-calendar"] });
    },
    onError: (e) => {
      setError(e instanceof ApiError ? e.message : "Could not restore skill.");
    },
  });

  const restoreGoalMut = useMutation({
    mutationFn: (id: string) => restoreGoal(id),
    onSuccess: () => {
      setError("");
      qc.invalidateQueries({ queryKey: ["goals-trash"] });
      qc.invalidateQueries({ queryKey: ["goals"] });
    },
    onError: (e) => {
      setError(e instanceof ApiError ? e.message : "Could not restore goal.");
    },
  });

  async function confirmDelete() {
    if (!pendingDelete) return;
    setDeleting(true);
    setError("");
    try {
      if (pendingDelete.type === "skill") {
        await permanentlyDeleteSkill(pendingDelete.id);
        qc.invalidateQueries({ queryKey: ["skills-trash"] });
        qc.invalidateQueries({ queryKey: ["activity"] });
        qc.invalidateQueries({ queryKey: ["streak-calendar"] });
      } else {
        await permanentlyDeleteGoal(pendingDelete.id);
        qc.invalidateQueries({ queryKey: ["goals-trash"] });
        qc.invalidateQueries({ queryKey: ["goals"] });
      }
      setPendingDelete(null);
    } catch (e) {
      setError(e instanceof ApiError ? e.message : `Could not permanently delete ${pendingDelete.type}.`);
    } finally {
      setDeleting(false);
    }
  }

  if (skillsQuery.isLoading || goalsQuery.isLoading) return <p>Loading trash…</p>;
  if (skillsQuery.isError && goalsQuery.isError) return <p className="error">Could not load trash.</p>;

  const trashedSkills = skillsQuery.data || [];
  const trashedGoals = goalsQuery.data || [];
  const isTrashEmpty = trashedSkills.length === 0 && trashedGoals.length === 0;

  return (
    <div className="grid">
      <h1>Trash</h1>
      {error && <p className="error">{error}</p>}

      {isTrashEmpty && (
        <div className="card">Your trash is empty.<br />No deleted skills or goals.</div>
      )}

      {/* Deleted Goals Section */}
      {trashedGoals.length > 0 && (
        <section style={{ marginTop: "1rem", marginBottom: "1.5rem" }}>
          <h2 style={{ fontSize: "1.25rem", marginBottom: "0.75rem", display: "flex", alignItems: "center", gap: "0.5rem" }}>
            <span>🎯</span> Deleted Goals ({trashedGoals.length})
          </h2>
          <div className="cards">
            {trashedGoals.map((g) => (
              <div
                className="card skill-card goal-trash-card"
                key={g.id}
                style={{ display: "flex", flexDirection: "column" }}
              >
                <span className="skill-card-name" title={g.goal_name} style={{ color: "#38bdf8" }}>
                  {g.goal_name}
                </span>

                {g.motivation_line && (
                  <p className="muted" style={{ fontSize: "0.85rem", fontStyle: "italic", margin: "0.4rem 0" }}>
                    &ldquo;{g.motivation_line}&rdquo;
                  </p>
                )}

                <p className="muted" style={{ fontSize: "0.85rem" }}>
                  Activities: <strong>{g.total_activities} recorded</strong>
                </p>

                <p className="muted" style={{ fontSize: "0.85rem", marginTop: "auto", marginBottom: "1rem" }}>
                  {formatExpiration(g.deleted_at, g.purge_at)}
                </p>

                <div style={{ display: "flex", gap: "0.5rem" }}>
                  <button
                    type="button"
                    className="primary"
                    style={{ flex: 1 }}
                    onClick={(e) => {
                      e.preventDefault();
                      e.stopPropagation();
                      restoreGoalMut.mutate(g.id);
                    }}
                    disabled={restoreGoalMut.isPending}
                  >
                    Restore
                  </button>
                  <button
                    type="button"
                    className="danger"
                    style={{ flex: 1 }}
                    onClick={(e) => {
                      e.preventDefault();
                      e.stopPropagation();
                      setPendingDelete({ id: g.id, name: g.goal_name, type: "goal" });
                    }}
                  >
                    Delete
                  </button>
                </div>
              </div>
            ))}
          </div>
        </section>
      )}

      {/* Deleted Skills Section */}
      {trashedSkills.length > 0 && (
        <section style={{ marginTop: trashedGoals.length > 0 ? "1rem" : "0" }}>
          {trashedGoals.length > 0 && (
            <h2 style={{ fontSize: "1.25rem", marginBottom: "0.75rem", display: "flex", alignItems: "center", gap: "0.5rem" }}>
              <span>📚</span> Deleted Skills ({trashedSkills.length})
            </h2>
          )}
          <div className="cards">
            {trashedSkills.map((s, i) => (
              <div className="card skill-card" key={s.id} style={{ display: "flex", flexDirection: "column" }}>
                <span className="skill-card-name" title={s.name} style={{ color: uniqueSkillColor(i) }}>
                  {s.name}
                </span>
                {s.empty ? (
                  <p className="muted">No topics yet</p>
                ) : (
                  <>
                    <Donut percent={s.percent} label={`${s.percent}%`} />
                    <p className="muted">
                      {s.completed_leaves}/{s.total_leaves}
                    </p>
                  </>
                )}

                <p className="muted" style={{ fontSize: "0.85rem", marginTop: "auto", marginBottom: "1rem" }}>
                  {formatExpiration(s.deleted_at, s.purge_at)}
                </p>

                <div style={{ display: "flex", gap: "0.5rem" }}>
                  <button
                    type="button"
                    className="primary"
                    style={{ flex: 1 }}
                    onClick={(e) => {
                      e.preventDefault();
                      e.stopPropagation();
                      restoreSkillMut.mutate(s.id);
                    }}
                    disabled={restoreSkillMut.isPending}
                  >
                    Restore
                  </button>
                  <button
                    type="button"
                    className="danger"
                    style={{ flex: 1 }}
                    onClick={(e) => {
                      e.preventDefault();
                      e.stopPropagation();
                      setPendingDelete({ id: s.id, name: s.name, type: "skill" });
                    }}
                  >
                    Delete
                  </button>
                </div>
              </div>
            ))}
          </div>
        </section>
      )}

      <ConfirmDialog
        open={Boolean(pendingDelete)}
        title={`Delete "${pendingDelete?.name ?? ""}" permanently?`}
        body={
          pendingDelete?.type === "goal"
            ? "You will permanently lose this goal and all associated daily activity records. This action cannot be undone."
            : "You will permanently lose this skill, including its topics and notes. This action cannot be undone."
        }
        confirmLabel="Delete Permanently"
        danger={true}
        busy={deleting}
        onConfirm={confirmDelete}
        onCancel={() => !deleting && setPendingDelete(null)}
      />
    </div>
  );
}
