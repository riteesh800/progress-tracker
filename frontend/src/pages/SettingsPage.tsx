import { FormEvent, useEffect, useState } from "react";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { deleteAccount, me, updateMe } from "../api/auth.api";
import { getDashboard } from "../api/progress.api";
import { listSkills, type Skill } from "../api/skills.api";

import { uniqueSkillColor } from "../lib/skillColors";
import TimezoneSelect from "../components/TimezoneSelect";
import ConfirmDialog from "../components/ConfirmDialog";

function PencilIcon() {
  return (
    <svg width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="1.8" aria-hidden>
      <path d="M12 20h9" strokeLinecap="round" />
      <path d="m16.5 3.5 4 4L8 20H4v-4L16.5 3.5Z" strokeLinejoin="round" />
    </svg>
  );
}

function colorForSkill(_skillId: string, index: number) {
  return uniqueSkillColor(index);
}

function formatPercent(completed: number, total: number) {
  if (total <= 0) return "0%";
  const pct = (completed / total) * 100;
  if (pct === 0) return "0%";
  if (pct === 100) return "100%";
  if (pct < 10) return `${pct.toFixed(2)}%`;
  return `${pct.toFixed(1)}%`;
}

function SkillProgressBars({ skills }: { skills: Skill[] }) {
  return (
    <div className="settings-skill-bars">
      {skills.map((skill, index) => {
        const color = colorForSkill(skill.id, index);
        const pct = skill.total_leaves === 0 ? 0 : (skill.completed_leaves / skill.total_leaves) * 100;
        return (
          <div className="settings-skill-bar-row" key={skill.id}>
            <div className="settings-skill-bar-name" title={skill.name}>
              {skill.name}
            </div>
            <div className="settings-skill-bar-line">
              <div className="settings-skill-bar-track" aria-hidden>
                <span style={{ width: `${Math.min(100, pct)}%`, background: color }} />
              </div>
              <div className="settings-skill-bar-meta muted">
                <span>
                  {skill.completed_leaves}/{skill.total_leaves}
                </span>
                <span>{formatPercent(skill.completed_leaves, skill.total_leaves)}</span>
              </div>
            </div>
          </div>
        );
      })}
    </div>
  );
}

