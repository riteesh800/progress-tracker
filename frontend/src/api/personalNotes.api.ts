import { api } from "./client";

export type PersonalTask = {
  id: string;
  content: string;
  is_completed: boolean;
  order_index: number;
};

export type PersonalNote = {
  id: string;
  title: string;
  created_at: string;
  updated_at: string;
  tasks: PersonalTask[];
};

export const listPersonalNotes = () => api<PersonalNote[]>("/personal-notes");
export const createPersonalNote = (title = "Untitled note") =>
  api<PersonalNote>("/personal-notes", { method: "POST", body: JSON.stringify({ title }) });
export const getPersonalNote = (id: string) => api<PersonalNote>(`/personal-notes/${id}`);
export const patchPersonalNote = (
  id: string,
  body: { title?: string; tasks?: { content: string; is_completed: boolean }[] },
) => api<PersonalNote>(`/personal-notes/${id}`, { method: "PATCH", body: JSON.stringify(body) });
export const deletePersonalNote = (id: string) =>
  api(`/personal-notes/${id}`, { method: "DELETE" });
