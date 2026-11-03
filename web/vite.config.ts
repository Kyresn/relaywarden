import { defineConfig } from "vite";
import react from "@vitejs/plugin-react";

export default defineConfig({
  base: "/v0/resource/plugins/cpa-guard/ui/",
  plugins: [react()],
  build: {
    outDir: "dist",
    emptyOutDir: true,
    sourcemap: false,
    assetsInlineLimit: 4096,
  },
  server: {
    proxy: {
      "/v0/management": "http://127.0.0.1:8317",
      "/v8/management": "http://127.0.0.1:8317",
    },
  },
});
