import { act, renderHook, waitFor } from "@testing-library/react";
import { beforeEach, describe, expect, it } from "vitest";
import { appendEvent, db, localEvents } from "../../lib/db";
import type { CatalogueEntry, Event, Match } from "../../types/core";
import { useTagging } from "./useTagging";

const MATCH = "match-1";
const DEVICE = "device-1";

const CATALOGUE: CatalogueEntry[] = [
	{ kind: "tackle_made", label: "Tackle made", group: "play", player: true },
	{ kind: "carry", label: "Carry", group: "play", player: true },
	{ kind: "try", label: "Try", group: "play", player: true, points: 5 },
	{ kind: "sub", label: "Substitution", group: "play", player: false },
	{ kind: "scrum_won", label: "Scrum won", group: "set_piece", player: false },
	{
		kind: "yellow_card",
		label: "Yellow card",
		group: "discipline",
		player: true,
	},
];

function match(): Match {
	return {
		id: MATCH,
		teamId: "team-1",
		seasonId: "2026-27",
		opponent: "Castelnau RC",
		kickoffAt: "2026-03-14T15:00:00Z",
		competition: "Regional 1",
		venue: "home",
		status: "scheduled",
		lineup: {
			starters: Array.from({ length: 15 }, (_, i) => ({
				jersey: i + 1,
				playerId: `p${i + 1}`,
			})),
			bench: [
				{ jersey: 16, playerId: "p16" },
				{ jersey: 17, playerId: "p17" },
			],
		},
	};
}

// The match clock is driven by an injected time source rather than by fake
// timers: Dexie's live query only delivers when a real clock advances, so
// faking time would stop the events under test from ever arriving.
let clock = 0;

function advance(ms: number): void {
	clock += ms;
}

function setup() {
	clock = 1_760_000_000_000;
	return renderHook(() =>
		useTagging({
			matchId: MATCH,
			match: match(),
			catalogue: CATALOGUE,
			deviceId: DEVICE,
			now: () => clock,
		}),
	);
}

/** Waits for the Dexie write to come back through the live query. */
async function eventsSettle(count: number): Promise<Event[]> {
	let events: Event[] = [];
	await waitFor(async () => {
		events = await localEvents(MATCH);
		expect(events).toHaveLength(count);
	});
	return events;
}

beforeEach(async () => {
	await Promise.all(db.tables.map((table) => table.clear()));
});

describe("arming", () => {
	// Two taps in either order, because on a sideline a coach's eyes are on the
	// match rather than the screen.
	it("logs an event when an action is tapped then a player", async () => {
		const { result } = setup();

		act(() => result.current.tapAction("tackle_made"));
		act(() => result.current.tapPlayer("p7"));

		const [event] = await eventsSettle(1);
		expect(event.kind).toBe("tackle_made");
		expect(event.playerId).toBe("p7");
		// The action stays armed so the next player is one tap, not two.
		await waitFor(() => expect(result.current.armed.kind).toBe("tackle_made"));
	});

	// Spec section 7's worked example: one action, three players, three taps.
	it("logs three events for an armed action and three players", async () => {
		const { result } = setup();

		act(() => result.current.tapAction("tackle_made"));
		act(() => result.current.tapPlayer("p7"));
		act(() => result.current.tapPlayer("p8"));
		act(() => result.current.tapPlayer("p9"));

		const events = await eventsSettle(3);
		expect(events.map((e) => e.playerId)).toEqual(["p7", "p8", "p9"]);
		expect(events.every((e) => e.kind === "tackle_made")).toBe(true);
	});

	it("logs an event when a player is tapped then an action", async () => {
		const { result } = setup();

		act(() => result.current.tapPlayer("p7"));
		await waitFor(() => expect(result.current.armed.playerId).toBe("p7"));
		act(() => result.current.tapAction("carry"));

		const [event] = await eventsSettle(1);
		expect(event).toMatchObject({ kind: "carry", playerId: "p7" });
		await waitFor(() => expect(result.current.armed.playerId).toBeNull());
	});

	// Backing out has to be possible without logging something to undo.
	it("disarms an action tapped twice", async () => {
		const { result } = setup();

		act(() => result.current.tapAction("tackle_made"));
		act(() => result.current.tapAction("tackle_made"));

		await waitFor(() => expect(result.current.armed.kind).toBeNull());
		expect(await localEvents(MATCH)).toEqual([]);
	});

	it("replaces the armed action when a second one is tapped", async () => {
		const { result } = setup();

		act(() => result.current.tapAction("tackle_made"));
		act(() => result.current.tapAction("carry"));

		await waitFor(() => expect(result.current.armed.kind).toBe("carry"));
		expect(await localEvents(MATCH)).toEqual([]);
	});

	it("logs a team action immediately, with no player and no arming", async () => {
		const { result } = setup();

		act(() => result.current.tapAction("scrum_won"));

		const [event] = await eventsSettle(1);
		expect(event.kind).toBe("scrum_won");
		expect(event.playerId).toBeUndefined();
		await waitFor(() => expect(result.current.armed.kind).toBeNull());
	});

	// A player in the bin or sent off is not out there to make a tackle.
	it("refuses to tag a player who is not on the pitch", async () => {
		const { result } = setup();

		act(() => result.current.tapAction("tackle_made"));
		act(() => result.current.tapPlayer("p16"));

		await new Promise((resolve) => setTimeout(resolve, 10));
		expect(await localEvents(MATCH)).toEqual([]);
	});
});

