import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import type { Event } from "../types/core";
import { ApiError, type ApiFetch } from "./api";
import { appendEvent, db, getCursor, localEvents, unsyncedEvents } from "./db";
import { createSync, type Sync, type SyncStatus } from "./sync";

const TEAM = "team-1";
const MATCH = "match-1";
const EVENTS_PATH = `/teams/${TEAM}/matches/${MATCH}/events`;

type Call = { path: string; init?: RequestInit };

/**
 * A stand-in for the API that records what it was asked for and can be told to
 * fail. Responses are queued so a test can say "fail twice, then succeed".
 */
function stubApi() {
	const calls: Call[] = [];
	const failures: unknown[] = [];
	let page: { events: Event[]; cursor: string } = { events: [], cursor: "" };

	const apiFetch = vi.fn(async (path: string, init?: RequestInit) => {
		calls.push({ path, init });
		const failure = failures.shift();
		if (failure) {
			throw failure;
		}
		return init?.method === "POST" ? {} : page;
	});

	return {
		apiFetch: apiFetch as unknown as ApiFetch,
		calls,
		posts: () => calls.filter((call) => call.init?.method === "POST"),
		gets: () => calls.filter((call) => call.init?.method !== "POST"),
		failNext: (...errors: unknown[]) => failures.push(...errors),
		serve: (next: { events: Event[]; cursor: string }) => {
			page = next;
		},
	};
}

function sentEvents(call: Call): Event[] {
	return (JSON.parse(String(call.init?.body)) as { events: Event[] }).events;
}

function tackle(id: string, clockMs: number): Event {
	return {
		id,
		kind: "tackle_made",
		playerId: "player-7",
		clockMs,
		period: 1,
		deviceId: "device-1",
	};
}

async function appendTaps(count: number) {
	for (let i = 0; i < count; i++) {
		await appendEvent(
			MATCH,
			tackle(`e${String(i).padStart(4, "0")}`, i * 1000),
		);
	}
}

let sync: Sync | undefined;

function newSync(api: ReturnType<typeof stubApi>): Sync {
	sync = createSync({ apiFetch: api.apiFetch, teamId: TEAM, matchId: MATCH });
	return sync;
}

beforeEach(async () => {
	await Promise.all(db.tables.map((table) => table.clear()));
	vi.stubGlobal("navigator", { onLine: true });
});

afterEach(() => {
	sync?.stop();
	sync = undefined;
	vi.unstubAllGlobals();
	vi.useRealTimers();
});

describe("push", () => {
	it("drains the outbox in batches and marks each one synced", async () => {
		const api = stubApi();
		await appendTaps(120);

		await newSync(api).push();

		const posts = api.posts();
		expect(posts.map((call) => sentEvents(call).length)).toEqual([50, 50, 20]);
		expect(posts[0].path).toBe(EVENTS_PATH);
		expect(await unsyncedEvents(MATCH, 500)).toEqual([]);
	});

	// Nothing may be lost when the push fails: the events are the match.
	it("leaves events unsynced and reports the failure", async () => {
		const api = stubApi();
		api.failNext(new ApiError(503, "service unavailable"));
		await appendTaps(1);
		const sync = newSync(api);
		const seen = track(sync);

		await sync.push();

		expect(await unsyncedEvents(MATCH, 10)).toHaveLength(1);
		expect(last(seen).state).toBe("error");
		expect(last(seen).error).toContain("service unavailable");
		expect(last(seen).pending).toBe(1);
	});

	it("retries with backoff and stops once it succeeds", async () => {
		vi.useFakeTimers({ toFake: ["setTimeout", "clearTimeout", "Date"] });
		const api = stubApi();
		api.failNext(
			new ApiError(503, "service unavailable"),
			new ApiError(503, "service unavailable"),
		);
		await appendTaps(1);
		const sync = newSync(api);

		await sync.push();
		expect(api.posts()).toHaveLength(1);

		// Each failure waits longer than the last, so a server that is down does
		// not get hammered by every device on the touchline at once.
		await vi.advanceTimersByTimeAsync(1000);
		expect(api.posts()).toHaveLength(2);

		await vi.advanceTimersByTimeAsync(1000);
		expect(api.posts()).toHaveLength(2);
		await vi.advanceTimersByTimeAsync(1000);
		expect(api.posts()).toHaveLength(3);

		expect(await unsyncedEvents(MATCH, 10)).toEqual([]);

		// Success means no further retry is owed.
		await vi.advanceTimersByTimeAsync(60_000);
		expect(api.posts()).toHaveLength(3);
	});

	// A validation failure is permanent. Retrying it forever would mean every
	// tap after it never reaches the server.
	it("does not retry a batch the server refused", async () => {
		vi.useFakeTimers({ toFake: ["setTimeout", "clearTimeout", "Date"] });
		const api = stubApi();
		api.failNext(new ApiError(400, "playerId: not in the squad"));
		await appendTaps(2);
		const sync = newSync(api);
		const seen = track(sync);

		await sync.push();

		expect(await unsyncedEvents(MATCH, 10)).toEqual([]);
		// Recorded, not deleted: a tap that vanished would leave a coach with
		// numbers nobody can explain.
		expect(await localEvents(MATCH)).toHaveLength(2);
		expect((await db.events.get("e0000"))?.rejected).toContain(
			"not in the squad",
		);
		// Nothing is owed to the server any more, but the refusal stays on the
		// status: those taps are never going to land, and a coach has to know.
		expect(last(seen).error).toContain("not in the squad");
		expect(last(seen).state).toBe("error");
		expect(last(seen).pending).toBe(0);

		await vi.advanceTimersByTimeAsync(60_000);
		expect(api.posts()).toHaveLength(1);
	});

	it("still pushes the taps that follow a rejected batch", async () => {
		const api = stubApi();
		api.failNext(new ApiError(400, "playerId: not in the squad"));
		await appendTaps(1);
		const sync = newSync(api);
		await sync.push();

		await appendEvent(MATCH, tackle("e9999", 99000));
		await sync.push();

		expect(sentEvents(api.posts()[1]).map((e) => e.id)).toEqual(["e9999"]);
		expect(await unsyncedEvents(MATCH, 10)).toEqual([]);
	});
});

