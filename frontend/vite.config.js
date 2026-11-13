import { defineConfig } from "vite"
import react from "@vitejs/plugin-react"
import path from "path"

export default defineConfig({
  plugins: [react()],
  build: {
    // MapLibre is intentionally kept as a lazy map chunk. The stricter,
    // executable limits live in scripts/check-bundle-budget.mjs.
    chunkSizeWarningLimit: 1050,
  },
  resolve: {
    alias: {
      "@": path.resolve(import.meta.dirname, "./src"),
    },
  },
})
