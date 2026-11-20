import { api } from "./client";

export type Goal = {
  id: string;
  goal_name: string;
  deadline_at: string;
  timezone: string;
  motivation_line?: string | null;
  deleted_at?: string | null;
  purge_at?: string | null;
  created_at: string;
  updated_at: string;
  is_expired: boolean;
  total_activities: number;
};

export type GoalDailySummary = {
  worked_days: number;
  missed_days: number;
  total_days: number;
  is_deadline_reached: boolean;
  inactivity_warning: boolean;
  current_score: number;
};

export type GoalDetail = {
  goal: Goal;
  summary: GoalDailySummary;
  inactivity_warning: boolean;
  missed_days: number;
  worked_days: number;
  total_days: number;
  current_score: number;
  is_deadline_reached: boolean;
};

export type GoalActivity = {
  id: string;
  goal_id: string;
  activity_text: string;
  activity_date: string;
  created_at: string;
};

export type GoalActivitiesPage = {
  activities: GoalActivity[];
  total: number;
  page: number;
  per_page: number;
};

export type GoalProgressPoint = {
  date: string;
  has_activity: boolean;
  daily_change: number;
  score: number;
  is_current_day: boolean;
};

export type GoalProgress = {
  goal_id: string;
  points: GoalProgressPoint[];
  current_score: number;
  worked_days: number;
  missed_days: number;
  total_tracked_days: number;
  is_deadline_reached: boolean;
  inactivity_warning: boolean;
};

export type CreateGoalPayload = {
  goal_name: string;
  deadline_at: string;
  timezone?: string;
  motivation_line?: string;
};

export const listGoals = () => api<Goal[]>("/goals");

export const createGoal = (payload: CreateGoalPayload) =>
  api<Goal>("/goals", {
    method: "POST",
    body: JSON.stringify(payload),
  });

export const getGoal = (id: string) => api<GoalDetail>(`/goals/${id}`);

export const deleteGoal = (id: string) =>
  api(`/goals/${id}`, {
    method: "DELETE",
  });

export const getTrashGoals = () => api<Goal[]>("/goals/trash");

export const restoreGoal = (id: string) =>
  api<Goal>(`/goals/${id}/restore`, {
    method: "POST",
  });

export const permanentlyDeleteGoal = (id: string) =>
  api(`/goals/${id}/permanent`, {
    method: "DELETE",
  });

export const addGoalActivity = (goalId: string, activityText: string) =>
  api<GoalActivity>(`/goals/${goalId}/activities`, {
    method: "POST",
    body: JSON.stringify({ activity_text: activityText }),
  });

export const listGoalActivities = (goalId: string, page = 1, perPage = 50) =>
  api<GoalActivitiesPage>(`/goals/${goalId}/activities?page=${page}&per_page=${perPage}`);

export const getGoalProgress = (goalId: string) => api<GoalProgress>(`/goals/${goalId}/progress`);
