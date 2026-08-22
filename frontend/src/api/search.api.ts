import { api } from "./client";

export type SearchHit = {
  kind: string;
  id: string;
  title: string;
  snippet: string;
  skill_id?: string | null;
  topic_id?: string | null;
};

export const search = (q: string) => api<SearchHit[]>(`/search?q=${encodeURIComponent(q)}`);
