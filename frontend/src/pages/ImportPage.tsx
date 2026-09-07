import { useEffect, useRef, useState, type DragEvent } from "react";
import { Link, useNavigate, useParams } from "react-router-dom";
import { confirmJob, getJob, patchTree, uploadPdf, type ImportJob, type ProposedNode } from "../api/pdf.api";
import { ApiError } from "../api/client";

function PreviewNode({
  node,
  onChange,
}: {
  node: ProposedNode;
  onChange: (n: ProposedNode) => void;
}) {
  const kids = node.children ?? [];
  return (
    <div className={`node ${node.confidence === "low" ? "low" : ""}`}>
      <div className="node-head">
        <input
          value={node.name}
          onChange={(e) => onChange({ ...node, name: e.target.value })}
        />
        {node.confidence === "low" && <span className="muted">low confidence</span>}
        <button
          className="danger"
          onClick={() => onChange({ ...node, name: "" })}
        >
          Clear name
        </button>
      </div>
      {kids.map((c, i) => (
        <PreviewNode
          key={i}
          node={c}
          onChange={(updated) => {
            const next = [...kids];
            next[i] = updated;
            onChange({ ...node, children: next });
          }}
        />
      ))}
      <button
        onClick={() => onChange({ ...node, children: [...kids, { name: "New topic", children: [] }] })}
      >
        Add child
      </button>
    </div>
  );
}

function PdfUploadArt() {
  return (
    <svg className="import-art" viewBox="0 0 160 120" aria-hidden>
      <rect x="38" y="18" width="52" height="68" rx="6" fill="#1e2936" stroke="#5eead4" strokeWidth="2" />
      <rect x="48" y="28" width="32" height="6" rx="2" fill="#334155" />
      <rect x="48" y="40" width="28" height="4" rx="2" fill="#334155" />
      <rect x="48" y="50" width="30" height="4" rx="2" fill="#334155" />
      <rect x="70" y="28" width="56" height="72" rx="6" fill="#243040" stroke="#38bdf8" strokeWidth="2" />
      <rect x="82" y="42" width="32" height="18" rx="4" fill="#ea580c" />
      <text x="98" y="55" textAnchor="middle" fill="#fff7ed" fontSize="9" fontFamily="Segoe UI, sans-serif">
        PDF
      </text>
      <rect x="82" y="68" width="28" height="4" rx="2" fill="#475569" />
      <rect x="82" y="78" width="24" height="4" rx="2" fill="#475569" />
      <ellipse cx="80" cy="98" rx="28" ry="12" fill="none" stroke="#fb7185" strokeWidth="2.2" />
      <circle cx="80" cy="94" r="10" fill="#fb7185" />
      <path d="M80 89v8M76 93l4-4 4 4" stroke="#fff" strokeWidth="1.8" strokeLinecap="round" strokeLinejoin="round" fill="none" />
      <path d="M118 22c4 2 6 6 2 8" stroke="#fbbf24" strokeWidth="1.6" fill="none" strokeLinecap="round" />
      <circle cx="34" cy="36" r="2" fill="#f97316" />
      <circle cx="128" cy="48" r="1.5" fill="#fbbf24" />
    </svg>
  );
}

function UploadCard({
  onFile,
  uploading,
  error,
}: {
  onFile: (file: File) => void;
  uploading: boolean;
  error: string;
}) {
  const inputRef = useRef<HTMLInputElement>(null);
  const [dragging, setDragging] = useState(false);
  const [pickedName, setPickedName] = useState("");
  const dragDepth = useRef(0);

  function acceptFile(file: File | undefined | null) {
    if (!file) return;
    if (file.type !== "application/pdf" && !file.name.toLowerCase().endsWith(".pdf")) {
      return;
    }
    setPickedName(file.name);
    onFile(file);
  }

  function onDrop(e: DragEvent) {
    e.preventDefault();
    dragDepth.current = 0;
    setDragging(false);
    acceptFile(e.dataTransfer.files?.[0]);
  }

  return (
    <div
      className={`import-upload-card${dragging ? " is-dragging" : ""}${uploading ? " is-uploading" : ""}`}
      onDragEnter={(e) => {
        e.preventDefault();
        dragDepth.current += 1;
        setDragging(true);
      }}
      onDragOver={(e) => e.preventDefault()}
      onDragLeave={(e) => {
        e.preventDefault();
        dragDepth.current = Math.max(0, dragDepth.current - 1);
        if (dragDepth.current === 0) setDragging(false);
      }}
      onDrop={onDrop}
    >
      <div className="import-upload-inner">
        <h2 className="import-upload-title">Upload your files</h2>
        <div className="import-art-wrap">
          <PdfUploadArt />
          <div className="import-file-chip">{pickedName || "No file chosen"}</div>
        </div>
        <p className="import-drop-label">Drop PDF files here</p>
        <div className="import-or">
          <span />
          <em>or</em>
          <span />
        </div>
        <input
          ref={inputRef}
          type="file"
          accept="application/pdf,.pdf"
          hidden
          onChange={(e) => {
            acceptFile(e.target.files?.[0]);
            e.target.value = "";
          }}
        />
        <button
          type="button"
          className="import-upload-btn"
          disabled={uploading}
          onClick={() => inputRef.current?.click()}
        >
          {uploading ? "Uploading…" : "Upload to edit"}
        </button>
        <p className="muted import-hint">Upload documents up to 10 MB in PDF</p>
        {error && <p className="error import-error">{error}</p>}
      </div>
    </div>
  );
}

