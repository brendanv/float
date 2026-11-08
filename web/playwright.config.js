import { defineConfig, devices } from "@playwright/test";

export default defineConfig({
  testDir: "./tests",
  outputDir: "./test-results",
  snapshotDir: "./test-snapshots",
  reporter: "list",
  forbidOnly: !!process.env.CI,
  use: {
    baseURL: "http://localhost:5174",
    screenshot: "on",
    trace: "off",
  },
  projects: [
    {
      name: "chromium",
      use: {
        ...devices["Desktop Chrome"],
        // Optional override for environments whose preinstalled Chromium
        // doesn't match the pinned Playwright version.
        ...(process.env.PW_CHROMIUM_PATH && {
          launchOptions: { executablePath: process.env.PW_CHROMIUM_PATH },
        }),
      },
    },
  ],
  webServer: {
    command: "bunx vite --port 5174",
    url: "http://localhost:5174",
    reuseExistingServer: false,
    timeout: 30000,
  },
});
