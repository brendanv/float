import { defineConfig } from "vite";
import preact from "@preact/preset-vite";

export default defineConfig({
  plugins: [preact()],
  build: {
    outDir: "../internal/webui/dist",
    emptyOutDir: true,
  },
  server: {
    proxy: {
      "/float.v1.LedgerService": "http://localhost:8080",
    },
  },
});
