import { useState, type ReactNode } from "react";
import type { ProposedNode } from "../api/pdf.api";

export function PreviewNode({
  node,
  onChange,
  onDelete,
}: {
  node: ProposedNode;
  onChange: (n: ProposedNode) => void;
  onDelete?: () => void;
}) {
  const [collapsed, setCollapsed] = useState(false);
  const kids = node.children ?? [];
  const hasKids = kids.length > 0;

  return (
    <div className={`node preview-node ${node.confidence === "low" ? "low" : ""}`}>
      <div className="node-head preview-node-head">
        {hasKids ? (
          <button
            type="button"
            className="preview-collapse-btn"
            onClick={() => setCollapsed(!collapsed)}
            aria-label={collapsed ? "Expand topic branch" : "Collapse topic branch"}
            title={collapsed ? "Expand branch" : "Collapse branch"}
          >
            {collapsed ? "▸" : "▾"}
          </button>
        ) : (
          <span className="preview-collapse-spacer" />
        )}
        <input
          value={node.name}
          onChange={(e) => onChange({ ...node, name: e.target.value })}
          placeholder="Topic name"
          aria-label="Topic name"
        />
        {node.confidence === "low" && <span className="muted preview-low-label">low confidence</span>}
        <button
          type="button"
          className="danger"
          onClick={() => onChange({ ...node, name: "" })}
          title="Clear topic name"
        >
          Clear name
        </button>
        {onDelete && (
          <button
            type="button"
            className="danger preview-delete-btn"
            onClick={onDelete}
            title="Delete this topic and its subtopics"
          >
            Delete
          </button>
        )}
      </div>

      {!collapsed && (
        <div className="preview-children">
          {kids.map((c, i) => (
            <PreviewNode
              key={i}
              node={c}
              onChange={(updated) => {
                const next = [...kids];
                next[i] = updated;
                onChange({ ...node, children: next });
              }}
              onDelete={() => {
                const next = kids.filter((_, idx) => idx !== i);
                onChange({ ...node, children: next });
              }}
            />
          ))}
          <button
            type="button"
            className="preview-add-child-btn"
            onClick={() => onChange({ ...node, children: [...kids, { name: "New topic", children: [] }] })}
          >
            + Add child
          </button>
        </div>
      )}
    </div>
  );
}

export function PreviewEditor({
  tree,
  onTreeChange,
  skillName,
  onSkillNameChange,
  title = "Preview editor",
  subtitle = "Nothing is saved until you confirm.",
  confirmLabel = "Confirm import",
  onConfirm,
  confirmDisabled = false,
  extraActions,
  bottomRef,
}: {
  tree: ProposedNode;
  onTreeChange: (tree: ProposedNode) => void;
  skillName: string;
  onSkillNameChange: (name: string) => void;
  title?: string;
  subtitle?: string;
  confirmLabel?: string;
  onConfirm: () => void;
  confirmDisabled?: boolean;
  extraActions?: ReactNode;
  bottomRef?: React.RefObject<HTMLDivElement | null>;
}) {
  return (
    <div className="card grid import-preview-card">
      <div className="import-preview-header">
        <div>
          <h3>{title}</h3>
          <p className="muted">{subtitle}</p>
        </div>
        <button
          type="button"
          className="scroll-end-btn"
          onClick={() => bottomRef?.current?.scrollIntoView({ behavior: "smooth" })}
        >
          Scroll to end ↓
        </button>
      </div>

      <input
        placeholder="Skill name"
        value={skillName}
        onChange={(e) => onSkillNameChange(e.target.value)}
        maxLength={200}
        aria-label="Skill name"
      />

      <div className="preview-tree-wrap">
        <PreviewNode
          node={tree}
          onChange={onTreeChange}
        />
      </div>

      <div className="row preview-footer-row" ref={bottomRef}>
        {extraActions}
        <button
          type="button"
          className="primary"
          onClick={onConfirm}
          disabled={confirmDisabled}
        >
          {confirmLabel}
        </button>
      </div>
    </div>
  );
}

