import { api } from "./client";

export type Note = { id: string; topic_id: string; content: string; created_at: string; updated_at: string };

export const listNotes = (topicId: string) => api<Note[]>(`/notes?topic_id=${topicId}`);
export const createNote = (topic_id: string, content: string) =>
  api<Note>("/notes", { method: "POST", body: JSON.stringify({ topic_id, content }) });
export const patchNote = (id: string, content: string) =>
  api<Note>(`/notes/${id}`, { method: "PATCH", body: JSON.stringify({ content }) });
export const deleteNote = (id: string) => api(`/notes/${id}`, { method: "DELETE" });
