import { defineConfig, devices } from "@playwright/test";

export default defineConfig({
  testDir: "./tests",
  outputDir: "./test-results",
  snapshotDir: "./test-snapshots",
  reporter: "list",
  use: {
    baseURL: "http://localhost:5174",
    screenshot: "on",
    trace: "off",
  },
  projects: [
    {
      name: "chromium",
      use: { ...devices["Desktop Chrome"] },
    },
  ],
  webServer: {
    command: "npx vite --port 5174",
    url: "http://localhost:5174",
    reuseExistingServer: false,
    timeout: 30000,
  },
});
