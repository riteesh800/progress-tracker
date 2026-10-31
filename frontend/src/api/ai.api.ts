import { api } from "./client";

export type AITopicNode = {
  name: string;
  description?: string | null;
  children?: AITopicNode[];
};

export type GenerateSyllabusResponse = {
  skill_name: string;
  topics: AITopicNode[];
};

export type CreatedSkill = {
  id: string;
  name: string;
  description?: string | null;
  created_at: string;
  completed_leaves: number;
  total_leaves: number;
  percent: number;
  empty: boolean;
};

export function generateSyllabus(subject: string, depth: string, purpose: string) {
  return api<GenerateSyllabusResponse>("/ai/generate-syllabus", {
    method: "POST",
    body: JSON.stringify({ subject, depth, purpose }),
    timeoutMs: 120000,
  });
}

export function refineSyllabus(
  subject: string,
  depth: string,
  purpose: string,
  current_tree: { skill_name: string; topics: AITopicNode[] },
  conversation_history: { role: string; content: string }[],
  instruction: string
) {
  return api<GenerateSyllabusResponse>("/ai/refine-syllabus", {
    method: "POST",
    body: JSON.stringify({
      subject,
      depth,
      purpose,
      current_tree,
      conversation_history,
      instruction,
    }),
    timeoutMs: 120000,
  });
}

export function confirmSyllabus(
  skill_name: string,
  topics: AITopicNode[],
  skill_description?: string
) {
  return api<CreatedSkill>("/ai/confirm-syllabus", {
    method: "POST",
    body: JSON.stringify({ skill_name, skill_description, topics }),
  });
}

