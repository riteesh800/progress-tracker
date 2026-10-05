import { FormEvent, useState } from "react";
import { useMutation, useQueryClient, useQuery } from "@tanstack/react-query";
import type { TopicNode } from "../api/topics.api";
import { listNotes } from "../api/notes.api";
import { completeTopic, createTopic } from "../api/topics.api";
import { insertChildTopic, isLeafNode } from "../lib/topicTree";
import { LineBar } from "./ProgressVisuals";
import NotesPanel from "./NotesPanel";

export default function TopicTree({ nodes, skillId }: { nodes: TopicNode[]; skillId: string }) {
  return (
    <div className="tree">
      {nodes.map((n) => (
        <TopicItem key={n.id} node={n} skillId={skillId} />
      ))}
    </div>
  );
}

function formatNodeName(name: string) {
  if (name.length <= 60) return name;
  const chunks = [];
  for (let i = 0; i < name.length; i += 60) {
    chunks.push(name.substring(i, i + 60));
  }
  return (
    <>
      {chunks.map((chunk, index) => (
        <span key={index} style={{ display: "block", wordBreak: "break-word" }}>
          {chunk}
        </span>
      ))}
    </>
  );
}

function TopicItem({ node, skillId }: { node: TopicNode; skillId: string }) {
  const qc = useQueryClient();
  const [open, setOpen] = useState(false);
  const [adding, setAdding] = useState(false);
  const [childName, setChildName] = useState("");
  const [showNotes, setShowNotes] = useState(false);
  const isLeaf = isLeafNode(node);
  const notesQuery = useQuery({ queryKey: ["notes", node.id], queryFn: () => listNotes(node.id), staleTime: 2 * 60 * 1000, enabled: showNotes });
  const hasNotes = (notesQuery.data?.length ?? 0) > 0;

  const refresh = async () => {
    await Promise.all([
      qc.invalidateQueries({ queryKey: ["topics", skillId] }),
      qc.invalidateQueries({ queryKey: ["skill", skillId] }),
      qc.invalidateQueries({ queryKey: ["dashboard"] }),
      qc.invalidateQueries({ queryKey: ["skills"] }),
      qc.invalidateQueries({ queryKey: ["activity"] }),
      qc.invalidateQueries({ queryKey: ["streak-calendar"] }),
    ]);
  };

  const toggle = useMutation({
    mutationFn: (completed: boolean) => completeTopic(node.id, completed),
    onError: () => refresh(),
    onSettled: () => refresh(),
  });

  async function addChild(e?: FormEvent) {
    e?.preventDefault();
    if (!childName.trim()) return;
    const created = await createTopic({ skill_id: skillId, parent_id: node.id, name: childName.trim() });
    qc.setQueryData<TopicNode[]>(["topics", skillId], (current) =>
      current ? insertChildTopic(current, node.id, created) : current,
    );
    setChildName("");
    setAdding(false);
    setOpen(true);
    await refresh();
  }

  return (
    <div className={isLeaf ? "node node-leaf-row" : "node node-parent"}>
      <div className="node-head">
        <div className="node-main">
          {!isLeaf && (
            <button type="button" className="icon-btn" onClick={() => setOpen(!open)} aria-label={open ? "Collapse" : "Expand"}>
              {open ? "▾" : "▸"}
            </button>
          )}
          {isLeaf ? (
            <label className="node-leaf">
              <input
                type="checkbox"
                checked={node.is_completed}
                onChange={(e) => toggle.mutate(e.target.checked)}
              />
              <span className="node-name-wrapper">{formatNodeName(node.name)}</span>
            </label>
          ) : (
            <>
              <span className="node-title">{formatNodeName(node.name)}</span>
              <LineBar completed={node.completed_leaves} total={node.total_leaves} />
            </>
          )}
        </div>
        <div className="node-actions">
          <button
            type="button"
            className="btn-notes"
            style={hasNotes ? { backgroundColor: "maroon" } : undefined}
            onClick={() => setShowNotes(!showNotes)}
          >
            Notes
          </button>
          <button
            type="button"
            className="icon-btn plus"
            aria-label="Add child topic"
            onClick={() => setAdding((v) => !v)}
          >
            +
          </button>
        </div>
      </div>
      {adding && (
        <form className="row add-child" onSubmit={addChild}>
          <input
            autoFocus
            placeholder="New child topic"
            value={childName}
            onChange={(e) => setChildName(e.target.value)}
          />
          <button type="submit" className="icon-btn plus" aria-label="Save child">
            +
          </button>
        </form>
      )}
      {showNotes && <NotesPanel topicId={node.id} topicName={node.name} />}
      {open && !isLeaf && <TopicTree nodes={node.children} skillId={skillId} />}
    </div>
  );
}
