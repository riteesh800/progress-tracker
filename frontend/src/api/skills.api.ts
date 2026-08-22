import { api } from "./client";

export type Skill = {
  id: string;
  name: string;
  description?: string | null;
  created_at: string;
  completed_leaves: number;
  total_leaves: number;
  percent: number;
  empty: boolean;
};

export const listSkills = () => api<Skill[]>("/skills");
export const createSkill = (name: string, description?: string) =>
  api<Skill>("/skills", { method: "POST", body: JSON.stringify({ name, description }) });
export const getSkill = (id: string) => api<Skill>(`/skills/${id}`);
export const patchSkill = (id: string, body: Partial<{ name: string; description: string }>) =>
  api<Skill>(`/skills/${id}`, { method: "PATCH", body: JSON.stringify(body) });
export const deleteSkill = (id: string, confirm: boolean) =>
  api(`/skills/${id}?confirm=${confirm}`, { method: "DELETE" });
export const skillImpact = (id: string) =>
  api<{ descendants: number; notes: number; requires_confirm: boolean }>(`/skills/${id}/delete-impact`);
