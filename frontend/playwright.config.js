import { defineConfig, devices } from "@playwright/test"
import os from "node:os"
import path from "node:path"

const baseURL = "http://127.0.0.1:4173"

export default defineConfig({
  testDir: "./e2e",
  fullyParallel: false,
  forbidOnly: true,
  retries: 0,
  reporter: "line",
  outputDir: path.join(os.tmpdir(), "veracity-maps-playwright"),
  use: {
    baseURL,
    screenshot: "only-on-failure",
    trace: "retain-on-failure",
  },
  webServer: {
    command: 'cmd.exe /c "npm.cmd run dev -- --host 127.0.0.1 --port 4173"',
    url: baseURL,
    reuseExistingServer: false,
    timeout: 120_000,
    stdout: "ignore",
    stderr: "pipe",
  },
  projects: [
    {
      name: "desktop-chromium",
      use: { ...devices["Desktop Chrome"] },
    },
    {
      name: "mobile-chromium",
      use: { ...devices["Pixel 5"] },
    },
  ],
})
