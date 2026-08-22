export type ActivityItem = {
  id: string;
  action_type: string;
  topic_name_snapshot?: string | null;
  skill_name_snapshot?: string | null;
  parent_name_snapshot?: string | null;
  summary?: string | null;
  occurred_at: string;
};

export function formatActivity(item: ActivityItem): string {
  if (item.summary) return item.summary;
  const topic = item.topic_name_snapshot || "a topic";
  const skill = item.skill_name_snapshot || "a skill";
  const parent = item.parent_name_snapshot || skill;
  switch (item.action_type) {
    case "topic_completed":
      return `You completed "${topic}" in "${skill}"`;
    case "topic_uncompleted":
      return `You unmarked "${topic}" in "${skill}"`;
    case "skill_created":
      return `You added the skill "${skill}"`;
    case "pdf_imported":
      return `You imported "${topic}" and created "${skill}"`;
    case "topic_created":
      return `You added "${topic}" to "${parent}"`;
    case "skill_completed":
      return `You completed the skill "${skill}"`;
    case "skill_deleted":
      return `You deleted the skill "${skill}"`;
    default:
      return item.action_type.replaceAll("_", " ");
  }
}
