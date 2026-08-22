import { useEffect, useMemo, useRef, useState } from "react";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import {
  createPersonalNote,
  deletePersonalNote,
  listPersonalNotes,
  patchPersonalNote,
  type PersonalNote,
} from "../api/personalNotes.api";
import { ApiError } from "../api/client";

type DraftTask = { content: string; is_completed: boolean };

function toDraft(note: PersonalNote): { title: string; tasks: DraftTask[] } {
  return {
    title: note.title,
    tasks: note.tasks.length
      ? [...note.tasks]
          .sort((a, b) => a.order_index - b.order_index)
          .map((t) => ({ content: t.content, is_completed: t.is_completed }))
      : [{ content: "", is_completed: false }],
  };
}

export default function MakeNotePage() {
  const qc = useQueryClient();
  const q = useQuery({ queryKey: ["personal-notes"], queryFn: listPersonalNotes });
  const [selectedId, setSelectedId] = useState<string | null>(null);
  const [title, setTitle] = useState("");
  const [tasks, setTasks] = useState<DraftTask[]>([{ content: "", is_completed: false }]);
  const [msg, setMsg] = useState<{ type: "ok" | "err"; text: string } | null>(null);
  const [saving, setSaving] = useState(false);
  const debounceRef = useRef<number | null>(null);
  const selectedIdRef = useRef<string | null>(null);
  const titleRef = useRef(title);
  const tasksRef = useRef(tasks);

  const selected = useMemo(
    () => q.data?.find((n) => n.id === selectedId) ?? q.data?.[0] ?? null,
    [q.data, selectedId],
  );

  useEffect(() => {
    selectedIdRef.current = selected?.id ?? null;
  }, [selected?.id]);

  useEffect(() => {
    titleRef.current = title;
  }, [title]);

  useEffect(() => {
    tasksRef.current = tasks;
  }, [tasks]);

  useEffect(() => {
    if (!selected) return;
    setSelectedId(selected.id);
    const draft = toDraft(selected);
    setTitle(draft.title);
    setTasks(draft.tasks);
    setMsg(null);
  }, [selected?.id]);

  async function persist(nextTitle: string, nextTasks: DraftTask[]) {
    const id = selectedIdRef.current;
    if (!id) return;
    setSaving(true);
    setMsg(null);
    try {
      const updated = await patchPersonalNote(id, {
        title: nextTitle,
        tasks: nextTasks.map((t) => ({ content: t.content, is_completed: t.is_completed })),
      });
      qc.setQueryData<PersonalNote[]>(["personal-notes"], (prev) => {
        if (!prev) return [updated];
        return prev.map((n) => (n.id === updated.id ? updated : n));
      });
      setMsg({ type: "ok", text: "Saved." });
    } catch (e) {
      setMsg({ type: "err", text: e instanceof ApiError ? e.message : "Could not save note." });
    } finally {
      setSaving(false);
    }
  }

  function persistNow(nextTitle: string, nextTasks: DraftTask[]) {
    if (debounceRef.current) window.clearTimeout(debounceRef.current);
    void persist(nextTitle, nextTasks);
  }

  function persistSoon(nextTitle: string, nextTasks: DraftTask[]) {
    if (debounceRef.current) window.clearTimeout(debounceRef.current);
    debounceRef.current = window.setTimeout(() => {
      void persist(nextTitle, nextTasks);
    }, 450);
  }

  useEffect(() => () => {
    if (debounceRef.current) window.clearTimeout(debounceRef.current);
  }, []);

  const create = useMutation({
    mutationFn: () => createPersonalNote("Untitled note"),
    onSuccess: async (note) => {
      await qc.invalidateQueries({ queryKey: ["personal-notes"] });
      setSelectedId(note.id);
      setMsg(null);
    },
    onError: (e) => setMsg({ type: "err", text: e instanceof ApiError ? e.message : "Could not create note." }),
  });

  const remove = useMutation({
    mutationFn: (id: string) => deletePersonalNote(id),
    onSuccess: async () => {
      setSelectedId(null);
      await qc.invalidateQueries({ queryKey: ["personal-notes"] });
    },
  });

  if (q.isLoading) return <p>Loading notes…</p>;
  if (q.isError) return <p className="error">Could not load notes.</p>;

  return (
    <div className="makenote-page">
      <aside className="makenote-list card">
        <div className="makenote-list-head">
          <h1>Make Note</h1>
        </div>
        {q.data?.length === 0 && <p className="muted">No notes yet.</p>}
        <ul>
          {q.data?.map((note) => (
            <li key={note.id}>
              <button
                type="button"
                className={note.id === selected?.id ? "makenote-item active" : "makenote-item"}
                onClick={() => setSelectedId(note.id)}
              >
                {note.title || "Untitled note"}
              </button>
            </li>
          ))}
        </ul>
      </aside>
      <section className="makenote-editor card">
        <div className="makenote-editor-head">
          <div className="muted">Personal tasks — not part of skill progress or streak</div>
          <button className="primary" type="button" onClick={() => create.mutate()} disabled={create.isPending}>
            + Create new note
          </button>
        </div>
        {!selected ? (
          <p className="muted">Create a note to start a flat task list.</p>
        ) : (
          <>
            <label>
              Note title
              <input
                value={title}
                onChange={(e) => {
                  const next = e.target.value;
                  setTitle(next);
                  persistSoon(next, tasksRef.current);
                }}
              />
            </label>
            <ol className="makenote-tasks">
              {tasks.map((task, index) => (
                <li key={index}>
                  <span className="muted makenote-num">{index + 1}.</span>
                  <input
                    type="checkbox"
                    checked={task.is_completed}
                    onChange={(e) => {
                      const next = [...tasks];
                      next[index] = { ...task, is_completed: e.target.checked };
                      setTasks(next);
                      persistNow(titleRef.current, next);
                    }}
                    aria-label={`Mark task ${index + 1} complete`}
                  />
                  <input
                    value={task.content}
                    onChange={(e) => {
                      const next = [...tasks];
                      next[index] = { ...task, content: e.target.value };
                      setTasks(next);
                      persistSoon(titleRef.current, next);
                    }}
                    placeholder="Task text"
                    aria-label={`Task ${index + 1}`}
                  />
                  <button
                    type="button"
                    onClick={() => {
                      const filtered = tasks.filter((_, i) => i !== index);
                      const next = filtered.length ? filtered : [{ content: "", is_completed: false }];
                      setTasks(next);
                      persistNow(titleRef.current, next);
                    }}
                    aria-label={`Remove task ${index + 1}`}
                  >
                    ×
                  </button>
                </li>
              ))}
            </ol>
            <div className="row">
              <button
                type="button"
                onClick={() => {
                  const next = [...tasks, { content: "", is_completed: false }];
                  setTasks(next);
                  persistNow(titleRef.current, next);
                }}
              >
                Add task
              </button>
              <button
                className="primary"
                type="button"
                disabled={saving}
                onClick={() => persistNow(titleRef.current, tasksRef.current)}
              >
                {saving ? "Saving…" : "Save"}
              </button>
              <button type="button" className="danger" disabled={remove.isPending} onClick={() => remove.mutate(selected.id)}>
                Delete note
              </button>
            </div>
            {msg && <div className={msg.type === "err" ? "error" : "settings-ok"}>{msg.text}</div>}
            {!msg && saving && <div className="muted">Saving…</div>}
          </>
        )}
      </section>
    </div>
  );
}