describe("substitutions", () => {
	// A bare sub would be refused by the server, so the screen collects both
	// players before it writes anything: off first, then on.
	it("takes the player coming off and then the one coming on", async () => {
		const { result } = setup();

		act(() => result.current.tapAction("sub"));
		act(() => result.current.tapPlayer("p7"));
		expect(await localEvents(MATCH)).toEqual([]);

		act(() => result.current.tapPlayer("p16"));

		const [event] = await eventsSettle(1);
		expect(event.kind).toBe("sub");
		expect(event.payload).toEqual({ offPlayerId: "p7", onPlayerId: "p16" });
		await waitFor(() => expect(result.current.armed.kind).toBeNull());
	});
});

describe("undo", () => {
	it("voids the most recent event this device tagged", async () => {
		const { result } = setup();
		act(() => result.current.tapAction("tackle_made"));
		act(() => result.current.tapPlayer("p7"));
		act(() => result.current.tapPlayer("p8"));
		const tagged = await eventsSettle(2);
		// Undo reads the log the hook can see, not the one on disk.
		await waitFor(() => expect(result.current.events).toHaveLength(2));

		act(() => result.current.undo());

		const events = await eventsSettle(3);
		const undoEvent = events.find((e) => e.kind === "void");
		expect(undoEvent?.voidsId).toBe(tagged[1].id);
	});

	// Undo is this device's undo. Cancelling the other coach's tap from here
	// would be a correction nobody on that iPad asked for or can see coming.
	it("never voids another device's event", async () => {
		const { result } = setup();
		await appendEvent(MATCH, {
			id: "remote-1",
			kind: "tackle_made",
			playerId: "p7",
			clockMs: 1000,
			period: 1,
			deviceId: "device-2",
		});
		await waitFor(async () => expect(await localEvents(MATCH)).toHaveLength(1));

		act(() => result.current.undo());

		await new Promise((resolve) => setTimeout(resolve, 10));
		expect(await localEvents(MATCH)).toHaveLength(1);
	});

	it("does nothing when there is nothing to undo", async () => {
		const { result } = setup();

		act(() => result.current.undo());

		await new Promise((resolve) => setTimeout(resolve, 10));
		expect(await localEvents(MATCH)).toEqual([]);
	});
});

