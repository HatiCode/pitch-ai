import { defineConfig, devices } from "@playwright/test";

const PORT = 4173;

export default defineConfig({
	testDir: "./e2e",
	// The one thing this suite proves needs a real browser, a real IndexedDB
	// and a real service worker, so it runs against the built bundle rather
	// than the dev server.
	webServer: {
		command: "npm run build:e2e && npx vite preview --port 4173 --strictPort",
		url: `http://localhost:${PORT}`,
		reuseExistingServer: !process.env.CI,
		timeout: 120_000,
	},
	use: {
		baseURL: `http://localhost:${PORT}`,
		trace: "on-first-retry",
	},
	projects: [{ name: "chromium", use: { ...devices["Desktop Chrome"] } }],
	forbidOnly: Boolean(process.env.CI),
	retries: process.env.CI ? 1 : 0,
	reporter: process.env.CI ? "github" : "list",
});
