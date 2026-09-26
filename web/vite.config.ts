import tailwindcss from "@tailwindcss/vite";
import react from "@vitejs/plugin-react";
import { VitePWA } from "vite-plugin-pwa";
import { defineConfig } from "vitest/config";

export default defineConfig({
	plugins: [
		react(),
		tailwindcss(),
		VitePWA({
			registerType: "autoUpdate",
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
		globals: true,
		setupFiles: ["./src/setupTests.ts"],
	},
});
