import { api } from "./client";

export const getDashboard = () =>
  api<{
    total_skills: number;
    total_topics: number;
    completed_topics: number;
    overall_progress: number;
    current_streak: number;
    longest_streak: number;
    empty: boolean;
  }>("/dashboard");

export const suggestedNext = () =>
  api<{
    skill_id?: string | null;
    skill_name?: string | null;
    topic_id?: string | null;
    topic_name?: string | null;
    reason: string;
  }>("/dashboard/suggested-next");
