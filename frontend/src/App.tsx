import { NavLink, Navigate, Route, Routes, useLocation } from "react-router-dom";
import { lazy, Suspense, useEffect, useState, type ReactNode } from "react";
import { useQuery } from "@tanstack/react-query";
import { me, logout } from "./api/auth.api";
import { getAccess } from "./api/client";
import { onWaking } from "./api/client";
import { clearAdminToken } from "./api/admin.api";

const LoginPage = lazy(() => import("./pages/LoginPage"));
const ForceChangePasswordPage = lazy(() => import("./pages/ForceChangePasswordPage"));
const DashboardPage = lazy(() => import("./pages/DashboardPage"));
const SkillsPage = lazy(() => import("./pages/SkillsPage"));
const SkillDetailPage = lazy(() => import("./pages/SkillDetailPage"));
const ImportPage = lazy(() => import("./pages/ImportPage"));
const SearchPage = lazy(() => import("./pages/SearchPage"));
const SettingsPage = lazy(() => import("./pages/SettingsPage"));
const AdminPage = lazy(() => import("./pages/AdminPage"));
const StreakPage = lazy(() => import("./pages/StreakPage"));
const ActivityPage = lazy(() => import("./pages/ActivityPage"));
const MakeNotePage = lazy(() => import("./pages/MakeNotePage"));
const TrashPage = lazy(() => import("./pages/TrashPage"));

function Icon({ d }: { d: string }) {
  return (
    <svg className="nav-icon" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="1.7" aria-hidden>
      <path d={d} strokeLinecap="round" strokeLinejoin="round" />
    </svg>
  );
}

const links = [
  { to: "/", label: "Dashboard", end: true, d: "M4 10.5 12 4l8 6.5V20a1 1 0 0 1-1 1h-5v-6H10v6H5a1 1 0 0 1-1-1v-9.5Z" },
  { to: "/skills", label: "Skills", end: false, d: "M4 6h16M4 12h16M4 18h10" },
  { to: "/import", label: "Import PDF", end: false, d: "M12 4v10m0 0 4-4m-4 4-4-4M5 18h14" },
  { to: "/search", label: "Search", end: true, d: "m20 20-3.5-3.5M10.5 18a7.5 7.5 0 1 1 0-15 7.5 7.5 0 0 1 0 15Z" },
  { to: "/streak", label: "Streak", end: true, d: "M8 20V10m4 10V4m4 16v-6" },
  { to: "/activity", label: "Recent Activity", end: true, d: "M5 6h14M5 12h10M5 18h7" },
  { to: "/make-note", label: "Make Note", end: true, d: "M7 4h8l4 4v12H7V4Zm8 0v4h4M9 12h6M9 16h4" },
  { to: "/trash", label: "Trash", end: true, d: "M3 6h18M19 6v14a2 2 0 0 1-2 2H7a2 2 0 0 1-2-2V6m3 0V4a2 2 0 0 1 2-2h4a2 2 0 0 1 2 2v2M10 11v6M14 11v6" },
  { to: "/settings", label: "Settings", end: true, d: "M12 15.5A3.5 3.5 0 1 0 12 8.5a3.5 3.5 0 0 0 0 7Zm8.2-3.1.9-1.6-1.8-3.1-1.9.3a7.6 7.6 0 0 0-1.5-.9l-.3-1.9H10.4l-.3 1.9a7.6 7.6 0 0 0-1.5.9l-1.9-.3-1.8 3.1.9 1.6a7.7 7.7 0 0 0 0 1.8l-.9 1.6 1.8 3.1 1.9-.3c.5.4 1 .7 1.5.9l.3 1.9h3.2l.3-1.9c.5-.2 1-.5 1.5-.9l1.9.3 1.8-3.1-.9-1.6c.1-.6.1-1.2 0-1.8Z" },
];

function Shell({ children }: { children: ReactNode }) {
  const [waking, setWaking] = useState(false);
  const [open, setOpen] = useState(false);
  const location = useLocation();
  useEffect(() => onWaking(setWaking), []);
  useEffect(() => {
    setOpen(false);
    window.scrollTo(0, 0);
  }, [location.pathname]);
  const q = useQuery({ queryKey: ["me"], queryFn: me, enabled: Boolean(getAccess()), retry: false });
  if (!getAccess()) return <Navigate to="/login" replace />;
  if (q.isLoading) return <p>Loading…</p>;
  if (q.data?.must_change_password) return <Navigate to="/change-password" replace />;

  return (
    <div className={`layout${open ? " nav-open" : ""}`}>
      <button type="button" className="nav-toggle" aria-label="Open menu" onClick={() => setOpen(true)}>
        ☰
      </button>
      {open && <button type="button" className="nav-backdrop" aria-label="Close menu" onClick={() => setOpen(false)} />}
      <aside className="sidebar">
        <NavLink className="brand" to="/" onClick={() => setOpen(false)}>
          Progress Tracker
        </NavLink>
        <nav className="nav">
          {links.map((item) => (
            <NavLink key={item.to} to={item.to} end={item.end} className="nav-item" onClick={() => setOpen(false)}>
              <Icon d={item.d} />
              <span>{item.label}</span>
            </NavLink>
          ))}
          {q.data?.can_open_admin && (
            <NavLink to="/admin" className="nav-item" onClick={() => setOpen(false)}>
              <Icon d="M12 3 5 6.5v5c0 4.2 2.7 8 7 9 4.3-1 7-4.8 7-9v-5L12 3Zm0 6v4m0 3h.01" />
              <span>Admin</span>
            </NavLink>
          )}
        </nav>
        <div className="sidebar-foot">
          <span className="muted sidebar-email">{q.data?.email}</span>
          <button
            type="button"
            onClick={async () => {
              clearAdminToken();
              await logout();
              window.location.href = "/login";
            }}
          >
            Log out
          </button>
        </div>
      </aside>
      <div className="main">
        {waking && (
          <div className="waking">Waking up the server… this can take up to a minute on the free tier.</div>
        )}
        <main className="page">{children}</main>
      </div>
    </div>
  );
}

export default function App() {
  return (
    <Suspense fallback={<p>Loading…</p>}>
      <Routes>
        <Route path="/login" element={<LoginPage />} />
        <Route path="/change-password" element={<ForceChangePasswordPage />} />
        <Route path="/" element={<Shell><DashboardPage /></Shell>} />
        <Route path="/skills" element={<Shell><SkillsPage /></Shell>} />
        <Route path="/skills/:id" element={<Shell><SkillDetailPage /></Shell>} />
        <Route path="/import" element={<Shell><ImportPage /></Shell>} />
        <Route path="/import/:jobId" element={<Shell><ImportPage /></Shell>} />
        <Route path="/search" element={<Shell><SearchPage /></Shell>} />
        <Route path="/streak" element={<Shell><StreakPage /></Shell>} />
        <Route path="/activity" element={<Shell><ActivityPage /></Shell>} />
        <Route path="/make-note" element={<Shell><MakeNotePage /></Shell>} />
        <Route path="/notes" element={<Navigate to="/make-note" replace />} />
        <Route path="/trash" element={<Shell><TrashPage /></Shell>} />
        <Route path="/settings" element={<Shell><SettingsPage /></Shell>} />
        <Route path="/admin" element={<Shell><AdminPage /></Shell>} />
      </Routes>
    </Suspense>
  );
}
