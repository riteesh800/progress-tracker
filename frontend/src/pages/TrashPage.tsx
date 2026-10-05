import { useState, useEffect } from "react";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { getTrashSkills, restoreSkill, permanentlyDeleteSkill, Skill } from "../api/skills.api";
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
    return `Permanently deleted in ${diffDays} day${diffDays > 1 ? 's' : ''} ${remainingHours > 0 ? remainingHours + ' hour' + (remainingHours > 1 ? 's' : '') : ''}`;
  }
  return `Permanently deleted in ${diffHours} hour${diffHours > 1 ? 's' : ''}`;
}

export default function TrashPage() {
  const qc = useQueryClient();
  const q = useQuery({ queryKey: ["skills-trash"], queryFn: getTrashSkills });
  const [pendingDelete, setPendingDelete] = useState<{ id: string; name: string } | null>(null);
  const [deleting, setDeleting] = useState(false);

  const restore = useMutation({
    mutationFn: (id: string) => restoreSkill(id),
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: ["skills-trash"] });
      qc.invalidateQueries({ queryKey: ["skills"] });
      qc.invalidateQueries({ queryKey: ["dashboard"] });
      qc.invalidateQueries({ queryKey: ["activity"] });
      qc.invalidateQueries({ queryKey: ["streak-calendar"] });
    }
  });

  async function confirmDelete() {
    if (!pendingDelete) return;
    setDeleting(true);
    try {
      await permanentlyDeleteSkill(pendingDelete.id);
      setPendingDelete(null);
      qc.invalidateQueries({ queryKey: ["skills-trash"] });
      qc.invalidateQueries({ queryKey: ["activity"] });
      qc.invalidateQueries({ queryKey: ["streak-calendar"] });
    } finally {
      setDeleting(false);
    }
  }

  if (q.isLoading) return <p>Loading trash…</p>;
  if (q.isError) return <p className="error">Could not load trash.</p>;

  return (
    <div className="grid">
      <h1>Trash</h1>
      
      {q.data?.length === 0 && (
        <div className="card">Your trash is empty.<br/>No deleted skills.</div>
      )}
      
      <div className="cards">
        {q.data?.map((s, i) => (
          <div className="card skill-card" key={s.id} style={{ display: 'flex', flexDirection: 'column' }}>
            <span className="skill-card-name" title={s.name} style={{ color: uniqueSkillColor(i) }}>
              {s.name}
            </span>
            {s.empty ? <p className="muted">No topics yet</p> : (
              <>
                <Donut percent={s.percent} label={`${s.percent}%`} />
                <p className="muted">{s.completed_leaves}/{s.total_leaves}</p>
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
                  restore.mutate(s.id);
                }}
                disabled={restore.isPending}
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
                  setPendingDelete({ id: s.id, name: s.name });
                }}
              >
                Delete
              </button>
            </div>
          </div>
        ))}
      </div>
      
      <ConfirmDialog
        open={Boolean(pendingDelete)}
        title={`Delete "${pendingDelete?.name ?? ""}" permanently?`}
        body={"You will permanently lose this skill, including its topics and notes. This action cannot be undone."}
        confirmLabel="Delete Permanently"
        danger={true}
        busy={deleting}
        onConfirm={confirmDelete}
        onCancel={() => !deleting && setPendingDelete(null)}
      />
    </div>
  );
}
