import { useState, useEffect, useCallback, type ReactNode } from "react";
import { Link, useNavigate } from "react-router-dom";
import { getAccess } from "../api/client";
import "./HomePage.css";

type FeatureItem = {
  id: string;
  pill: string;
  category: string;
  title: string;
  tagline: string;
  description: string;
  highlights: string[];
  ctaLabel: string;
  ctaLink: string;
  renderVisual: () => ReactNode;
};

export default function HomePage() {
  const navigate = useNavigate();
  const [activeIdx, setActiveIdx] = useState(0);
  const [isAutoPlay, setIsAutoPlay] = useState(true);
  const [isHovered, setIsHovered] = useState(false);
  const isAuthenticated = Boolean(getAccess());

  const features: FeatureItem[] = [
    {
      id: "ai-gen",
      pill: "AI Generator",
      category: "AI Intelligence",
      title: "Automated AI Syllabus Generator",
      tagline: "Turn any subject into an exhaustive, structured learning roadmap in seconds.",
      description:
        "Powered by Google Gemini AI, generate complete hierarchical course curricula instantly. Tailor depth across 4 distinct tiers and calibrate topics precisely according to your specific learning purpose.",
      highlights: [
        "4 Granular Depth Levels: Easy (Beginner), Medium (Core), Hard (In-Depth), and Comprehensive (A-to-Z)",
        "3 Purpose-Driven Modes: Technical Interview Prep, Quiz/Exam Revision, and A-to-Z Deep Mastery",
        "Dynamic topic taxonomy analyzing fundamental concepts, mechanisms, and advanced real-world edge cases",
        "Built-in SlowAPI rate limiting protection ensuring stable, high-availability generation",
      ],
      ctaLabel: "Generate a Syllabus",
      ctaLink: isAuthenticated ? "/generate" : "/login?mode=register",
      renderVisual: () => (
        <div className="home-mockup-wrap">
          <div className="mockup-ai-gen">
            <div className="mockup-input-box">
              <span>Python & Backend Architecture</span>
              <span style={{ color: "#5eead4", fontSize: "0.75rem", fontWeight: 600 }}>Gemini 2.5</span>
            </div>
            <div style={{ display: "flex", flexDirection: "column", gap: "0.3rem" }}>
              <span className="muted" style={{ fontSize: "0.75rem" }}>Depth Tier:</span>
              <div className="mockup-chips">
                <span className="mockup-chip">Easy</span>
                <span className="mockup-chip">Medium</span>
                <span className="mockup-chip">Hard</span>
                <span className="mockup-chip selected">★ Comprehensive</span>
              </div>
            </div>
            <div style={{ display: "flex", flexDirection: "column", gap: "0.3rem" }}>
              <span className="muted" style={{ fontSize: "0.75rem" }}>Target Purpose:</span>
              <div className="mockup-chips">
                <span className="mockup-chip selected">💼 Interview Prep</span>
                <span className="mockup-chip">📝 Quiz Prep</span>
                <span className="mockup-chip">🌐 A-to-Z Knowledge</span>
              </div>
            </div>
            <div className="mockup-pulse-status">
              <span className="mockup-pulse-dot" />
              <span>Analyzing subject structure & organizing hierarchical topic tree…</span>
            </div>
          </div>
        </div>
      ),
    },
    {
      id: "ai-studio",
      pill: "AI Studio",
      category: "AI Intelligence",
      title: "Interactive Syllabus Studio & AI Chat Refinement",
      tagline: "Inspect, prune, edit, and conversationally refine your syllabus before saving.",
      description:
        "Never get locked into a static syllabus. The AI Preview Studio lets you inspect proposed hierarchies, chat directly with Gemini to add or remove concepts, accept or discard proposals, and undo changes with ease.",
      highlights: [
        "Conversational AI Chat Refinement: type prompts like 'Add Docker and Kubernetes' or 'Focus on Async IO'",
        "Two-stage candidate validation: review proposed tree changes and click 'Accept' or 'Discard'",
        "Full Undo history: safely revert any chat refinement back to previous tree states",
        "Visual tree editing: rename nodes inline, add custom child subtopics, or delete unwanted branches",
      ],
      ctaLabel: "Explore AI Studio",
      ctaLink: isAuthenticated ? "/generate" : "/login?mode=register",
      renderVisual: () => (
        <div className="home-mockup-wrap">
          <div style={{ display: "flex", flexDirection: "column", gap: "0.7rem" }}>
            <div style={{ background: "#151d26", padding: "0.6rem 0.8rem", borderRadius: "8px", border: "1px solid #283747" }}>
              <div style={{ display: "flex", justifyContent: "space-between", marginBottom: "0.3rem" }}>
                <span style={{ fontSize: "0.78rem", color: "#38bdf8", fontWeight: 600 }}>Proposed Tree Changes</span>
                <div style={{ display: "flex", gap: "0.4rem" }}>
                  <span style={{ background: "#134e4a", color: "#5eead4", fontSize: "0.7rem", padding: "0.1rem 0.4rem", borderRadius: "4px" }}>✓ Accept</span>
                  <span style={{ background: "#3f1d24", color: "#fb7185", fontSize: "0.7rem", padding: "0.1rem 0.4rem", borderRadius: "4px" }}>✕ Discard</span>
                  <span style={{ background: "#243040", color: "#93a4b5", fontSize: "0.7rem", padding: "0.1rem 0.4rem", borderRadius: "4px" }}>↩ Undo</span>
                </div>
              </div>
              <div style={{ fontSize: "0.75rem", color: "#cbd5e1" }}>+ Added: Containerization & Cloud Deployment (3 subtopics)</div>
            </div>
            <div style={{ background: "#121921", padding: "0.6rem 0.8rem", borderRadius: "8px", border: "1px solid #202b38" }}>
              <div style={{ fontSize: "0.72rem", color: "#93a4b5", marginBottom: "0.2rem" }}>AI Refinement Chat</div>
              <div style={{ fontSize: "0.8rem", color: "#e2e8f0", background: "#1b2532", padding: "0.4rem 0.6rem", borderRadius: "6px" }}>
                💬 "Add microservices communication patterns and gRPC"
              </div>
            </div>
          </div>
        </div>
      ),
    },
    {
      id: "pdf-import",
      pill: "PDF Import",
      category: "Document Ingestion",
      title: "PDF Syllabus Parser & Hierarchical Ingestion",
      tagline: "Turn academic course sheets and PDF documents into living, trackable trees.",
      description:
        "Upload any syllabus PDF up to 10MB. Our document extraction pipeline parses structured outlines, identifies numerical hierarchies (1 → 1.1 → 1.2.1), and converts static pages into interactive progress checklists.",
      highlights: [
        "Drag-and-drop document upload supporting syllabus PDFs up to 10MB",
        "Intelligent text extraction identifying standardized numerical outlines (1, 1.1, 1.2)",
        "Asynchronous job processing with real-time status: Uploaded → Extracting → Parsing → Ready",
        "Includes copyable prompt templates to easily format syllabi generated from external AI tools",
      ],
      ctaLabel: "Upload Syllabus PDF",
      ctaLink: isAuthenticated ? "/import" : "/login?mode=register",
      renderVisual: () => (
        <div className="home-mockup-wrap">
          <div style={{ border: "2px dashed #2c3846", borderRadius: "10px", padding: "1.2rem", textAlign: "center", background: "#141c24" }}>
            <div style={{ width: "42px", height: "42px", borderRadius: "8px", background: "#ea580c", color: "#fff", display: "inline-flex", alignItems: "center", justifyContent: "center", fontWeight: 700, fontSize: "0.85rem", marginBottom: "0.5rem" }}>
              PDF
            </div>
            <div style={{ fontSize: "0.88rem", fontWeight: 600, color: "#ffffff" }}>Drop Syllabus PDF here</div>
            <div style={{ fontSize: "0.75rem", color: "#93a4b5", margin: "0.25rem 0 0.8rem" }}>Up to 10 MB in standard text PDF format</div>
            <span style={{ background: "#243040", color: "#5eead4", padding: "0.35rem 0.8rem", borderRadius: "6px", fontSize: "0.78rem", border: "1px solid #3b4e63" }}>
              Upload to edit
            </span>
          </div>
        </div>
      ),
    },
    {
      id: "topic-tree",
      pill: "Topic Tree",
      category: "Learning Core",
      title: "Multi-Level Hierarchical Topic Tree",
      tagline: "Organize subjects into collapsible modules, chapters, and checkable leaf topics.",
      description:
        "Tackle large, intimidating skills step-by-step. The deep topic tree groups your curriculum into expandable parent modules and granular leaf topics with live progress rollups.",
      highlights: [
        "N-level nesting: break down any skill into modules, subtopics, and granular study tasks",
        "Collapsible parent nodes with smooth accordion arrows for clean, uncluttered navigation",
        "Visual LineBar indicators displaying completed vs total leaves for every single branch",
        "One-click checkbox toggles that update parent branch progress and overall completion instantly",
      ],
      ctaLabel: "Explore Topic Tree",
      ctaLink: isAuthenticated ? "/skills" : "/login?mode=register",
      renderVisual: () => (
        <div className="home-mockup-wrap">
          <div className="mockup-tree">
            <div className="mockup-tree-parent">
              <span style={{ fontWeight: 600, color: "#ffffff" }}>▾ 1. System Design Principles</span>
              <span style={{ fontSize: "0.75rem", color: "#5eead4" }}>3 / 4 leaves</span>
            </div>
            <div className="mockup-tree-leaf">
              <div className="mockup-check-box checked">✓</div>
              <span>1.1 Horizontal vs Vertical Scaling</span>
            </div>
            <div className="mockup-tree-leaf">
              <div className="mockup-check-box checked">✓</div>
              <span>1.2 CAP Theorem & Distributed Consistency</span>
            </div>
            <div className="mockup-tree-leaf">
              <div className="mockup-check-box checked">✓</div>
              <span>1.3 Caching Strategies (Redis / Memcached)</span>
            </div>
            <div className="mockup-tree-leaf">
              <div className="mockup-check-box"></div>
              <span>1.4 Message Queues & Event Streaming</span>
            </div>
          </div>
        </div>
      ),
    },
    {
      id: "countdown-timer",
      pill: "Goal Countdown",
      category: "Pacing & Deadlines",
      title: "Target Timeline & Goal Countdown Timer",
      tagline: "Set target mastery dates and stay accountable with animated timeline gauges.",
      description:
        "Turn vague goals into concrete deadlines. Attach a target date range ('From' and 'To') to any skill to track elapsed days against total duration and view human-friendly countdown readouts.",
      highlights: [
        "Interactive From/To date selector with automatic future calendar day validation",
        "Visual progress gauge showing elapsed vs total duration (e.g. 42 / 90 days)",
        "Dynamic countdown readout: '2 months 14 days remaining' or 'Goal completed'",
        "Future schedule support with 'Timer will start from YYYY-MM-DD' waiting indicator",
      ],
      ctaLabel: "Set Target Deadlines",
      ctaLink: isAuthenticated ? "/skills" : "/login?mode=register",
      renderVisual: () => (
        <div className="home-mockup-wrap">
          <div className="mockup-timer">
            <span style={{ fontSize: "0.8rem", color: "#93a4b5" }}>Skill Completion Goal</span>
            <div className="mockup-timer-bar-wrap">
              <div className="mockup-timer-bar-fill" />
            </div>
            <div className="mockup-timer-text">1 month 22 days remaining</div>
            <div style={{ display: "flex", justifyContent: "space-between", width: "100%", fontSize: "0.75rem", color: "#64748b" }}>
              <span>From: 01-Nov-2026</span>
              <span>To: 31-Dec-2026</span>
            </div>
          </div>
        </div>
      ),
    },
    {
      id: "topic-notes",
      pill: "Topic Notes",
      category: "Study Notes",
      title: "In-Context Hierarchical Topic Notes",
      tagline: "Capture revision notes, code snippets, and key formulas directly on topics.",
      description:
        "Keep your notes right where they belong. The embedded Notes Drawer slides open next to any topic in your tree, letting you record notes, solutions, and reference links as you study.",
      highlights: [
        "Embedded slide-out Notes Drawer accessible without navigating away from your syllabus tree",
        "Add, view, edit, and delete revision notes attached to specific learning checkpoints",
        "Timestamped note histories documenting when concepts were studied and updated",
        "Fully indexed in unified search for instant retrieval across all your skills",
      ],
      ctaLabel: "Try Topic Notes",
      ctaLink: isAuthenticated ? "/skills" : "/login?mode=register",
      renderVisual: () => (
        <div className="home-mockup-wrap">
          <div style={{ background: "#151e28", border: "1px solid #283747", borderRadius: "10px", padding: "0.85rem" }}>
            <div style={{ display: "flex", justifyContent: "space-between", alignItems: "center", marginBottom: "0.5rem" }}>
              <span style={{ fontSize: "0.8rem", fontWeight: 600, color: "#38bdf8" }}>📝 Topic Note: B-Tree Indexes</span>
              <span style={{ fontSize: "0.7rem", color: "#64748b" }}>Nov 4, 2026</span>
            </div>
            <div style={{ fontSize: "0.78rem", color: "#cbd5e1", lineHeight: 1.45, background: "#111821", padding: "0.6rem", borderRadius: "6px", border: "1px solid #1f2a37" }}>
              "Composite indexes (a, b) satisfy queries filtering on (a) or (a, b), but not (b) alone. Prefer index covering for high-traffic read paths."
            </div>
            <div style={{ display: "flex", gap: "0.4rem", marginTop: "0.5rem", justifyContent: "flex-end" }}>
              <span style={{ fontSize: "0.72rem", color: "#5eead4", cursor: "pointer" }}>Edit</span>
              <span style={{ fontSize: "0.72rem", color: "#fb7185", cursor: "pointer" }}>Delete</span>
            </div>
          </div>
        </div>
      ),
    },
    {
      id: "personal-notes",
      pill: "Make Note",
      category: "Productivity",
      title: "Personal Scratchpad & Interactive Checklists",
      tagline: "A dedicated daily scratchpad for quick study to-dos, checklists, and revision lists.",
      description:
        "Separate your daily tasks from long-term syllabi. 'Make Note' gives you independent task lists with auto-saving, strike-through task completion, and custom note titles.",
      highlights: [
        "Multiple standalone notes with custom titles up to 35 characters",
        "Interactive task checklist with one-click strike-through completion status",
        "Automated background auto-saving with 450ms debounce and instant 'Saved' feedback",
        "Keyboard-friendly task addition and deletion for frictionless thought capture",
      ],
      ctaLabel: "Open Make Note",
      ctaLink: isAuthenticated ? "/make-note" : "/login?mode=register",
      renderVisual: () => (
        <div className="home-mockup-wrap">
          <div style={{ background: "#141c25", border: "1px solid #283747", borderRadius: "10px", padding: "0.85rem" }}>
            <div style={{ display: "flex", justifyContent: "space-between", alignItems: "center", marginBottom: "0.6rem" }}>
              <span style={{ fontWeight: 600, color: "#ffffff", fontSize: "0.85rem" }}>Daily Study Checklist</span>
              <span style={{ color: "#4ade80", fontSize: "0.72rem", display: "flex", alignItems: "center", gap: "3px" }}>● Saved</span>
            </div>
            <div style={{ display: "flex", flexDirection: "column", gap: "0.45rem", fontSize: "0.8rem" }}>
              <div style={{ display: "flex", alignItems: "center", gap: "0.5rem", color: "#94a3b8", textDecoration: "line-through" }}>
                <span style={{ color: "#5eead4" }}>☑</span> Read Chapter 4: Database Transactions
              </div>
              <div style={{ display: "flex", alignItems: "center", gap: "0.5rem", color: "#94a3b8", textDecoration: "line-through" }}>
                <span style={{ color: "#5eead4" }}>☑</span> Practice 2 Dynamic Programming problems
              </div>
              <div style={{ display: "flex", alignItems: "center", gap: "0.5rem", color: "#e2e8f0" }}>
                <span style={{ color: "#64748b" }}>☐</span> Implement WebSocket real-time broadcast
              </div>
            </div>
          </div>
        </div>
      ),
    },
    {
      id: "analytics-dash",
      pill: "Dashboard",
      category: "Analytics",
      title: "Visual Progress Analytics & Executive Dashboard",
      tagline: "Your central command center for measuring momentum, completed leaves, and overall mastery.",
      description:
        "Get an instant high-level overview of your entire learning journey. The dashboard combines circular progress gauges, high-level metrics, and consistency stats into one inspiring command center.",
      highlights: [
        "SVG Animated Circular Progress Ring ('OverallProgressRing') displaying global mastery percentage",
        "High-level metric cards: Total Skills, Total Topics, Completed Leaves, and Current Streak",
        "Helpful empty-state guidance with quick one-click shortcuts to Import PDF or Create Skill",
        "Optimized layout with context-menu protections for clean, distraction-free metrics inspection",
      ],
      ctaLabel: "View Dashboard",
      ctaLink: isAuthenticated ? "/dashboard" : "/login",
      renderVisual: () => (
        <div className="home-mockup-wrap">
          <div style={{ display: "flex", flexDirection: "column", gap: "0.7rem", alignItems: "center" }}>
            <div style={{ position: "relative", width: "95px", height: "95px", display: "flex", alignItems: "center", justifyContent: "center" }}>
              <svg viewBox="0 0 100 100" style={{ width: "100%", height: "100%", transform: "rotate(-90deg)" }}>
                <circle cx="50" cy="50" r="42" fill="none" stroke="#1f2937" strokeWidth="8" />
                <circle cx="50" cy="50" r="42" fill="none" stroke="url(#grad)" strokeWidth="8" strokeDasharray="264" strokeDashoffset="66" strokeLinecap="round" />
                <defs>
                  <linearGradient id="grad" x1="0%" y1="0%" x2="100%" y2="100%">
                    <stop offset="0%" stopColor="#5eead4" />
                    <stop offset="100%" stopColor="#38bdf8" />
                  </linearGradient>
                </defs>
              </svg>
              <span style={{ position: "absolute", fontSize: "1.1rem", fontWeight: 700, color: "#ffffff" }}>75%</span>
            </div>
            <div className="mockup-stats-grid" style={{ width: "100%" }}>
              <div className="mockup-stat-card">
                <span className="mockup-stat-num">6</span>
                <span className="mockup-stat-lbl">👥 Skills</span>
              </div>
              <div className="mockup-stat-card">
                <span className="mockup-stat-num">142</span>
                <span className="mockup-stat-lbl">📚 Topics</span>
              </div>
              <div className="mockup-stat-card">
                <span className="mockup-stat-num">107</span>
                <span className="mockup-stat-lbl">✅ Completed</span>
              </div>
              <div className="mockup-stat-card">
                <span className="mockup-stat-num">18</span>
                <span className="mockup-stat-lbl">🔥 Streak</span>
              </div>
            </div>
          </div>
        </div>
      ),
    },
    {
      id: "consistency-heatmap",
      pill: "Consistency Heatmap",
      category: "Habit Building",
      title: "GitHub-Style Consistency Heatmap & Streak Calendar",
      tagline: "Build unbreakable daily study habits with an interactive contribution matrix.",
      description:
        "Witness your daily discipline compound over time. Inspired by GitHub's contribution matrix, the streak calendar visualizes every study action across weeks and months with 4-level color intensity.",
      highlights: [
        "12-month rolling contribution matrix with 4 distinct activity intensity levels",
        "Dynamic hover tooltips showing exact calendar dates and completed action counts",
        "Multi-year filter: inspect current rolling window or jump back to historical years",
        "Streak metrics: Active streak counter, all-time record, and active study days",
      ],
      ctaLabel: "Check Your Streak",
      ctaLink: isAuthenticated ? "/streak" : "/login?mode=register",
      renderVisual: () => (
        <div className="home-mockup-wrap">
          <div className="mockup-heatmap">
            <div style={{ display: "flex", justifyContent: "space-between", alignItems: "center" }}>
              <span style={{ fontSize: "0.75rem", color: "#93a4b5" }}>Consistency Matrix (2026)</span>
              <span style={{ fontSize: "0.75rem", color: "#5eead4", fontWeight: 600 }}>🔥 18-day streak</span>
            </div>
            <div className="mockup-heatmap-grid">
              {Array.from({ length: 70 }).map((_, i) => {
                const lvl = (i % 7 === 1 || i % 5 === 0) ? "lvl-3" : (i % 3 === 0) ? "lvl-2" : (i % 4 === 0) ? "lvl-4" : (i % 2 === 0) ? "lvl-1" : "";
                return <div key={i} className={`mockup-heatmap-cell ${lvl}`} />;
              })}
            </div>
            <div style={{ display: "flex", justifyContent: "space-between", fontSize: "0.7rem", color: "#64748b" }}>
              <span>Less</span>
              <div style={{ display: "flex", gap: "3px", alignItems: "center" }}>
                <span className="mockup-heatmap-cell" style={{ width: "9px", height: "9px" }} />
                <span className="mockup-heatmap-cell lvl-1" style={{ width: "9px", height: "9px" }} />
                <span className="mockup-heatmap-cell lvl-2" style={{ width: "9px", height: "9px" }} />
                <span className="mockup-heatmap-cell lvl-3" style={{ width: "9px", height: "9px" }} />
                <span className="mockup-heatmap-cell lvl-4" style={{ width: "9px", height: "9px" }} />
              </div>
              <span>More</span>
            </div>
          </div>
        </div>
      ),
    },
    {
      id: "activity-log",
      pill: "Activity Log",
      category: "Audit & History",
      title: "Chronological Activity Feed & Audit Trail",
      tagline: "A verifiable, timestamped record of every milestone, note, and topic completed.",
      description:
        "Track your momentum down to the exact second. The activity feed logs every topic completed, uncompleted, skill created, PDF parsed, or note saved in reverse chronological order.",
      highlights: [
        "Reverse chronological timeline showcasing your latest learning milestones",
        "Detailed event context snapshotting topic, parent module, and skill names",
        "Localized timestamps formatted according to your chosen timezone",
        "Audit database powering the consistency engine and streak calculation",
      ],
      ctaLabel: "View Activity Log",
      ctaLink: isAuthenticated ? "/activity" : "/login?mode=register",
      renderVisual: () => (
        <div className="home-mockup-wrap">
          <div style={{ display: "flex", flexDirection: "column", gap: "0.55rem" }}>
            <div style={{ background: "#151e28", padding: "0.5rem 0.75rem", borderRadius: "6px", borderLeft: "3px solid #5eead4", fontSize: "0.78rem" }}>
              <div style={{ color: "#ffffff", fontWeight: 500 }}>Completed 'Redis Caching Strategies'</div>
              <div style={{ color: "#64748b", fontSize: "0.7rem" }}>Backend Architecture • 2 hours ago</div>
            </div>
            <div style={{ background: "#151e28", padding: "0.5rem 0.75rem", borderRadius: "6px", borderLeft: "3px solid #38bdf8", fontSize: "0.78rem" }}>
              <div style={{ color: "#ffffff", fontWeight: 500 }}>Added note to 'PostgreSQL Partitioning'</div>
              <div style={{ color: "#64748b", fontSize: "0.7rem" }}>Database Mastery • Yesterday, 4:15 PM</div>
            </div>
            <div style={{ background: "#151e28", padding: "0.5rem 0.75rem", borderRadius: "6px", borderLeft: "3px solid #fbbf24", fontSize: "0.78rem" }}>
              <div style={{ color: "#ffffff", fontWeight: 500 }}>Generated AI Syllabus 'Go Concurrency'</div>
              <div style={{ color: "#64748b", fontSize: "0.7rem" }}>AI Generator • 3 days ago</div>
            </div>
          </div>
        </div>
      ),
    },
    {
      id: "global-search",
      pill: "Global Search",
      category: "Knowledge Search",
      title: "Instant Multi-Entity Global Search Engine",
      tagline: "Find any skill, topic name, description, study note, or scratchpad item in milliseconds.",
      description:
        "Stop hunting through nested folders. The unified search engine queries across your entire database, returning classified matches with highlighted snippets and direct navigation links.",
      highlights: [
        "Unified search querying skills, topic names, descriptions, topic notes, and personal scratchpads",
        "Distinct result type badges: skill, topic, note, and personal_note",
        "Contextual snippet previews highlighting search query occurrences",
        "Direct 'Open in tree' deep links jumping straight to the matching skill in your syllabus tree",
      ],
      ctaLabel: "Search Knowledge Base",
      ctaLink: isAuthenticated ? "/search" : "/login?mode=register",
      renderVisual: () => (
        <div className="home-mockup-wrap">
          <div style={{ display: "flex", flexDirection: "column", gap: "0.6rem" }}>
            <div style={{ display: "flex", alignItems: "center", gap: "0.5rem", background: "#17202a", border: "1px solid #38bdf8", borderRadius: "8px", padding: "0.45rem 0.75rem", fontSize: "0.82rem", color: "#ffffff" }}>
              <span>🔍</span>
              <span>connection pooling</span>
            </div>
            <div style={{ background: "#141c25", border: "1px solid #283747", borderRadius: "6px", padding: "0.5rem 0.75rem" }}>
              <span style={{ fontSize: "0.68rem", background: "#1e293b", color: "#38bdf8", padding: "0.1rem 0.35rem", borderRadius: "4px" }}>topic</span>
              <div style={{ fontSize: "0.78rem", fontWeight: 600, color: "#ffffff", marginTop: "0.2rem" }}>Database Connection Pooling</div>
              <div style={{ fontSize: "0.7rem", color: "#93a4b5" }}>In: PostgreSQL Architecture → [Open in tree]</div>
            </div>
          </div>
        </div>
      ),
    },
    {
      id: "trash-recovery",
      pill: "Trash Recovery",
      category: "Data Safety",
      title: "Two-Step Trash Safety & 30-Day Recovery Buffer",
      tagline: "Accidentally deleted a skill? No panic. Recover it anytime within 30 days.",
      description:
        "Never lose your hard work to accidental misclicks. Deleted skills are placed in a 30-day quarantine buffer with live expiration countdowns and full one-click restoration.",
      highlights: [
        "Two-stage soft deletion safeguarding against unintentional skill removal",
        "Precise countdown readout: 'Permanently deleted in X days Y hours'",
        "One-click instant restoration bringing back all nested topics and notes intact",
        "Permanent hard delete option protected by modal confirmation dialogs",
      ],
      ctaLabel: "Inspect Trash Buffer",
      ctaLink: isAuthenticated ? "/trash" : "/login?mode=register",
      renderVisual: () => (
        <div className="home-mockup-wrap">
          <div style={{ background: "#182029", border: "1px solid #3f2026", borderRadius: "10px", padding: "0.85rem" }}>
            <div style={{ display: "flex", justifyContent: "space-between", alignItems: "center", marginBottom: "0.4rem" }}>
              <span style={{ fontWeight: 600, color: "#ffffff", fontSize: "0.85rem" }}>Kubernetes Mastery</span>
              <span style={{ background: "#3f1d24", color: "#fb7185", fontSize: "0.7rem", padding: "0.15rem 0.45rem", borderRadius: "4px" }}>Deleted</span>
            </div>
            <div style={{ fontSize: "0.75rem", color: "#fbbf24", marginBottom: "0.7rem" }}>
              ⏳ Permanently deleted in 26 days 18 hours
            </div>
            <div style={{ display: "flex", gap: "0.5rem" }}>
              <span style={{ background: "#134e4a", color: "#5eead4", fontSize: "0.75rem", padding: "0.3rem 0.65rem", borderRadius: "6px", cursor: "pointer" }}>
                ↩ Restore Skill
              </span>
              <span style={{ background: "#243040", color: "#fb7185", fontSize: "0.75rem", padding: "0.3rem 0.65rem", borderRadius: "6px", cursor: "pointer" }}>
                Permanent Delete
              </span>
            </div>
          </div>
        </div>
      ),
    },
    {
      id: "profile-settings",
      pill: "User Profiles",
      category: "Personalization",
      title: "Personalized Profile Settings & Visual Skill Bars",
      tagline: "Customize your name, select your timezone, and inspect per-skill progress bars.",
      description:
        "Tailor the platform to your personal workflow. Manage display names, configure your international timezone for streak calculations, and track individual skill completion bars.",
      highlights: [
        "Display name editing with inline pencil icon and instant update validation",
        "Global IANA Timezone selector (50+ timezones auto-detected on signup)",
        "Color-coded individual skill progress bars with unique distinct pastel hues",
        "Celebratory 'Congratulations!' banners commemorating 100% completed skills",
      ],
      ctaLabel: "Open Settings",
      ctaLink: isAuthenticated ? "/settings" : "/login?mode=register",
      renderVisual: () => (
        <div className="home-mockup-wrap">
          <div style={{ display: "flex", flexDirection: "column", gap: "0.7rem" }}>
            <div style={{ background: "#141c25", padding: "0.6rem 0.8rem", borderRadius: "8px", border: "1px solid #243242" }}>
              <div style={{ display: "flex", justifyContent: "space-between", fontSize: "0.78rem", marginBottom: "0.3rem" }}>
                <span style={{ color: "#ffffff", fontWeight: 600 }}>Python Backend</span>
                <span style={{ color: "#4ade80", fontWeight: 700 }}>100% Completed! 🎉</span>
              </div>
              <div style={{ height: "6px", background: "#0e1720", borderRadius: "4px", overflow: "hidden" }}>
                <div style={{ height: "100%", width: "100%", background: "#4ade80" }} />
              </div>
            </div>
            <div style={{ background: "#141c25", padding: "0.6rem 0.8rem", borderRadius: "8px", border: "1px solid #243242" }}>
              <div style={{ display: "flex", justifyContent: "space-between", fontSize: "0.78rem", marginBottom: "0.3rem" }}>
                <span style={{ color: "#ffffff", fontWeight: 600 }}>Cloud DevOps</span>
                <span style={{ color: "#38bdf8" }}>28 / 40 (70%)</span>
              </div>
              <div style={{ height: "6px", background: "#0e1720", borderRadius: "4px", overflow: "hidden" }}>
                <div style={{ height: "100%", width: "70%", background: "#38bdf8" }} />
              </div>
            </div>
          </div>
        </div>
      ),
    },
    {
      id: "security-auth",
      pill: "Security & Auth",
      category: "Security",
      title: "Enterprise-Grade Security & Authentication",
      tagline: "Rigorous Gmail validation, Argon2 hashing, JWT rotation, and 6-digit OTP resets.",
      description:
        "Your data is safeguarded with modern web security practices. Built with Argon2/bcrypt hashing, HttpOnly JWT token rotation, 6-digit OTP verification, and hardened security headers.",
      highlights: [
        "Strict Gmail address validation ensuring authentic user accounts",
        "6-digit OTP password reset workflow with 10-minute expiry and 60-second resend cooldown",
        "JWT Access Tokens & HttpOnly Refresh Token rotation with instantaneous session revocation",
        "Comprehensive HTTP Security Headers (CSP, Frame DENY, nosniff, HSTS) and SlowAPI rate protection",
      ],
      ctaLabel: "Create Secure Account",
      ctaLink: "/login?mode=register",
      renderVisual: () => (
        <div className="home-mockup-wrap">
          <div style={{ background: "#141d27", border: "1px solid #233345", borderRadius: "10px", padding: "0.9rem" }}>
            <div style={{ display: "flex", alignItems: "center", gap: "0.5rem", marginBottom: "0.6rem" }}>
              <span style={{ color: "#5eead4", fontSize: "1.1rem" }}>🔒</span>
              <span style={{ fontWeight: 600, color: "#ffffff", fontSize: "0.85rem" }}>Security Architecture</span>
            </div>
            <div style={{ display: "flex", flexDirection: "column", gap: "0.4rem", fontSize: "0.75rem", color: "#93a4b5" }}>
              <div style={{ display: "flex", justifyContent: "space-between" }}>
                <span>Password Hashing:</span>
                <span style={{ color: "#5eead4", fontWeight: 600 }}>Argon2id</span>
              </div>
              <div style={{ display: "flex", justifyContent: "space-between" }}>
                <span>OTP Verification:</span>
                <span style={{ color: "#38bdf8" }}>6-digit (60s cooldown)</span>
              </div>
              <div style={{ display: "flex", justifyContent: "space-between" }}>
                <span>Token Rotation:</span>
                <span style={{ color: "#4ade80" }}>HttpOnly Refresh</span>
              </div>
              <div style={{ display: "flex", justifyContent: "space-between" }}>
                <span>Rate Limiter:</span>
                <span style={{ color: "#fbbf24" }}>SlowAPI Active</span>
              </div>
            </div>
          </div>
        </div>
      ),
    },
    {
      id: "admin-console",
      pill: "Admin Console",
      category: "Administration",
      title: "Administrative Command Center & Security Locks",
      tagline: "Superuser directory inspection, password overrides, and auto-locking protection.",
      description:
        "Protected admin tools for multi-user platforms. Administrators can audit registered users, inspect account states, unlock sessions with secret passkeys, and manage credentials safely.",
      highlights: [
        "Passkey-gated admin console with automatic session locking on tab switch or navigation",
        "User directory inspection: view registered emails, activation dates, and pending deletions",
        "Direct administrator password override triggering mandatory password change on next login",
        "Automated background cron triggers for orphan data cleanup and expired trash purging",
      ],
      ctaLabel: "Admin Access",
      ctaLink: isAuthenticated ? "/admin" : "/login",
      renderVisual: () => (
        <div className="home-mockup-wrap">
          <div style={{ background: "#131922", border: "1px solid #2a3b4e", borderRadius: "10px", padding: "0.85rem" }}>
            <div style={{ display: "flex", justifyContent: "space-between", alignItems: "center", marginBottom: "0.6rem" }}>
              <span style={{ fontSize: "0.82rem", fontWeight: 600, color: "#ffffff" }}>Admin Control Shield</span>
              <span style={{ background: "#134e4a", color: "#5eead4", fontSize: "0.7rem", padding: "0.15rem 0.4rem", borderRadius: "4px" }}>
                🔒 Auto-Locks on Blur
              </span>
            </div>
            <div style={{ display: "flex", flexDirection: "column", gap: "0.4rem", fontSize: "0.74rem" }}>
              <div style={{ display: "flex", justifyContent: "space-between", background: "#192330", padding: "0.35rem 0.55rem", borderRadius: "4px" }}>
                <span style={{ color: "#cbd5e1" }}>user@gmail.com</span>
                <span style={{ color: "#5eead4" }}>Active</span>
              </div>
              <div style={{ display: "flex", justifyContent: "space-between", background: "#192330", padding: "0.35rem 0.55rem", borderRadius: "4px" }}>
                <span style={{ color: "#cbd5e1" }}>dev@gmail.com</span>
                <span style={{ color: "#fbbf24" }}>Reset Password</span>
              </div>
            </div>
          </div>
        </div>
      ),
    },
    {
      id: "fluid-ui",
      pill: "Responsive UI",
      category: "Architecture",
      title: "Fluid Responsive UI & Dark Modern Architecture",
      tagline: "Engineered with React 19, TypeScript, and modern CSS for lightning-fast performance.",
      description:
        "Enjoy a fast, distraction-free study environment across all devices. Crafted with a sleek dark-mode aesthetic, glowing neon accents, collapsible mobile drawers, and smooth animations.",
      highlights: [
        "Single-Page Application powered by React 19, TypeScript, and React Router 7",
        "Optimistic cache mutations and instant UI synchronization powered by TanStack React Query",
        "Collapsible mobile navigation drawer with backdrop overlay for smartphones and tablets",
        "Server cold-start wake-up notification banner for seamless free-tier cloud hosting",
      ],
      ctaLabel: "Launch Application",
      ctaLink: isAuthenticated ? "/dashboard" : "/login",
      renderVisual: () => (
        <div className="home-mockup-wrap">
          <div style={{ display: "flex", gap: "0.6rem", alignItems: "center", justifyContent: "center" }}>
            <div style={{ background: "#16202c", border: "1px solid #38bdf8", borderRadius: "8px", padding: "0.7rem", textAlign: "center", width: "110px" }}>
              <div style={{ fontSize: "1.2rem", marginBottom: "0.2rem" }}>💻</div>
              <span style={{ fontSize: "0.72rem", color: "#38bdf8", fontWeight: 600 }}>Desktop</span>
            </div>
            <div style={{ background: "#16202c", border: "1px solid #5eead4", borderRadius: "8px", padding: "0.7rem", textAlign: "center", width: "90px" }}>
              <div style={{ fontSize: "1.2rem", marginBottom: "0.2rem" }}>📱</div>
              <span style={{ fontSize: "0.72rem", color: "#5eead4", fontWeight: 600 }}>Mobile</span>
            </div>
          </div>
          <div style={{ display: "flex", gap: "0.4rem", justifyContent: "center", marginTop: "0.8rem" }}>
            <span style={{ background: "#111822", color: "#93a4b5", fontSize: "0.68rem", padding: "0.2rem 0.5rem", borderRadius: "4px" }}>React 19</span>
            <span style={{ background: "#111822", color: "#93a4b5", fontSize: "0.68rem", padding: "0.2rem 0.5rem", borderRadius: "4px" }}>TypeScript</span>
            <span style={{ background: "#111822", color: "#93a4b5", fontSize: "0.68rem", padding: "0.2rem 0.5rem", borderRadius: "4px" }}>FastAPI</span>
          </div>
        </div>
      ),
    },
  ];

  const totalFeatures = features.length;

  const nextSlide = useCallback(() => {
    setActiveIdx((prev) => (prev + 1) % totalFeatures);
  }, [totalFeatures]);

  const prevSlide = useCallback(() => {
    setActiveIdx((prev) => (prev - 1 + totalFeatures) % totalFeatures);
  }, [totalFeatures]);

  // Keyboard navigation (Left / Right Arrow)
  useEffect(() => {
    function handleKeyDown(e: KeyboardEvent) {
      if (e.key === "ArrowLeft") {
        prevSlide();
      } else if (e.key === "ArrowRight") {
        nextSlide();
      }
    }
    window.addEventListener("keydown", handleKeyDown);
    return () => window.removeEventListener("keydown", handleKeyDown);
  }, [nextSlide, prevSlide]);

  // Auto-play interval
  useEffect(() => {
    if (!isAutoPlay || isHovered) return;
    const timer = setInterval(() => {
      nextSlide();
    }, 6500);
    return () => clearInterval(timer);
  }, [isAutoPlay, isHovered, nextSlide]);

  const current = features[activeIdx];

  return (
    <div className="home-root">
      {/* ====================================================================
          TOP HEADER: Above the horizontal line
          - Left: Website Name with Logo
          - Right: Sign In and Sign Up buttons (take to existing login/signup page)
          ==================================================================== */}
      <header className="home-header">
        <Link to="/" className="home-brand" aria-label="Skill Progress Tracker Home">
          <div className="home-logo-wrap" title="Skill Progress Tracker">
            <img src="/favicon.png?v=2" alt="Skill Progress Tracker Logo" className="home-logo-img" />
          </div>
          <div className="home-brand-text">
            <span className="home-brand-name">Skill Progress Tracker</span>
            <span className="home-brand-tag">Syllabus to Mastery Engine</span>
          </div>
        </Link>

        <div className="home-header-actions">
          {isAuthenticated && (
            <Link to="/dashboard" className="home-btn-dash">
              <svg width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2">
                <path d="M4 10.5 12 4l8 6.5V20a1 1 0 0 1-1 1h-5v-6H10v6H5a1 1 0 0 1-1-1v-9.5Z" />
              </svg>
              <span>Dashboard</span>
            </Link>
          )}
          <Link to="/login" className="home-btn-signin">
            Sign In
          </Link>
          <Link to="/login?mode=register" className="home-btn-signup">
            Sign Up
          </Link>
        </div>
      </header>

      {/* Horizontal Divider Line Across Screen */}
      <hr className="home-divider" />

      {/* ====================================================================
          MAIN CONTENT AREA:
          - Quick Jump Pills Bar
          - Centered Feature Box with Left < and Right > Red Arrows
          - Red Indicator Dots below the box
          - Auto-play toggle & slide counter
          ==================================================================== */}
      <main className="home-main">
        {/* Quick Jump Feature Selector Pills */}
        <nav className="home-pills-bar" aria-label="Feature navigation pills">
          {features.map((f, i) => (
            <button
              key={f.id}
              type="button"
              className={`home-pill-btn${i === activeIdx ? " active" : ""}`}
              onClick={() => setActiveIdx(i)}
              title={f.title}
            >
              {i + 1}. {f.pill}
            </button>
          ))}
        </nav>

        {/* Carousel Showcase Container */}
        <div
          className="home-carousel-wrapper"
          onMouseEnter={() => setIsHovered(true)}
          onMouseLeave={() => setIsHovered(false)}
        >
          {/* Left Red Arrow < (matching user drawing) */}
          <button
            type="button"
            className="home-nav-arrow prev"
            onClick={prevSlide}
            aria-label="Previous Feature"
            title="Previous Feature (Left Arrow)"
          >
            <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeLinecap="round" strokeLinejoin="round">
              <polyline points="15 18 9 12 15 6" />
            </svg>
          </button>

          {/* Centered Feature Box (Exact Size as drawn in user diagram) */}
          <article className="home-feature-box" aria-live="polite">
            <div className="home-feature-grid">
              {/* Left Column: Feature Info */}
              <div className="home-feature-info">
                <div>
                  <div className="home-feature-top-meta">
                    <span className="home-feature-badge">
                      <span>✦</span>
                      <span>{current.category}</span>
                    </span>
                    <span className="home-feature-count">
                      Feature {String(activeIdx + 1).padStart(2, "0")} / {String(totalFeatures).padStart(2, "0")}
                    </span>
                  </div>

                  <h2 className="home-feature-title">{current.title}</h2>
                  <p className="home-feature-tagline">{current.tagline}</p>
                  <p className="home-feature-desc">{current.description}</p>

                  <ul className="home-feature-highlights">
                    {current.highlights.map((h, i) => (
                      <li key={i}>
                        <svg className="check-icon" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2.5">
                          <polyline points="20 6 9 17 4 12" />
                        </svg>
                        <span>{h}</span>
                      </li>
                    ))}
                  </ul>
                </div>

                <div style={{ marginTop: "1rem" }}>
                  <Link to={current.ctaLink} className="home-feature-cta">
                    <span>{current.ctaLabel}</span>
                    <svg width="15" height="15" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2.5">
                      <line x1="5" y1="12" x2="19" y2="12" />
                      <polyline points="12 5 19 12 12 19" />
                    </svg>
                  </Link>
                </div>
              </div>

              {/* Right Column: Interactive/Visual Component Mockup */}
              <div className="home-feature-visual">
                {current.renderVisual()}
              </div>
            </div>
          </article>

          {/* Right Red Arrow > (matching user drawing) */}
          <button
            type="button"
            className="home-nav-arrow next"
            onClick={nextSlide}
            aria-label="Next Feature"
            title="Next Feature (Right Arrow)"
          >
            <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeLinecap="round" strokeLinejoin="round">
              <polyline points="9 18 15 12 9 6" />
            </svg>
          </button>
        </div>

        {/* Red Indicator Dots Below the Box (Matching user drawing: . . . . . . .) */}
        <div className="home-dots-row" role="tablist" aria-label="Feature slides">
          {features.map((f, i) => (
            <button
              key={f.id}
              type="button"
              role="tab"
              aria-selected={i === activeIdx}
              className={`home-dot${i === activeIdx ? " active" : ""}`}
              onClick={() => setActiveIdx(i)}
              aria-label={`Jump to feature ${i + 1}: ${f.title}`}
            />
          ))}
        </div>

        {/* Auto-Play Toggle & Slide Controls */}
        <div className="home-carousel-controls">
          <button
            type="button"
            className={`home-autoplay-toggle${isAutoPlay ? " active" : ""}`}
            onClick={() => setIsAutoPlay(!isAutoPlay)}
            title={isAutoPlay ? "Pause Auto-Slide" : "Start Auto-Slide"}
          >
            {isAutoPlay ? "⏸ Auto-advance: On" : "▶ Auto-advance: Off"}
          </button>
          <span>Use keyboard ← / → arrows to navigate</span>
        </div>
      </main>

      {/* Footer */}
      <footer className="home-footer">
        <div>© 2026 Skill Progress Tracker. All rights reserved.</div>
        <div className="home-footer-links">
          <Link to={isAuthenticated ? "/dashboard" : "/login"}>App</Link>
          <Link to="/login">Sign In</Link>
          <Link to="/login?mode=register">Sign Up</Link>
        </div>
      </footer>
    </div>
  );
}