function PdfGuide() {
  return (
    <section className="import-guide">
      <h2 className="import-guide-title">How should your PDF be structured?</h2>
      <div className="card import-guide-card">
        <div className="import-guide-block">
          <h3>1. Generate your syllabus using ChatGPT or any AI tool</h3>
          <p className="muted">
            You can use{" "}
            <a href="https://chatgpt.com/" target="_blank" rel="noreferrer">
              ChatGPT
            </a>{" "}
            or any other AI tool.
          </p>
          <p>Example prompt:</p>
          <blockquote className="import-prompt">
            Generate a syllabus for <strong>[subject]</strong> that is{" "}
            <strong>[comprehensive / only important concepts]</strong> for{" "}
            <strong>[interview / quiz / A-to-Z knowledge]</strong> using the following hierarchical
            structure:
            <pre>{`1
1.1
1.2
1.2.1
1.2.2
2
2.1
3`}</pre>
            Organize the content as topics, subtopics, and sub-subtopics using this numbering
            structure.
          </blockquote>
        </div>

        <div className="import-guide-block">
          <h3>Good PDF structure</h3>
          <p className="muted">
            Best results come from a clean, text-based PDF with a numbered topic hierarchy like{" "}
            <code>1 → 1.1 → 1.2 → 1.2.1</code>.
          </p>
          <pre className="import-good-sample">{`1 Introduction
1.1 Definition
1.2 Types
1.2.1 Type A
1.2.2 Type B

2 Core Concepts
2.1 Concept A
2.2 Concept B

3 Advanced Topics
3.1 Topic A`}</pre>
        </div>

        <div className="import-guide-block">
          <h3>Avoid</h3>
          <ul className="import-avoid-list">
            <li>Extra explanations before or after the syllabus</li>
            <li>Random/unstructured text</li>
            <li>Emojis</li>
            <li>Decorative lines</li>
            <li>Paragraphs mixed into the hierarchy</li>
            <li>Images instead of text</li>
            <li>Non-numbered structures when a hierarchy is intended</li>
          </ul>
        </div>
      </div>
    </section>
  );
}

export default function ImportPage() {
  const { jobId } = useParams();
  const nav = useNavigate();
  const [job, setJob] = useState<ImportJob | null>(null);
  const [error, setError] = useState("");
  const [statusMsg, setStatusMsg] = useState("");
  const [skillName, setSkillName] = useState("");
  const [uploading, setUploading] = useState(false);
  const bottomRef = useRef<HTMLDivElement>(null);

  useEffect(() => {
    if (!jobId) return;
    let stop = false;
    async function poll() {
      try {
        const j = await getJob(jobId!);
        if (stop) return;
        setJob(j);
        if (j.status === "extracting") setStatusMsg("Extracting text…");
        else if (j.status === "parsing" || j.status === "uploaded") setStatusMsg("Parsing PDF… Analyzing structure…");
        else setStatusMsg("");
        if (j.status === "uploaded" || j.status === "extracting" || j.status === "parsing") {
          window.setTimeout(poll, 1000);
        }
      } catch (e) {
        setError(e instanceof ApiError ? e.message : "Could not load job");
      }
    }
    poll();
    return () => {
      stop = true;
    };
  }, [jobId]);

  async function onFile(file: File) {
    setError("");
    setUploading(true);
    try {
      const j = await uploadPdf(file);
      nav(`/import/${j.id}`);
    } catch (e) {
      setError(e instanceof ApiError ? e.message : "Upload failed");
    } finally {
      setUploading(false);
    }
  }

  async function saveTree() {
    if (!job?.generated_tree_json || !jobId) return;
    await patchTree(jobId, job.generated_tree_json);
  }

  async function confirm() {
    if (!jobId) return;
    await saveTree();
    const done = await confirmJob(jobId, skillName || undefined);
    if (done.skill_id) nav(`/skills/${done.skill_id}`);
  }

  const tree = job?.generated_tree_json;
  const showUploadHome = !jobId;

  return (
    <div className="grid import-page">
      <h1>Import PDF</h1>
      <p className="muted">Every upload creates a brand-new skill after you confirm the preview.</p>

      {showUploadHome && (
        <>
          <UploadCard onFile={onFile} uploading={uploading} error={error} />
          <PdfGuide />
        </>
      )}

      {jobId && (
        <div className="grid">
          <div className="row">
            <Link to="/import">← Upload another PDF</Link>
          </div>
          {statusMsg && <div className="card">{statusMsg}</div>}
          {error && <p className="error">{error}</p>}
          {job?.status === "failed" && <p className="error">{job.error_message}</p>}
          {job?.status === "ready_for_preview" && tree && (
            <div className="card grid import-preview-card">
              <div className="import-preview-header">
                <div>
                  <h3>Preview editor</h3>
                  <p className="muted">Low-confidence nodes are outlined. Nothing is saved until you confirm.</p>
                </div>
                <button
                  className="scroll-end-btn"
                  onClick={() => bottomRef.current?.scrollIntoView({ behavior: "smooth" })}
                >
                  Scroll to end ↓
                </button>
              </div>
              <input placeholder="Skill name" value={skillName} onChange={(e) => setSkillName(e.target.value)} />
              <PreviewNode
                node={tree}
                onChange={(n) => setJob({ ...job, generated_tree_json: n })}
              />
              <div className="row" ref={bottomRef}>
                <button onClick={saveTree}>Save edits</button>
                <button className="primary" onClick={confirm}>Confirm import</button>
              </div>
            </div>
          )}
        </div>
      )}
    </div>
  );
}
