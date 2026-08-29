import { defineConfig } from "@playwright/test";

// Pure unit tests for the client-side cube decoder and query engine. These need
// no browser and no dev server, so this config deliberately omits the webServer
// the screenshot config starts — running them should be near-instant.
export default defineConfig({
  testDir: "./tests",
  testMatch: /.*\.unit\.spec\.js/,
  outputDir: "./test-results",
  reporter: "list",
});
