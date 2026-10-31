import { useState } from "react";
import { useNavigate } from "react-router-dom";
import { generateSyllabus } from "../api/ai.api";
import { ApiError } from "../api/client";

const DEPTH_OPTIONS = [
  {
    key: "easy",
    title: "Easy",
    badge: "Beginner",
    points: [
      "Basic fundamentals & definitions",
      "Introductory concepts",
      "Essential beginner-level topics",
      "Limited subtopics",
    ],
  },
  {
    key: "medium",
    title: "Medium",
    badge: "Core",
    points: [
      "Core fundamentals & important concepts",
      "Commonly used topics & mechanisms",
      "Relevant subtopics",
      "Moderate coverage for solid footing",
    ],
  },
  {
    key: "intermediate",
    title: "Intermediate",
    badge: "In-Depth",
    points: [
      "Detailed concept coverage",
      "Moderately advanced & practical concepts",
      "Expanded subtopics & sub-subtopics",
      "Appropriate depth for serious learning",
    ],
  },
  {
    key: "comprehensive",
    title: "Comprehensive",
    badge: "A to Z",
    points: [
      "Complete A-to-Z coverage from fundamentals to advanced",
      "All important subtopics & sub-subtopics",
      "Genuinely relevant adjacent concepts",
      "Deep technical & practical knowledge",
    ],
  },
];

const PURPOSE_OPTIONS = [
  {
    key: "interview",
    title: "Interview Preparation",
    desc: "Focus on frequently tested technical concepts, system design, coding patterns, edge cases, and practical interview questions.",
  },
  {
    key: "quiz",
    title: "Quiz Preparation",
    desc: "Focus on examinable concepts, definitions, classifications, properties, rules, and core conceptual understanding.",
  },
  {
    key: "a_to_z",
    title: "A-to-Z Knowledge",
    desc: "Focus on mastering the entire subject systematically from basic building blocks to advanced real-world implementations.",
  },
];

const LOADING_MESSAGES = [
  "Generating your syllabus…",
  "Analyzing subject structure and taxonomy…",
  "Organizing topics, subtopics, and sub-subtopics…",
  "Refining depth according to your chosen purpose…",
  "Structuring hierarchical tree…",
];

export default function GenerateAIPage() {
  const navigate = useNavigate();
  const [subject, setSubject] = useState("");
  const [depth, setDepth] = useState<string>("medium");
  const [purpose, setPurpose] = useState<string>("interview");
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState("");
  const [loadingMsgIndex, setLoadingMsgIndex] = useState(0);

  const cleanSubject = subject.trim();
  const isFormValid = cleanSubject.length > 0 && depth && purpose && !loading;

  async function handleGenerate(e: React.FormEvent) {
    e.preventDefault();
    if (!cleanSubject) {
      setError("Please enter a subject or topic name.");
      return;
    }
    if (!depth) {
      setError("Please select a syllabus depth.");
      return;
    }
    if (!purpose) {
      setError("Please select a generation purpose.");
      return;
    }

    setError("");
    setLoading(true);
    setLoadingMsgIndex(0);

    const timer = window.setInterval(() => {
      setLoadingMsgIndex((prev) => (prev + 1) % LOADING_MESSAGES.length);
    }, 3500);

    try {
      const response = await generateSyllabus(cleanSubject, depth, purpose);
      window.clearInterval(timer);

      // Store in sessionStorage as fallback so browser refresh preserves the draft
      const draftState = {
        skillName: response.skill_name || cleanSubject,
        topics: response.topics,
        subject: cleanSubject,
        depth,
        purpose,
        timestamp: Date.now(),
      };
      sessionStorage.setItem("pt_ai_draft", JSON.stringify(draftState));

      navigate("/generate/preview", {
        state: draftState,
      });
    } catch (err) {
      window.clearInterval(timer);
      setLoading(false);
      setError(
        err instanceof ApiError
          ? err.message
          : "Failed to generate syllabus. Please check your connection and try again."
      );
    }
  }

  return (
    <div className="grid generate-ai-page">
      <div className="generate-ai-header">
        <h1>Generate using AI</h1>
        <p className="muted">
          Create a comprehensive, custom-tailored syllabus powered by Google Gemini. Review and refine before saving.
        </p>
      </div>

      <form onSubmit={handleGenerate} className="card grid generate-form-card">
        {/* A. Subject / Topic Name */}
        <div className="generate-field">
          <label htmlFor="ai-subject-input" className="generate-label">
            Subject / Topic Name <span className="generate-required">*</span>
          </label>
          <input
            id="ai-subject-input"
            type="text"
            placeholder="e.g. Operating Systems, System Design, Data Structures & Algorithms, React"
            value={subject}
            onChange={(e) => setSubject(e.target.value)}
            maxLength={300}
            disabled={loading}
            autoFocus
            aria-required="true"
          />
          <span className="muted generate-hint">
            Be as specific as you like (e.g., "PostgreSQL Indexing & Optimization" or "Distributed Systems").
          </span>
        </div>

        {/* B. Syllabus Depth */}
        <div className="generate-field">
          <label className="generate-label">
            Syllabus Depth <span className="generate-required">*</span>
          </label>
          <p className="muted generate-field-sub">
            Controls how deeply topics are broken down into subtopics and sub-subtopics.
          </p>
          <div className="generate-depth-grid">
            {DEPTH_OPTIONS.map((opt) => (
              <button
                key={opt.key}
                type="button"
                className={`generate-depth-card ${depth === opt.key ? "selected" : ""}`}
                onClick={() => setDepth(opt.key)}
                disabled={loading}
                aria-pressed={depth === opt.key}
              >
                <div className="generate-card-top">
                  <span className="generate-depth-title">{opt.title}</span>
                  <span className="generate-depth-badge">{opt.badge}</span>
                </div>
                <ul className="generate-points-list">
                  {opt.points.map((pt, idx) => (
                    <li key={idx}>{pt}</li>
                  ))}
                </ul>
              </button>
            ))}
          </div>
        </div>

        {/* C. Generation Purpose */}
        <div className="generate-field">
          <label className="generate-label">
            Generation Purpose <span className="generate-required">*</span>
          </label>
          <p className="muted generate-field-sub">
            Directly influences topic focus, problem areas, and sequencing.
          </p>
          <div className="generate-purpose-grid">
            {PURPOSE_OPTIONS.map((opt) => (
              <button
                key={opt.key}
                type="button"
                className={`generate-purpose-card ${purpose === opt.key ? "selected" : ""}`}
                onClick={() => setPurpose(opt.key)}
                disabled={loading}
                aria-pressed={purpose === opt.key}
              >
                <div className="generate-purpose-title">{opt.title}</div>
                <div className="generate-purpose-desc muted">{opt.desc}</div>
              </button>
            ))}
          </div>
        </div>

        {/* Error message */}
        {error && <p className="error generate-error-banner" role="alert">{error}</p>}

        {/* D. Generate Button */}
        <div className="generate-actions">
          <button
            type="submit"
            className="primary generate-submit-btn"
            disabled={!isFormValid}
          >
            {loading ? (
              <span className="generate-loading-text">
                <span className="generate-spinner" /> {LOADING_MESSAGES[loadingMsgIndex]}
              </span>
            ) : (
              "Generate Syllabus"
            )}
          </button>
          {loading && (
            <p className="muted generate-loading-note">
              This usually takes 5-15 seconds depending on the chosen depth.
            </p>
          )}
        </div>
      </form>
    </div>
  );
}