describe("pull", () => {
	it("asks from the stored cursor, stores what comes back, and advances it", async () => {
		const api = stubApi();
		api.serve({ events: [tackle("server-1", 5000)], cursor: "cursor-2" });
		const sync = newSync(api);

		await sync.pull();

		expect(api.gets()[0].path).toBe(`${EVENTS_PATH}?since=`);
		expect((await localEvents(MATCH)).map((e) => e.id)).toEqual(["server-1"]);
		expect(await getCursor(MATCH)).toBe("cursor-2");

		api.serve({ events: [], cursor: "cursor-3" });
		await sync.pull();

		expect(api.gets()[1].path).toBe(`${EVENTS_PATH}?since=cursor-2`);
	});

	// A device's own taps come back from the server on the next pull.
	it("does not duplicate an event it already has", async () => {
		const api = stubApi();
		await appendEvent(MATCH, tackle("e1", 1000));
		api.serve({ events: [tackle("e1", 1000)], cursor: "cursor-2" });

		await newSync(api).pull();

		expect(await localEvents(MATCH)).toHaveLength(1);
		expect(await unsyncedEvents(MATCH, 10)).toEqual([]);
	});
});

describe("connectivity", () => {
	it("pushes nothing while offline and catches up when the network returns", async () => {
		const api = stubApi();
		vi.stubGlobal("navigator", { onLine: false });
		await appendTaps(1);
		const sync = newSync(api);
		const seen = track(sync);

		await sync.push();

		expect(api.posts()).toHaveLength(0);
		expect(last(seen).state).toBe("offline");
		expect(last(seen).pending).toBe(1);

		vi.stubGlobal("navigator", { onLine: true });
		sync.start();
		window.dispatchEvent(new Event("online"));
		await vi.waitFor(() => expect(api.posts()).toHaveLength(1));

		expect(await unsyncedEvents(MATCH, 10)).toEqual([]);
	});
});

describe("the status subscription", () => {
	it("reports each change and stops on unsubscribe", async () => {
		const api = stubApi();
		await appendTaps(1);
		const sync = newSync(api);

		const seen: SyncStatus[] = [];
		const unsubscribe = sync.subscribe((status) => seen.push(status));

		await sync.push();
		expect(seen.map((status) => status.state)).toContain("syncing");
		expect(last(seen).state).toBe("synced");
		expect(last(seen).pending).toBe(0);

		const afterUnsubscribe = seen.length;
		unsubscribe();
		await appendTaps(1);
		await sync.push();

		expect(seen).toHaveLength(afterUnsubscribe);
	});
});

describe("the sync loop", () => {
	it("stops cycling once it is stopped", async () => {
		vi.useFakeTimers({
			toFake: [
				"setTimeout",
				"clearTimeout",
				"setInterval",
				"clearInterval",
				"Date",
			],
		});
		const api = stubApi();
		const sync = newSync(api);

		sync.start();
		await vi.advanceTimersByTimeAsync(25_000);
		const whileRunning = api.calls.length;
		expect(whileRunning).toBeGreaterThan(1);

		sync.stop();
		await vi.advanceTimersByTimeAsync(60_000);

		expect(api.calls).toHaveLength(whileRunning);
	});
});

function track(sync: Sync): SyncStatus[] {
	const seen: SyncStatus[] = [];
	sync.subscribe((status) => seen.push(status));
	return seen;
}

function last(seen: SyncStatus[]): SyncStatus {
	const status = seen.at(-1);
	if (!status) {
		throw new Error("no status was reported");
	}
	return status;
}