describe("the clock", () => {
	it("starts, pauses and resumes", async () => {
		const { result } = setup();

		act(() => result.current.toggleClock());
		await waitFor(() => expect(result.current.running).toBe(true));

		act(() => result.current.toggleClock());
		await waitFor(() => expect(result.current.running).toBe(false));

		act(() => result.current.toggleClock());
		await waitFor(() => expect(result.current.running).toBe(true));

		const events = await eventsSettle(3);
		expect(events.map((e) => e.kind)).toEqual([
			"period_started",
			"clock_paused",
			"clock_resumed",
		]);
	});

	it("ends a period and the match", async () => {
		const { result } = setup();
		act(() => result.current.toggleClock());
		await waitFor(() => expect(result.current.running).toBe(true));

		act(() => result.current.endPeriod());
		await waitFor(() => expect(result.current.running).toBe(false));
		act(() => result.current.endMatch());

		const events = await eventsSettle(3);
		expect(events.map((e) => e.kind)).toEqual([
			"period_started",
			"period_ended",
			"match_ended",
		]);
	});

	// Nothing accumulates while the clock is stopped, and that includes the
	// stamp on a tap made during the stoppage.
	it("freezes the stamped clock while paused", async () => {
		const { result, rerender } = setup();

		act(() => result.current.toggleClock());
		await waitFor(() => expect(result.current.running).toBe(true));

		advance(60_000);
		act(() => result.current.toggleClock());
		await waitFor(() => expect(result.current.running).toBe(false));
		rerender();
		const pausedAt = result.current.clockMs;
		expect(pausedAt).toBe(60_000);

		advance(120_000);
		act(() => result.current.tapAction("scrum_won"));
		advance(30_000);
		act(() => result.current.tapAction("scrum_won"));

		const events = await eventsSettle(4);
		const stamps = events
			.filter((e) => e.kind === "scrum_won")
			.map((e) => e.clockMs);
		expect(stamps).toEqual([pausedAt, pausedAt]);
	});

	// The other coach stopping their clock has to stop this one, or the two
	// devices stamp different times for the same moment.
	it("follows a pause that arrives from another device", async () => {
		const { result, rerender } = setup();

		act(() => result.current.toggleClock());
		await waitFor(() => expect(result.current.running).toBe(true));
		advance(60_000);

		await appendEvent(MATCH, {
			id: "ffffffff-0000-7000-8000-000000000001",
			kind: "clock_paused",
			clockMs: 90_000,
			period: 1,
			deviceId: "device-2",
		});

		await waitFor(() => expect(result.current.running).toBe(false));
		expect(result.current.clockMs).toBe(90_000);

		// Nothing this device does makes it tick again on its own.
		advance(30_000);
		rerender();
		expect(result.current.clockMs).toBe(90_000);
	});

	// A referee stops time at 40:00 and the half plays on with the clock
	// stopped, which is also what keeps clockMs monotonic into the second half.
	it("stops itself at the end of a half, once", async () => {
		await appendEvent(MATCH, {
			id: "00000000-0000-7000-8000-000000000001",
			kind: "period_started",
			clockMs: 0,
			period: 1,
			deviceId: DEVICE,
		});
		await appendEvent(MATCH, {
			id: "00000000-0000-7000-8000-000000000002",
			kind: "clock_resumed",
			clockMs: 2_390_000,
			period: 1,
			deviceId: DEVICE,
		});
		const { result, rerender } = setup();
		await waitFor(() => expect(result.current.running).toBe(true));

		advance(20_000);
		rerender();

		await waitFor(() => expect(result.current.running).toBe(false));
		const pauses = (await localEvents(MATCH)).filter(
			(e) => e.kind === "clock_paused",
		);
		expect(pauses).toHaveLength(1);
		expect(pauses[0].clockMs).toBe(2_400_000);

		// The overrun is tagged with the clock stopped, then the coach resumes
		// for the second half rather than the boundary firing again.
		act(() => result.current.toggleClock());
		await waitFor(() => expect(result.current.running).toBe(true));
		advance(20_000);
		rerender();

		expect(
			(await localEvents(MATCH)).filter((e) => e.kind === "clock_paused"),
		).toHaveLength(1);
	});
});

describe("the toggles", () => {
	it("append the whole zone and possession state, not a delta", async () => {
		const { result } = setup();

		act(() => result.current.setZone("their_22"));
		const [zoneEvent] = await eventsSettle(1);
		expect(zoneEvent.kind).toBe("zone_changed");
		expect(zoneEvent.payload).toEqual({
			zone: "their_22",
			possession: "us",
		});

		act(() => result.current.setPossession("them"));
		const events = await eventsSettle(2);
		expect(events[1].payload).toEqual({
			zone: "their_22",
			possession: "them",
		});
	});
});

describe("every event", () => {
	it("carries the device, the period and the clock", async () => {
		const { result } = setup();

		act(() => result.current.toggleClock());
		await waitFor(() => expect(result.current.running).toBe(true));
		advance(65_000);
		act(() => result.current.tapAction("tackle_made"));
		act(() => result.current.tapPlayer("p7"));

		const events = await eventsSettle(2);
		const tackle = events.find((e) => e.kind === "tackle_made");
		expect(tackle?.deviceId).toBe(DEVICE);
		expect(tackle?.period).toBe(1);
		expect(tackle?.clockMs).toBe(65_000);
	});
});
