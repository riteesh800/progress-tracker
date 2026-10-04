import { FormEvent, useState } from "react";
import { useQuery, useQueryClient } from "@tanstack/react-query";
import { useParams } from "react-router-dom";
import { getSkill } from "../api/skills.api";
import { createTopic, skillTopics } from "../api/topics.api";
import Donut from "../components/ProgressVisuals";
import TopicTree from "../components/TopicTree";
import SkillTimerCard from "../components/SkillTimerCard";

export default function SkillDetailPage() {
  const { id } = useParams();
  const qc = useQueryClient();
  const skill = useQuery({ queryKey: ["skill", id], queryFn: () => getSkill(id!), enabled: Boolean(id) });
  const tree = useQuery({ queryKey: ["topics", id], queryFn: () => skillTopics(id!), enabled: Boolean(id) });
  const [rootName, setRootName] = useState("");

  if (!skill.data && skill.isPending) return <p>Loading skill…</p>;
  if (!tree.data && tree.isPending) return <p>Loading skill…</p>;
  if (skill.isError || tree.isError) return <p className="error">Could not load this skill.</p>;
  const s = skill.data!;
  const nodes = tree.data ?? [];

  async function addRoot(e: FormEvent) {
    e.preventDefault();
    if (!id || !rootName.trim()) return;
    await createTopic({ skill_id: id, name: rootName.trim() });
    setRootName("");
    await Promise.all([
      qc.invalidateQueries({ queryKey: ["topics", id] }),
      qc.invalidateQueries({ queryKey: ["skill", id] }),
      qc.invalidateQueries({ queryKey: ["dashboard"] }),
      qc.invalidateQueries({ queryKey: ["skills"] }),
    ]);
  }

  return (
    <div className="grid">
      <h1>{s.name}</h1>
      <div style={{ display: "flex", gap: "1rem", flexWrap: "wrap", alignItems: "stretch" }}>
        {s.empty ? (
          <div className="card" style={{ flex: 1, maxWidth: "500px" }}>
            No topics yet — upload a syllabus or add manually.
          </div>
        ) : (
          <div className="card" style={{ flex: 1, maxWidth: "500px" }}>
            <Donut percent={s.percent} label={`${s.percent}%`} />
            <p className="muted">{s.completed_leaves}/{s.total_leaves} leaf topics</p>
          </div>
        )}
        <SkillTimerCard skillId={id!} />
      </div>
      <form className="row" onSubmit={addRoot}>
        <input placeholder="Add root topic" value={rootName} onChange={(e) => setRootName(e.target.value)} />
        <button className="primary" type="submit">Add topic</button>
      </form>
      <TopicTree nodes={nodes} skillId={id!} />
    </div>
  );
}
