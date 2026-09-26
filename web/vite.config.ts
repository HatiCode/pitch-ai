import tailwindcss from "@tailwindcss/vite";
import react from "@vitejs/plugin-react";
import { VitePWA } from "vite-plugin-pwa";
import { configDefaults, defineConfig } from "vitest/config";

export default defineConfig({
	plugins: [
		react(),
		tailwindcss(),
		VitePWA({
			registerType: "autoUpdate",
			// The registration goes in its own script tag rather than into the
			// app bundle. The service worker is what makes the app load at all
			// without a network, so it must not depend on the bundle evaluating:
			// a throw anywhere in the import graph — a missing Firebase config,
			// say — would otherwise take offline capability with it, silently and
			// permanently.
			injectRegister: "script",
			manifest: {
				name: "pitch-ai",
				short_name: "pitch-ai",
				description: "Rugby performance tracking",
				display: "standalone",
				orientation: "landscape",
				background_color: "#f8fafc",
				theme_color: "#0f172a",
				start_url: "/",
				icons: [
					{ src: "/icon-192.png", sizes: "192x192", type: "image/png" },
					{ src: "/icon-512.png", sizes: "512x512", type: "image/png" },
					{
						src: "/icon-512.png",
						sizes: "512x512",
						type: "image/png",
						purpose: "maskable",
					},
				],
			},
			workbox: {
				// The plugin only sets these itself for injectRegister "auto", so
				// choosing "script" above opts out of them. Without clientsClaim
				// the worker does not control the page that registered it, and the
				// first visit — the one before a coach ever goes offline — would
				// have no service worker serving it.
				clientsClaim: true,
				skipWaiting: true,
				globPatterns: ["**/*.{js,css,html,svg,png,webmanifest}"],
				navigateFallback: "/index.html",
				// The API is never cached. Offline reads come from IndexedDB, which
				// the app controls; a stale HTTP cache would hand the tagging screen
				// a lineup it cannot explain and cannot invalidate.
				navigateFallbackDenylist: [/^\/api\//],
				runtimeCaching: [{ urlPattern: /^\/api\//, handler: "NetworkOnly" }],
			},
		}),
	],
	server: {
		// Talk to the Go server during development without CORS.
		proxy: { "/api": "http://localhost:8080" },
	},
	test: {
		environment: "jsdom",
		// The Playwright specs live under e2e/ and match Vitest's default glob;
		// they need a browser, not jsdom.
		exclude: [...configDefaults.exclude, "e2e/**"],
		globals: true,
		setupFiles: ["./src/setupTests.ts"],
	},
});
