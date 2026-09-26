import type { Event } from "../types/core";
import { ApiError, type ApiFetch } from "./api";
import {
	getCursor,
	markRejected,
	markSynced,
	putServerEvents,
	setCursor,
	unsyncedEvents,
} from "./db";

// The outbox. Tapping writes to IndexedDB and returns; this is what eventually
// carries those taps to the server, and what brings back whatever the other
// device on the touchline tagged.
//
// The endpoint is idempotent: events are keyed by their own ID, so a retry
// after a flaky response overwrites identical data. There is no dedup logic
// here and no "did that send?" ambiguity — the worst case of a retry is a
// wasted write.

const BATCH_SIZE = 50;
const CYCLE_MS = 10_000;
const FIRST_RETRY_MS = 1_000;
const MAX_RETRY_MS = 30_000;

export type SyncState = "offline" | "pending" | "syncing" | "synced" | "error";

export type SyncStatus = {
	state: SyncState;
	pending: number;
	error: string | null;
};

export type Sync = {
	start(): void;
	stop(): void;
	push(): Promise<void>;
	pull(): Promise<void>;
	subscribe(listener: (status: SyncStatus) => void): () => void;
};

export type SyncOptions = {
	apiFetch: ApiFetch;
	teamId: string;
	matchId: string;
};

export function createSync({ apiFetch, teamId, matchId }: SyncOptions): Sync {
	const eventsPath = `/teams/${teamId}/matches/${matchId}/events`;
	const listeners = new Set<(status: SyncStatus) => void>();

	let status: SyncStatus = { state: "pending", pending: 0, error: null };
	// Why the server refused a batch, kept until the page is reloaded. A
	// transient failure clears the moment a push succeeds, but a refusal means
	// taps a coach made are never going to land, and that has to stay on screen
	// rather than disappear behind the next successful sync.
	let refusal: string | null = null;
	let cycling = false;
	let interval: ReturnType<typeof setInterval> | undefined;
	let retry: ReturnType<typeof setTimeout> | undefined;
	let retryDelay = FIRST_RETRY_MS;

	function emit(next: Partial<SyncStatus>): void {
		status = { ...status, ...next };
		for (const listener of listeners) {
			listener(status);
		}
	}

	async function pending(): Promise<number> {
		return (await unsyncedEvents(matchId, Number.MAX_SAFE_INTEGER)).length;
	}

	/** Reports where the outbox stands once an attempt is over. */
	async function settle(transient: string | null): Promise<void> {
		const outstanding = await pending();
		const error = transient ?? refusal;

		if (!navigator.onLine) {
			emit({ state: "offline", pending: outstanding, error });
			return;
		}
		if (error) {
			emit({ state: "error", pending: outstanding, error });
			return;
		}
		emit({
			state: outstanding > 0 ? "pending" : "synced",
			pending: outstanding,
			error: null,
		});
	}

	/**
	 * Schedules another attempt, waiting longer after each failure so a server
	 * that is down is not hammered by every device on the touchline at once.
	 */
	function scheduleRetry(): void {
		clearTimeout(retry);
		retry = setTimeout(() => {
			retry = undefined;
			void push();
		}, retryDelay);
		retryDelay = Math.min(retryDelay * 2, MAX_RETRY_MS);
	}

	function clearRetry(): void {
		clearTimeout(retry);
		retry = undefined;
		retryDelay = FIRST_RETRY_MS;
	}

	/**
	 * Sends the outbox, oldest first, one batch at a time.
	 *
	 * A batch the server refuses with a 400 is recorded as rejected rather than
	 * retried: the reason will not change on the next attempt, and one poison
	 * tap holding its place at the front of the queue would stop every later tap
	 * from ever syncing. Anything else — a timeout, a 500, no signal at all — is
	 * transient, so the batch stays owed and a retry is scheduled.
	 */
	async function push(): Promise<void> {
		if (!navigator.onLine) {
			await settle(status.error);
			return;
		}

		emit({ state: "syncing" });
		for (;;) {
			const batch = await unsyncedEvents(matchId, BATCH_SIZE);
			if (batch.length === 0) {
				clearRetry();
				await settle(null);
				return;
			}

			const failure = await send(batch);
			if (failure === null) {
				continue;
			}
			if (failure.permanent) {
				await markRejected(ids(batch), failure.message);
				refusal = failure.message;
				await settle(null);
				continue;
			}

			scheduleRetry();
			await settle(failure.message);
			return;
		}
	}

	/** Returns null on success, or why the batch did not land. */
	async function send(
		batch: Event[],
	): Promise<{ message: string; permanent: boolean } | null> {
		try {
			await apiFetch(eventsPath, {
				method: "POST",
				body: JSON.stringify({ events: batch }),
			});
			await markSynced(ids(batch));
			return null;
		} catch (error) {
			return {
				message: error instanceof Error ? error.message : String(error),
				// Only the server saying the events themselves are wrong is
				// permanent. Everything else deserves another go.
				permanent: error instanceof ApiError && error.status === 400,
			};
		}
	}

	/**
	 * Fetches whatever has reached the server since this device last looked,
	 * including its own taps coming back, which land on the rows they already
	 * have because everything is keyed by event ID.
	 */
	async function pull(): Promise<void> {
		if (!navigator.onLine) {
			await settle(status.error);
			return;
		}

		const since = await getCursor(matchId);
		try {
			const page = await apiFetch<{ events: Event[]; cursor: string }>(
				`${eventsPath}?since=${encodeURIComponent(since)}`,
			);
			if (page.events.length > 0) {
				await putServerEvents(matchId, page.events);
			}
			if (page.cursor) {
				await setCursor(matchId, page.cursor);
			}
		} catch (error) {
			await settle(error instanceof Error ? error.message : String(error));
		}
	}

	/**
	 * Push before pull, so a coach's own taps leave the device at the first
	 * opportunity rather than waiting behind a read.
	 */
	async function cycle(): Promise<void> {
		// An interval firing during a slow push must not double-send.
		if (cycling) {
			return;
		}
		cycling = true;
		try {
			await push();
			await pull();
		} finally {
			cycling = false;
		}
	}

	const onOnline = () => void cycle();
	const onOffline = () => void settle(status.error);
	const onVisible = () => {
		if (document.visibilityState === "visible") {
			void cycle();
		}
	};

	return {
		start(): void {
			if (interval !== undefined) {
				return;
			}
			interval = setInterval(() => void cycle(), CYCLE_MS);
			window.addEventListener("online", onOnline);
			window.addEventListener("offline", onOffline);
			document.addEventListener("visibilitychange", onVisible);
			void cycle();
		},

		stop(): void {
			clearInterval(interval);
			interval = undefined;
			clearRetry();
			window.removeEventListener("online", onOnline);
			window.removeEventListener("offline", onOffline);
			document.removeEventListener("visibilitychange", onVisible);
		},

		push,
		pull,

		subscribe(listener: (status: SyncStatus) => void): () => void {
			listeners.add(listener);
			return () => {
				listeners.delete(listener);
			};
		},
	};
}

function ids(events: Event[]): string[] {
	return events.map((event) => event.id);
}
