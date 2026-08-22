import { api } from "./client";

export type ImportJob = {
  id: string;
  status: string;
  original_filename: string;
  file_size_bytes: number;
  skill_id?: string | null;
  generated_tree_json?: ProposedNode | null;
  confidence_notes?: unknown;
  error_message?: string | null;
};

export type ProposedNode = {
  name: string;
  confidence?: string;
  source?: string;
  children?: ProposedNode[];
};

export function uploadPdf(file: File) {
  const fd = new FormData();
  fd.append("file", file);
  return api<ImportJob>("/pdf/upload", { method: "POST", body: fd });
}
export const getJob = (id: string) => api<ImportJob>(`/pdf/jobs/${id}`);
export const patchTree = (id: string, generated_tree_json: ProposedNode) =>
  api<ImportJob>(`/pdf/jobs/${id}/tree`, { method: "PATCH", body: JSON.stringify({ generated_tree_json }) });
export const confirmJob = (id: string, skill_name?: string) =>
  api<ImportJob>(`/pdf/jobs/${id}/confirm`, { method: "POST", body: JSON.stringify({ skill_name }) });
