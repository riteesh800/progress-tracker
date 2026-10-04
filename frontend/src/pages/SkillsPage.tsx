import { FormEvent, useState, useEffect } from "react";
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

  useEffect(() => {
    if (error) {
      const timer = setTimeout(() => setError(""), 3000);
      return () => clearTimeout(timer);
    }
  }, [error]);
  const create = useMutation({
    mutationFn: () => createSkill(name),
    onSuccess: () => {
      setName("");
      qc.invalidateQueries({ queryKey: ["skills"] });
      qc.invalidateQueries({ queryKey: ["dashboard"] });
    },
    onError: (e) => setError(e instanceof ApiError ? e.message : "Could not create skill"),
  });

  async function confirmDelete() {
    if (!pendingDelete) return;
    setDeleting(true);
    try {
      await deleteSkill(pendingDelete.id, true);
      setPendingDelete(null);
      qc.invalidateQueries({ queryKey: ["skills"] });
      qc.invalidateQueries({ queryKey: ["dashboard"] });
      qc.invalidateQueries({ queryKey: ["skills-trash"] });
      qc.invalidateQueries({ queryKey: ["activity"] });
      qc.invalidateQueries({ queryKey: ["streak-calendar"] });
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
        <input style={{ width: "75%" }} placeholder="New skill name" value={name} onChange={(e) => setName(e.target.value)} required maxLength={200} />
        <button className="primary" type="submit" disabled={create.isPending}>
          {create.isPending ? "Creating…" : "Create"}
        </button>
      </form>
      {error && <p className="error">{error}</p>}
      {q.data?.length === 0 && (
        <div className="card" style={{ justifySelf: "center", margin: "4rem auto", display: "flex", flexDirection: "column", alignItems: "center", gap: "1rem", padding: "2rem 3rem", textAlign: "center" }}>
          <div>No skills yet — create your first one, or</div>
          <Link to="/import" tabIndex={-1} style={{ textDecoration: "none" }}>
            <button type="button">Upload a Syllabus</button>
          </Link>
        </div>
      )}
      <div className="cards">
        {q.data?.map((s, i) => (
          <div className="card skill-card" key={s.id}>
            <Link
              className="skill-card-hit"
              to={`/skills/${s.id}`}
              aria-label={`Open ${s.name}`}
            />
            <span className="skill-card-name" title={s.name} style={{ color: "lightgreen" }}>
              {s.name}
            </span>
            {s.empty ? <p className="muted">No topics yet</p> : (
              <>
                <Donut percent={s.percent} label={`${s.percent}%`} />
                <p className="muted">{s.completed_leaves}/{s.total_leaves}</p>
              </>
            )}
            <button
              type="button"
              className="danger skill-card-delete"
              onClick={(e) => {
                e.preventDefault();
                e.stopPropagation();
                setPendingDelete({ id: s.id, name: s.name });
              }}
            >
              Delete
            </button>
          </div>
        ))}
      </div>
      <ConfirmDialog
        open={Boolean(pendingDelete)}
        title={`Move "${pendingDelete?.name ?? ""}" to Trash?`}
        body={"You can recover this skill for 3 days. After 3 days, it will be permanently deleted. You can also permanently Delete it from Trash during this period."}
        confirmLabel="Move to Trash"
        maxWidth="530px"
        busy={deleting}
        onConfirm={confirmDelete}
        onCancel={() => !deleting && setPendingDelete(null)}
      />
    </div>
  );
}
