import { FormEvent, useState } from "react";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { Link } from "react-router-dom";
import { createSkill, deleteSkill, listSkills } from "../api/skills.api";
import Donut from "../components/ProgressVisuals";
import ConfirmDialog from "../components/ConfirmDialog";
import { ApiError } from "../api/client";

export default function SkillsPage() {
  const qc = useQueryClient();
  const q = useQuery({ queryKey: ["skills"], queryFn: listSkills });
  const [name, setName] = useState("");
  const [error, setError] = useState("");
  const [pendingDelete, setPendingDelete] = useState<{ id: string; name: string } | null>(null);
  const [deleting, setDeleting] = useState(false);
  const create = useMutation({
    mutationFn: () => createSkill(name),
    onSuccess: () => { setName(""); qc.invalidateQueries({ queryKey: ["skills"] }); },
    onError: (e) => setError(e instanceof ApiError ? e.message : "Could not create skill"),
  });

  async function confirmDelete() {
    if (!pendingDelete) return;
    setDeleting(true);
    try {
      await deleteSkill(pendingDelete.id, true);
      setPendingDelete(null);
      qc.invalidateQueries({ queryKey: ["skills"] });
    } finally {
      setDeleting(false);
    }
  }

  if (q.isLoading) return <p>Loading skills…</p>;
  if (q.isError) return <p className="error">Could not load skills.</p>;

  return (
    <div className="grid">
      <h1>Skills</h1>
      <form className="row" onSubmit={(e: FormEvent) => { e.preventDefault(); create.mutate(); }}>
        <input placeholder="New skill name" value={name} onChange={(e) => setName(e.target.value)} required />
        <button className="primary" type="submit" disabled={create.isPending}>
          {create.isPending ? "Creating…" : "Create"}
        </button>
      </form>
      {error && <p className="error">{error}</p>}
      {q.data?.length === 0 && (
        <div className="card">No skills yet — create your first one, or <Link to="/import">upload a syllabus</Link>.</div>
      )}
      <div className="cards">
        {q.data?.map((s) => (
          <div className="card" key={s.id}>
            <Link to={`/skills/${s.id}`}><strong>{s.name}</strong></Link>
            {s.empty ? <p className="muted">No topics yet</p> : (
              <>
                <Donut percent={s.percent} label={`${s.percent}%`} />
                <p className="muted">{s.completed_leaves}/{s.total_leaves}</p>
              </>
            )}
            <button className="danger" onClick={() => setPendingDelete({ id: s.id, name: s.name })}>Delete</button>
          </div>
        ))}
      </div>
      <ConfirmDialog
        open={Boolean(pendingDelete)}
        title={`Are you sure you want to delete "${pendingDelete?.name ?? ""}"?`}
        body="You will permanently lose this complete skill, including its topics and notes."
        busy={deleting}
        onConfirm={confirmDelete}
        onCancel={() => !deleting && setPendingDelete(null)}
      />
    </div>
  );
}
