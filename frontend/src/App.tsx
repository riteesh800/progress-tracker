import { NavLink, Navigate, Route, Routes, useLocation } from "react-router-dom";
import { useEffect, useState, type ReactNode } from "react";
import { useQuery } from "@tanstack/react-query";
import { me, logout } from "./api/auth.api";
import { getAccess } from "./api/client";
import { onWaking } from "./api/client";
import LoginPage from "./pages/LoginPage";
import DashboardPage from "./pages/DashboardPage";
import SkillsPage from "./pages/SkillsPage";
import SkillDetailPage from "./pages/SkillDetailPage";
import ImportPage from "./pages/ImportPage";
import SearchPage from "./pages/SearchPage";
import SettingsPage from "./pages/SettingsPage";
import StreakPage from "./pages/StreakPage";
import ActivityPage from "./pages/ActivityPage";
import MakeNotePage from "./pages/MakeNotePage";

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
  { to: "/settings", label: "Settings", end: true, d: "M12 15.5A3.5 3.5 0 1 0 12 8.5a3.5 3.5 0 0 0 0 7Zm8.2-3.1.9-1.6-1.8-3.1-1.9.3a7.6 7.6 0 0 0-1.5-.9l-.3-1.9H10.4l-.3 1.9a7.6 7.6 0 0 0-1.5.9l-1.9-.3-1.8 3.1.9 1.6a7.7 7.7 0 0 0 0 1.8l-.9 1.6 1.8 3.1 1.9-.3c.5.4 1 .7 1.5.9l.3 1.9h3.2l.3-1.9c.5-.2 1-.5 1.5-.9l1.9.3 1.8-3.1-.9-1.6c.1-.6.1-1.2 0-1.8Z" },
];

function Shell({ children }: { children: ReactNode }) {
  const [waking, setWaking] = useState(false);
  const [open, setOpen] = useState(false);
  const location = useLocation();
  useEffect(() => onWaking(setWaking), []);
  useEffect(() => setOpen(false), [location.pathname]);
  const q = useQuery({ queryKey: ["me"], queryFn: me, enabled: Boolean(getAccess()), retry: false });
  if (!getAccess()) return <Navigate to="/login" replace />;

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
        </nav>
        <div className="sidebar-foot">
          <span className="muted sidebar-email">{q.data?.email}</span>
          <button
            type="button"
            onClick={async () => {
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
    <Routes>
      <Route path="/login" element={<LoginPage />} />
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
      <Route path="/settings" element={<Shell><SettingsPage /></Shell>} />
    </Routes>
  );
}
