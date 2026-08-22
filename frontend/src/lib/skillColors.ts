const PALETTE = [
  "#5eead4",
  "#fbbf24",
  "#fb7185",
  "#38bdf8",
  "#a78bfa",
  "#4ade80",
  "#f97316",
  "#e879f9",
  "#22d3ee",
  "#f472b6",
  "#84cc16",
  "#60a5fa",
  "#facc15",
  "#c084fc",
  "#34d399",
  "#fb923c",
  "#67e8f9",
  "#f43f5e",
  "#818cf8",
  "#d946ef",
];

export function uniqueSkillColor(index: number): string {
  if (index < PALETTE.length) return PALETTE[index];
  const hue = Math.round((index * 137.508) % 360);
  return `hsl(${hue} 72% 58%)`;
}
