import { defineConfig, type ProxyOptions } from "vite";
import react from "@vitejs/plugin-react";

/** Proxy API calls to FastAPI, but never steal SPA page navigations (Accept: text/html). */
function apiProxy(): ProxyOptions {
  return {
    target: "http://localhost:8000",
    bypass(req) {
      const accept = req.headers.accept ?? "";
      if (accept.includes("text/html")) return "/index.html";
    },
  };
}

export default defineConfig({
  plugins: [react()],
  server: {
    port: 5173,
    proxy: {
      "/auth": apiProxy(),
      "/skills": apiProxy(),
      "/topics": apiProxy(),
      "/notes": apiProxy(),
      "/personal-notes": apiProxy(),
      "/search": apiProxy(),
      "/streak": apiProxy(),
      "/activity": apiProxy(),
      "/dashboard": apiProxy(),
      "/pdf": apiProxy(),
      "/health": apiProxy(),
    },
  },
});
