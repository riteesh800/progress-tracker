import { api } from "./client";

export interface SkillTimer {
  id: string;
  skill_id: string;
  from_date: string;
  to_date: string;
}

export interface SetTimerData {
  from_date: string;
  to_date: string;
}

export async function getSkillTimer(skillId: string): Promise<SkillTimer | null> {
  return api(`/skills/${skillId}/timer`);
}

export async function setSkillTimer(skillId: string, data: SetTimerData): Promise<SkillTimer> {
  return api(`/skills/${skillId}/timer`, {
    method: "POST",
    body: JSON.stringify(data),
  });
}

export async function deleteSkillTimer(skillId: string): Promise<void> {
  return api(`/skills/${skillId}/timer`, { method: "DELETE" });
}
