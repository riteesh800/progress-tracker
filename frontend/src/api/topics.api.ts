import { api } from "./client";

export type TopicNode = {
  id: string;
  skill_id: string;
  parent_id: string | null;
  name: string;
  description?: string | null;
  order_index: number;
  is_leaf: boolean;
  is_completed: boolean;
  completed_leaves: number;
  total_leaves: number;
  percent: number;
  children: TopicNode[];
};

export const skillTopics = (skillId: string) => api<TopicNode[]>(`/skills/${skillId}/topics`);
export const createTopic = (body: { skill_id: string; parent_id?: string | null; name: string }) =>
  api<TopicNode>("/topics", { method: "POST", body: JSON.stringify(body) });
export const completeTopic = (id: string, completed: boolean) =>
  api<TopicNode>(`/topics/${id}/complete`, { method: "POST", body: JSON.stringify({ completed }) });
