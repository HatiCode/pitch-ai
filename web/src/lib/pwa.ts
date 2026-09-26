import { registerSW } from "virtual:pwa-register";

// registerServiceWorker installs the worker that makes the app load without a
// network, which is the precondition for tagging a match on a pitch with no
// signal. autoUpdate means a new deploy takes effect on the next load rather
// than asking a coach to confirm an update mid-match.
//
// A no-op where there is no service worker: jsdom has none, and every component
// test would otherwise fail on import.
export function registerServiceWorker(): void {
	if (!("serviceWorker" in navigator)) {
		return;
	}
	registerSW({ immediate: true });
}
