import { defineConfig, devices } from "@playwright/test";

// Real-backend end-to-end suite. Unlike ./tests (mocked RPCs), every test here
// gets its own floatd — a fresh copy of e2e/data on a free port, with real
// hledger and git snapshots — so specs can write freely and assert on the
// journal files and RPC results as well as the screen. See e2e/fixtures.js.
export default defineConfig({
  testDir: "./e2e",
  outputDir: "./test-results/e2e",
  reporter: "list",
  forbidOnly: !!process.env.CI,
  globalSetup: "./e2e/global-setup.js",
  timeout: 60_000,
  expect: { timeout: 10_000 },
  use: {
    screenshot: "only-on-failure",
    trace: "off",
  },
  projects: [
    {
      name: "chromium",
      use: {
        ...devices["Desktop Chrome"],
        ...(process.env.PW_CHROMIUM_PATH && {
          launchOptions: { executablePath: process.env.PW_CHROMIUM_PATH },
        }),
      },
    },
  ],
});
