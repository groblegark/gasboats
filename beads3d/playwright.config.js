import { defineConfig } from '@playwright/test';

export default defineConfig({
  testDir: './tests',
  timeout: 60000,
  expect: {
    toHaveScreenshot: {
      maxDiffPixelRatio: 0.35, // force-directed layout is non-deterministic; we test rendering, not positions
    },
  },

  // HTML report with embedded screenshots, diffs, and videos (bd-ibxu4)
  reporter: [
    ['html', { open: 'never', outputFolder: 'test-results/html-report' }],
    ['list'],  // console output for CI
  ],

  use: {
    baseURL: 'http://localhost:3333',
    viewport: { width: 1280, height: 800 },
    // WebGL needs real GPU context — use headed Chromium
    launchOptions: {
      args: ['--use-gl=angle', '--use-angle=swiftshader'],
    },
    // Visual feedback: record video of every test run (bd-ibxu4)
    video: 'on',
    // Trace on first retry — captures DOM snapshots, network, console for debugging
    trace: 'on-first-retry',
    // Screenshot on failure for quick diagnosis
    screenshot: 'only-on-failure',
  },

  // Retry failed tests once (trace is captured on retry for debugging)
  retries: 1,

  // Output directory for videos, traces, and screenshots
  outputDir: 'test-results/artifacts',

  webServer: {
    command: 'npm run dev -- --host',
    port: 3333,
    reuseExistingServer: true,
    timeout: 15000,
  },
  projects: [
    {
      name: 'chromium',
      use: { browserName: 'chromium' },
    },
    {
      name: 'camera-mouse',
      testMatch: /camera|interactions|mouse-controls/,
      use: {
        browserName: 'chromium',
        screenshot: 'on',  // capture before/after screenshots for all camera & mouse tests
      },
    },
  ],
});
