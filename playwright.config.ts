import { defineConfig } from "@playwright/test";

export default defineConfig({
  testDir: "./e2e",
  fullyParallel: true,
  forbidOnly: !!process.env.CI,
  retries: 0,
  workers: 2,
  timeout: 90_000,
  expect: { timeout: 10_000 },
  reporter: [["list"], ["html", { open: "never" }]],
  use: {
    baseURL: "http://127.0.0.1:8790",
    browserName: "chromium",
    colorScheme: "dark",
    locale: "ja-JP",
    actionTimeout: 10_000,
    trace: "retain-on-failure",
    screenshot: "only-on-failure",
  },
  projects: [
    { name: "phone", use: { viewport: { width: 390, height: 844 }, isMobile: true, hasTouch: true } },
    { name: "tablet-portrait", use: { viewport: { width: 820, height: 1180 }, isMobile: true, hasTouch: true } },
    { name: "tablet-landscape", use: { viewport: { width: 1180, height: 820 }, isMobile: true, hasTouch: true } },
  ],
  webServer: {
    command: "npm run dev -- --local --ip 127.0.0.1 --port 8790 --persist-to .wrangler/e2e/state",
    url: "http://127.0.0.1:8790/api/health",
    reuseExistingServer: false,
    stdout: "pipe",
    stderr: "pipe",
    env: { WRANGLER_SEND_METRICS: "false" },
    gracefulShutdown: { signal: "SIGTERM", timeout: 5000 },
  },
});
