import { useEffect, useRef, useState } from "react";
import { Link, useLocation, useNavigate } from "react-router-dom";
import { useQueryClient } from "@tanstack/react-query";
import { PreviewEditor, PreviewNode } from "../components/PreviewEditor";
import { confirmSyllabus, refineSyllabus, type AITopicNode } from "../api/ai.api";
import { ApiError } from "../api/client";
import type { ProposedNode } from "../api/pdf.api";

type ChatMessage = {
  id: string;
  role: "user" | "assistant";
  content: string;
  error?: boolean;
};

type DraftState = {
  skillName: string;
  topics: ProposedNode[];
  subject: string;
  depth: string;
  purpose: string;
  timestamp?: number;
};

function convertToAITopics(nodes: ProposedNode[]): AITopicNode[] {
  return nodes.map((n) => ({
    name: n.name.trim() || "Untitled Topic",
    description: n.source ? `Source: ${n.source}` : undefined,
    children: n.children ? convertToAITopics(n.children) : [],
  }));
}

function hasEmptyTopicNames(nodes: ProposedNode[]): boolean {
  for (const n of nodes) {
    if (!n.name || !n.name.trim()) return true;
    if (n.children && hasEmptyTopicNames(n.children)) return true;
  }
  return false;
}

export default function AIPreviewPage() {
  const location = useLocation();
  const navigate = useNavigate();
  const qc = useQueryClient();

  // Try navigation state first, fall back to sessionStorage
  const [draftState, setDraftState] = useState<DraftState | null>(() => {
    const fromNav = location.state as DraftState | null;
    if (fromNav?.topics) return fromNav;
    try {
      const stored = sessionStorage.getItem("pt_ai_draft");
      if (stored) return JSON.parse(stored);
    } catch {
      // ignore
    }
    return null;
  });

  const [tree, setTree] = useState<ProposedNode>(() => ({
    name: draftState?.skillName || "Generated Syllabus",
    children: draftState?.topics || [],
  }));

  const [skillName, setSkillName] = useState(draftState?.skillName || "");

  // Staged regeneration: pending candidate that user can Accept or Discard
  const [pendingTree, setPendingTree] = useState<ProposedNode | null>(null);
  // Undo history: last accepted tree
  const [undoTree, setUndoTree] = useState<ProposedNode | null>(null);

  // Chat refinement state
  const [chatOpen, setChatOpen] = useState(true);
  const [messages, setMessages] = useState<ChatMessage[]>([]);
  const [chatInput, setChatInput] = useState("");
  const [refining, setRefining] = useState(false);
  const [chatError, setChatError] = useState("");

  // Confirmation state
  const [confirming, setConfirming] = useState(false);
  const [confirmError, setConfirmError] = useState("");

  const bottomRef = useRef<HTMLDivElement>(null);
  const chatMessagesEndRef = useRef<HTMLDivElement>(null);

  // Keep draft in sessionStorage whenever tree or skillName changes
  useEffect(() => {
    if (!draftState) return;
    const updated: DraftState = {
      ...draftState,
      skillName,
      topics: tree.children || [],
    };
    try {
      sessionStorage.setItem("pt_ai_draft", JSON.stringify(updated));
    } catch {
      // ignore quota errors
    }
  }, [tree, skillName, draftState]);

  // Scroll chat messages to bottom
  useEffect(() => {
    if (chatOpen) {
      chatMessagesEndRef.current?.scrollIntoView({ behavior: "smooth" });
    }
  }, [messages, refining, chatOpen]);

  if (!draftState || !tree.children || tree.children.length === 0) {
    return (
      <div className="grid ai-preview-page">
        <h1>AI Syllabus Preview</h1>
        <div className="card ai-preview-empty-card">
          <p className="muted">No generated syllabus found in this session.</p>
          <button
            type="button"
            className="primary"
            onClick={() => navigate("/generate")}
            style={{ marginTop: "1rem" }}
          >
            Go to Generate using AI
          </button>
        </div>
      </div>
    );
  }

  const activeTree = pendingTree || tree;

  async function handleRefine(e?: React.FormEvent) {
    if (e) e.preventDefault();
    if (!draftState) return;
    const instruction = chatInput.trim();
    if (!instruction || refining) return;

    setChatError("");
    const userMsgId = `user_${Date.now()}`;
    const userMsg: ChatMessage = { id: userMsgId, role: "user", content: instruction };

    setMessages((prev) => [...prev, userMsg]);
    setChatInput("");
    setRefining(true);

    try {
      // Build conversation history for context
      const history = messages
        .filter((m) => !m.error)
        .map((m) => ({ role: m.role, content: m.content }));

      // Format current tree including ALL manual user edits!
      const currentPayload = {
        skill_name: skillName,
        topics: convertToAITopics(tree.children || []),
      };

      const result = await refineSyllabus(
        draftState.subject,
        draftState.depth,
        draftState.purpose,
        currentPayload,
        history,
        instruction
      );

      const candidateTree: ProposedNode = {
        name: result.skill_name || skillName,
        children: result.topics as ProposedNode[],
      };

      setPendingTree(candidateTree);

      const botMsg: ChatMessage = {
        id: `bot_${Date.now()}`,
        role: "assistant",
        content: `I've updated the syllabus based on: "${instruction}". Review the changes in the preview. Click "Accept" to apply or "Discard" to revert.`,
      };
      setMessages((prev) => [...prev, botMsg]);
    } catch (err) {
      const errMsg =
        err instanceof ApiError
          ? err.message
          : "Failed to refine syllabus. Please try again with a different instruction.";
      setChatError(errMsg);
      setMessages((prev) => [
        ...prev,
        {
          id: `bot_err_${Date.now()}`,
          role: "assistant",
          content: errMsg,
          error: true,
        },
      ]);
    } finally {
      setRefining(false);
    }
  }

  function handleAcceptPending() {
    if (!pendingTree) return;
    setUndoTree(tree);
    setTree(pendingTree);
    if (pendingTree.name) {
      setSkillName(pendingTree.name);
    }
    setPendingTree(null);
  }

  function handleDiscardPending() {
    setPendingTree(null);
  }

  function handleUndo() {
    if (!undoTree) return;
    setTree(undoTree);
    setUndoTree(null);
  }

  async function handleConfirm() {
    const finalSkillName = skillName.trim();
    if (!finalSkillName) {
      setConfirmError("Please provide a skill name before confirming.");
      return;
    }
    const currentNodes = activeTree.children || [];
    if (currentNodes.length === 0) {
      setConfirmError("Cannot create a skill without topics. Please add at least one topic.");
      return;
    }
    if (hasEmptyTopicNames(currentNodes)) {
      setConfirmError("Please enter names for all topics or remove empty ones before confirming.");
      return;
    }

    setConfirmError("");
    setConfirming(true);

    try {
      const topicsPayload = convertToAITopics(currentNodes);
      const created = await confirmSyllabus(finalSkillName, topicsPayload);

      // Invalidate queries so new skill and updated dashboard appear immediately
      await Promise.all([
        qc.invalidateQueries({ queryKey: ["skills"] }),
        qc.invalidateQueries({ queryKey: ["dashboard"] }),
        qc.invalidateQueries({ queryKey: ["activity"] }),
        qc.invalidateQueries({ queryKey: ["streak"] }),
      ]);

      // Clear draft storage
      sessionStorage.removeItem("pt_ai_draft");

      navigate(`/skills/${created.id}`);
    } catch (err) {
      setConfirming(false);
      setConfirmError(
        err instanceof ApiError
          ? err.message
          : "Failed to create skill. Please try again."
      );
    }
  }

  return (
    <div className="grid ai-preview-page">
      <div className="ai-preview-title-row">
        <div>
          <h1>AI-generated syllabus preview</h1>
          <p className="muted">
            Generated for <strong>{draftState.subject}</strong> ({draftState.depth === "intermediate" ? "hard" : draftState.depth} depth, {draftState.purpose.replace("_", " ")} purpose).
            Review, edit manually, or refine with AI. Nothing is saved until you confirm.
          </p>
        </div>
        <div className="ai-preview-top-actions">
          <button
            type="button"
            className={`ai-chat-toggle-btn ${chatOpen ? "active" : ""}`}
            onClick={() => setChatOpen(!chatOpen)}
            aria-label="Toggle AI Refinement chat panel"
          >
            {chatOpen ? "Hide AI Assistant" : "Refine with AI ✦"}
          </button>
        </div>
      </div>

      {pendingTree && (
        <div className="card ai-pending-banner">
          <div className="ai-pending-text">
            <strong>Reviewing AI Modifications:</strong> Showing proposed changes. Would you like to accept or discard this version?
          </div>
          <div className="row ai-pending-buttons">
            <button type="button" className="primary" onClick={handleAcceptPending}>
              Accept Changes
            </button>
            <button type="button" className="danger" onClick={handleDiscardPending}>
              Discard Changes
            </button>
          </div>
        </div>
      )}

      {undoTree && !pendingTree && (
        <div className="card ai-undo-banner">
          <span>Changes accepted. Need to go back?</span>
          <button type="button" onClick={handleUndo}>
            Undo Last Acceptance
          </button>
        </div>
      )}

      <div className={`ai-preview-grid ${chatOpen ? "chat-visible" : ""}`}>
        {/* Hierarchical Topic Editor */}
        <div className="ai-preview-main">
          <div className="card grid import-preview-card">
            <div className="import-preview-header">
              <div>
                <h3>Syllabus Structure</h3>
                <p className="muted">
                  Click on any topic name to edit it, use + Add child to nest subtopics, or Delete to remove topics.
                </p>
              </div>
              <button
                type="button"
                className="scroll-end-btn"
                onClick={() => bottomRef.current?.scrollIntoView({ behavior: "smooth" })}
              >
                Scroll to end ↓
              </button>
            </div>

            <div className="ai-skill-name-field">
              <label htmlFor="ai-skill-name-input" className="generate-label">
                Skill Name
              </label>
              <input
                id="ai-skill-name-input"
                placeholder="Skill name"
                value={skillName}
                onChange={(e) => setSkillName(e.target.value)}
                maxLength={200}
                aria-label="Skill name"
              />
            </div>

            {confirmError && (
              <p className="error ai-confirm-error" role="alert">
                {confirmError}
              </p>
            )}

            <div className="preview-tree-wrap">
              {(activeTree.children ?? []).map((rootNode, i) => (
                <PreviewNode
                  key={i}
                  node={rootNode}
                  onChange={(updated) => {
                    const next = [...(activeTree.children ?? [])];
                    next[i] = updated;
                    const nextTree = { ...activeTree, children: next };
                    if (pendingTree) setPendingTree(nextTree);
                    else setTree(nextTree);
                  }}
                  onDelete={() => {
                    const next = (activeTree.children ?? []).filter((_, idx) => idx !== i);
                    const nextTree = { ...activeTree, children: next };
                    if (pendingTree) setPendingTree(nextTree);
                    else setTree(nextTree);
                  }}
                />
              ))}
              <button
                type="button"
                className="preview-add-root-btn"
                onClick={() => {
                  const next = [...(activeTree.children ?? []), { name: "New Topic", children: [] }];
                  const nextTree = { ...activeTree, children: next };
                  if (pendingTree) setPendingTree(nextTree);
                  else setTree(nextTree);
                }}
              >
                + Add Topic
              </button>
            </div>

            <div className="row preview-footer-row" ref={bottomRef}>
              <Link to="/generate" className="muted" style={{ marginRight: "auto" }}>
                ← Start Over with Different Options
              </Link>

              {pendingTree ? (
                <>
                  <button type="button" className="danger" onClick={handleDiscardPending}>
                    Discard
                  </button>
                  <button type="button" className="primary" onClick={handleAcceptPending}>
                    Accept & Continue
                  </button>
                </>
              ) : (
                <button
                  type="button"
                  className="primary ai-confirm-btn"
                  onClick={handleConfirm}
                  disabled={confirming}
                >
                  {confirming ? "Creating Skill…" : "Confirm & Create Skill"}
                </button>
              )}
            </div>
          </div>
        </div>

        {/* Chatbot-style Refinement Panel */}
        {chatOpen && (
          <aside className="card ai-chat-panel" aria-label="AI Refinement Assistant">
            <div className="ai-chat-header">
              <div className="ai-chat-header-title">
                <span className="ai-chat-sparkle">✦</span>
                <strong>Refine with AI</strong>
              </div>
              <button
                type="button"
                className="ai-chat-close-btn"
                onClick={() => setChatOpen(false)}
                title="Hide AI Assistant"
                aria-label="Close refinement panel"
              >
                ✕
              </button>
            </div>

            <div className="ai-chat-messages-container">
              <div className="ai-chat-welcome muted">
                <p>Tell AI how to adjust your syllabus. For example:</p>
                <ul>
                  <li>"Add more advanced concepts to topic 2"</li>
                  <li>"Remove unnecessary beginner topics"</li>
                  <li>"Expand topic 1.2 into more detailed subtopics"</li>
                  <li>"Add practical hands-on implementation topics"</li>
                  <li>"Reorganize hierarchy logically for interviews"</li>
                </ul>
              </div>

              {messages.map((msg) => (
                <div
                  key={msg.id}
                  className={`ai-chat-bubble ${msg.role === "user" ? "user-bubble" : "bot-bubble"} ${
                    msg.error ? "bubble-error" : ""
                  }`}
                >
                  <span className="ai-bubble-sender">
                    {msg.role === "user" ? "You" : "AI"}
                  </span>
                  <div className="ai-bubble-content">{msg.content}</div>
                </div>
              ))}

              {refining && (
                <div className="ai-chat-bubble bot-bubble refining-bubble">
                  <span className="ai-bubble-sender">AI</span>
                  <div className="ai-bubble-content">
                    <span className="generate-spinner" /> Updating syllabus hierarchy…
                  </div>
                </div>
              )}

              <div ref={chatMessagesEndRef} />
            </div>

            <form onSubmit={handleRefine} className="ai-chat-input-form">
              {chatError && <p className="error ai-chat-error-text">{chatError}</p>}
              <div className="ai-chat-input-row">
                <textarea
                  className="ai-chat-textarea"
                  placeholder="Ask AI to add, remove, or modify topics…"
                  value={chatInput}
                  onChange={(e) => setChatInput(e.target.value)}
                  onKeyDown={(e) => {
                    if (e.key === "Enter" && !e.shiftKey) {
                      e.preventDefault();
                      handleRefine();
                    }
                  }}
                  disabled={refining}
                  rows={2}
                  maxLength={2000}
                  aria-label="Refinement instruction"
                />
                <button
                  type="submit"
                  className="primary ai-chat-send-btn"
                  disabled={refining || !chatInput.trim()}
                  title="Send instruction (Enter)"
                >
                  {refining ? "…" : "Send"}
                </button>
              </div>
              <span className="muted ai-chat-hint">Press Enter to send, Shift+Enter for newline</span>
            </form>
          </aside>
        )}
      </div>
    </div>
  );
}
