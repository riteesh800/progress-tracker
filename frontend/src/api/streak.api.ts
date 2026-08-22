import { api } from "./client";
import type { ActivityItem } from "../lib/activityText";

export type StreakSummary = { current_streak: number; longest_streak: number };

export type StreakCalendar = {
  start: string;
  end: string;
  timezone: string;
  total_submissions: number;
  total_active_days: number;
  current_streak: number;
  longest_streak: number;
  days: { date: string; count: number }[];
};

export const getStreak = () => api<StreakSummary>("/streak");
export const getStreakCalendar = () => api<StreakCalendar>("/streak/calendar");
export const getActivity = () => api<ActivityItem[]>("/activity?limit=20");
