import path from "node:path";
import { defineConfig } from "vite";
import react from "@vitejs/plugin-react";
import tailwindcss from "@tailwindcss/vite";

export default defineConfig({
  plugins: [react(), tailwindcss()],
  resolve: {
    alias: {
      "@": path.resolve(__dirname, "./src"),
    },
  },
  build: {
    outDir: "../internal/webui/dist",
    emptyOutDir: true,
    rollupOptions: {
      output: {
        manualChunks(id) {
          if (id.includes("node_modules")) {
            if (id.includes("@tanstack/react-router") || id.includes("@tanstack/react-query")) {
              return "vendor-router";
            }
            if (id.includes("@connectrpc") || id.includes("@bufbuild")) {
              return "vendor-connect";
            }
            if (id.includes("@phosphor-icons") || id.includes("lucide-react")) {
              return "vendor-icons";
            }
          }
        },
      },
    },
  },
  server: {
    proxy: {
      "/float.v1.LedgerService": "http://localhost:8080",
    },
  },
});
