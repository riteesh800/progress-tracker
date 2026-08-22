import { FormEvent, useState } from "react";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { createNote, deleteNote, listNotes } from "../api/notes.api";
import { ApiError } from "../api/client";

export default function NotesPanel({ topicId, topicName }: { topicId: string; topicName: string }) {
  const qc = useQueryClient();
  const q = useQuery({ queryKey: ["notes", topicId], queryFn: () => listNotes(topicId) });
  const [content, setContent] = useState("");
  const [error, setError] = useState("");
  const add = useMutation({
    mutationFn: () => createNote(topicId, content),
    onSuccess: () => {
      setContent("");
      setError("");
      qc.invalidateQueries({ queryKey: ["notes", topicId] });
    },
    onError: (e) => setError(e instanceof ApiError ? e.message : "Could not save note"),
  });

  function submit(e: FormEvent) {
    e.preventDefault();
    if (!content.trim()) {
      setError("Note content cannot be empty");
      return;
    }
    add.mutate();
  }

  return (
    <div className="card" style={{ marginTop: 8 }}>
      <h4>Notes — {topicName}</h4>
      {q.isLoading && <p>Loading notes…</p>}
      {q.data?.length === 0 && <p className="muted">No notes yet.</p>}
      {q.data?.map((n) => (
        <div key={n.id} className="row" style={{ marginBottom: 6 }}>
          <pre style={{ whiteSpace: "pre-wrap", flex: 1, margin: 0 }}>{n.content}</pre>
          <button className="danger" onClick={() => deleteNote(n.id).then(() => qc.invalidateQueries({ queryKey: ["notes", topicId] }))}>
            Delete
          </button>
        </div>
      ))}
      <form onSubmit={submit} className="grid">
        <textarea value={content} onChange={(e) => setContent(e.target.value)} rows={3} placeholder="Plain text note" />
        {error && <div className="error">{error}</div>}
        <button className="primary" type="submit">{add.isPending ? "Saving note…" : "Add note"}</button>
      </form>
    </div>
  );
}