export default function SettingsPage() {
  const qc = useQueryClient();
  const profile = useQuery({ queryKey: ["me"], queryFn: me });
  const dash = useQuery({ queryKey: ["dashboard"], queryFn: getDashboard });
  const skillsQ = useQuery({ queryKey: ["skills"], queryFn: listSkills });

  const [editingName, setEditingName] = useState(false);
  const [nameDraft, setNameDraft] = useState("");
  const [nameMsg, setNameMsg] = useState<{ type: "ok" | "err"; text: string } | null>(null);

  const [accountOpen, setAccountOpen] = useState(false);
  const [accountBusy, setAccountBusy] = useState(false);
  const [tz, setTz] = useState("");
  const [tzDirty, setTzDirty] = useState(false);
  const [tzMsg, setTzMsg] = useState<{ type: "ok" | "err"; text: string } | null>(null);

  useEffect(() => {
    if (profile.data && !editingName) setNameDraft(profile.data.name);
    if (profile.data && !tzDirty) setTz(profile.data.timezone);
  }, [profile.data, editingName, tzDirty]);

  const saveName = useMutation({
    mutationFn: (name: string) => updateMe({ name }),
    onSuccess: async () => {
      await qc.invalidateQueries({ queryKey: ["me"] });
      setEditingName(false);
      setNameMsg({ type: "ok", text: "Name updated." });
    },
    onError: () => setNameMsg({ type: "err", text: "Could not update name." }),
  });

  const saveTz = useMutation({
    mutationFn: (timezone: string) => updateMe({ timezone }),
    onSuccess: async () => {
      await qc.invalidateQueries({ queryKey: ["me"] });
      setTzDirty(false);
      setTzMsg({ type: "ok", text: "Timezone saved." });
    },
    onError: () => setTzMsg({ type: "err", text: "Could not save timezone." }),
  });

  if (profile.isLoading) return <p>Loading settings…</p>;
  if (profile.isError || !profile.data) return <p className="error">Could not load profile.</p>;

  const user = profile.data;
  const d = dash.data;
  const skills = skillsQ.data ?? [];

  async function onSaveName(e: FormEvent) {
    e.preventDefault();
    const next = nameDraft.trim();
    if (!next) {
      setNameMsg({ type: "err", text: "Name cannot be empty." });
      return;
    }
    setNameMsg(null);
    saveName.mutate(next);
  }

  async function onSaveTz(e: FormEvent) {
    e.preventDefault();
    const next = tz.trim();
    if (!next) {
      setTzMsg({ type: "err", text: "Timezone is required." });
      return;
    }
    setTzMsg(null);
    saveTz.mutate(next);
  }

  return (
    <div className="grid settings-page">
      <h1>Settings</h1>

      <section className="settings-section">
        <h2 className="settings-section-title">Profile information</h2>
        <div className="card settings-card">
          <div className="settings-row">
            <div className="settings-label-col">
              <div>Display name</div>
              <div className="muted settings-hint">Shown across your account</div>
            </div>
            <div className="settings-value-col">
              {!editingName ? (
                <div className="settings-readonly-row">
                  <span className="settings-readonly">{user.name || "—"}</span>
                  <button
                    type="button"
                    className="icon-btn settings-edit-btn"
                    aria-label="Edit name"
                    title="Edit name"
                    onClick={() => {
                      setNameDraft(user.name);
                      setNameMsg(null);
                      setEditingName(true);
                    }}
                  >
                    <PencilIcon />
                  </button>
                </div>
              ) : (
                <form className="settings-edit-form" onSubmit={onSaveName}>
                  <input
                    value={nameDraft}
                    onChange={(e) => setNameDraft(e.target.value)}
                    placeholder="Display name"
                    autoFocus
                    aria-label="Display name"
                  />
                  <div className="settings-edit-actions">
                    <button className="primary" type="submit" disabled={saveName.isPending}>
                      {saveName.isPending ? "Saving…" : "Save"}
                    </button>
                    <button
                      type="button"
                      disabled={saveName.isPending}
                      onClick={() => {
                        setEditingName(false);
                        setNameDraft(user.name);
                        setNameMsg(null);
                      }}
                    >
                      Cancel
                    </button>
                  </div>
                </form>
              )}
              {nameMsg && <div className={nameMsg.type === "err" ? "error" : "settings-ok"}>{nameMsg.text}</div>}
            </div>
          </div>

          <div className="settings-row">
            <div className="settings-label-col">
              <div>Primary email</div>
              <div className="muted settings-hint">Email address cannot be changed here.</div>
            </div>
            <div className="settings-value-col">
              <div className="settings-email-box" title={user.email}>
                {user.email}
              </div>
            </div>
          </div>
        </div>
      </section>

      <section className="settings-section">
        <h2 className="settings-section-title">Progress overview</h2>
        <div className="card settings-card">
          {(dash.isLoading || skillsQ.isLoading) && <p className="muted" style={{ padding: "1rem 1.1rem" }}>Loading progress…</p>}
          {(dash.isError || skillsQ.isError) && <p className="error" style={{ padding: "1rem 1.1rem" }}>Could not load progress overview.</p>}
          {d && !dash.isLoading && !skillsQ.isLoading && !dash.isError && !skillsQ.isError && (
            <>
              <div className="settings-overview-head">
                <div>
                  <div className="settings-overview-title">Overall skills</div>
                  <div className="muted">One progress graph per skill</div>
                </div>
                <div className="settings-pct-badge">{d.overall_progress}%</div>
              </div>
              {d.empty || skills.length === 0 ? (
                <p className="muted" style={{ padding: "0 1.1rem 1rem" }}>
                  No skills yet. Create a skill to see progress graphs.
                </p>
              ) : (
                <SkillProgressBars skills={skills} />
              )}
              <div className="settings-stats">
                <div className="settings-stat">
                  <div className="muted">Total skills</div>
                  <div>{d.total_skills}</div>
                </div>
                <div className="settings-stat">
                  <div className="muted">Total topics</div>
                  <div>{d.total_topics}</div>
                </div>
                <div className="settings-stat">
                  <div className="muted">Completed leaves</div>
                  <div>{d.completed_topics}</div>
                </div>
                <div className="settings-stat">
                  <div className="muted">Overall completion</div>
                  <div>{d.overall_progress}%</div>
                </div>
                <div className="settings-stat">
                  <div className="muted">Current streak</div>
                  <div>{d.current_streak}</div>
                </div>
                <div className="settings-stat">
                  <div className="muted">Longest streak</div>
                  <div>{d.longest_streak}</div>
                </div>
              </div>
            </>
          )}
        </div>
      </section>

      <section className="settings-section">
        <h2 className="settings-section-title">Preferences</h2>
        <div className="card settings-card">
          <form className="settings-row settings-pref-row" onSubmit={onSaveTz}>
            <div className="settings-label-col">
              <div>Timezone</div>
              <div className="muted settings-hint">IANA timezone used for streak calendar days</div>
            </div>
            <div className="settings-value-col">
              <TimezoneSelect
                value={tz}
                onChange={(next) => {
                  setTz(next);
                  setTzDirty(true);
                  setTzMsg(null);
                }}
              />
              <div className="settings-edit-actions">
                <button className="primary" type="submit" disabled={saveTz.isPending || !tzDirty}>
                  {saveTz.isPending ? "Saving…" : "Save"}
                </button>
              </div>
              {tzMsg && <div className={tzMsg.type === "err" ? "error" : "settings-ok"}>{tzMsg.text}</div>}
            </div>
          </form>
        </div>
      </section>

      <section className="settings-section">
        <h2 className="settings-section-title settings-danger-title">Danger zone</h2>
        <div className="card settings-card settings-danger-card">
          <div className="settings-row settings-danger-row">
            <div className="settings-label-col">
              <div>Delete account</div>
              <div className="muted settings-hint">
                Immediately and permanently removes your skills, topics, notes, activity, and import history.
              </div>
            </div>
            <div className="settings-value-col">
              <button
                type="button"
                className="danger"
                onClick={() => setAccountOpen(true)}
              >
                Delete my account
              </button>
            </div>
          </div>
        </div>
      </section>
      <ConfirmDialog
        open={accountOpen}
        title="Are you sure you want to delete your account?"
        body="You will permanently lose your account and all associated data. This action cannot be undone."
        busy={accountBusy}
        onConfirm={async () => {
          setAccountBusy(true);
          try {
            await deleteAccount();
            window.location.href = "/login";
          } catch {
            setAccountBusy(false);
          }
        }}
        onCancel={() => !accountBusy && setAccountOpen(false)}
      />
    </div>
  );
}
