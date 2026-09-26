import { beforeEach, describe, expect, it } from "vitest";
import type { CatalogueEntry, Event, Match, Player } from "../types/core";
import {
	appendEvent,
	cacheCatalogue,
	cachedCatalogue,
	cachedMatch,
	cachedPlayers,
	cacheMatch,
	cachePlayers,
	db,
	deviceId,
	getCursor,
	localEvents,
	markSynced,
	putServerEvents,
	setCursor,
	unsyncedEvents,
} from "./db";

const MATCH = "match-1";

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

beforeEach(async () => {
	await Promise.all(db.tables.map((table) => table.clear()));
});

describe("the event log", () => {
	it("stores a tap unsynced and reads it back", async () => {
		await appendEvent(MATCH, tackle("e1", 1000));

		expect(await localEvents(MATCH)).toEqual([tackle("e1", 1000)]);
		expect(await unsyncedEvents(MATCH, 10)).toEqual([tackle("e1", 1000)]);
	});

	it("keeps one match's events out of another's", async () => {
		await appendEvent(MATCH, tackle("e1", 1000));
		await appendEvent("match-2", tackle("e2", 2000));

		expect(await localEvents(MATCH)).toEqual([tackle("e1", 1000)]);
	});

	it("drains the outbox oldest first, up to the limit", async () => {
		await appendEvent(MATCH, tackle("e1", 1000));
		await appendEvent(MATCH, tackle("e2", 2000));
		await appendEvent(MATCH, tackle("e3", 3000));

		const batch = await unsyncedEvents(MATCH, 2);

		expect(batch.map((e) => e.id)).toEqual(["e1", "e2"]);
	});

	// The log is append-only: a synced event leaves the outbox but stays in the
	// log, because the fold reads every event the device has ever recorded.
	it("marks events synced without removing them from the log", async () => {
		await appendEvent(MATCH, tackle("e1", 1000));
		await appendEvent(MATCH, tackle("e2", 2000));

		await markSynced(["e1"]);

		expect((await unsyncedEvents(MATCH, 10)).map((e) => e.id)).toEqual(["e2"]);
		expect((await localEvents(MATCH)).map((e) => e.id)).toEqual(["e1", "e2"]);
	});

	// A device's own event coming back from the server must not become a second
	// row, or every count it feeds would double.
	it("upserts a server event onto the local row it already had", async () => {
		await appendEvent(MATCH, tackle("e1", 1000));

		await putServerEvents(MATCH, [tackle("e1", 1000)]);

		expect(await localEvents(MATCH)).toHaveLength(1);
		expect(await unsyncedEvents(MATCH, 10)).toEqual([]);
	});

	it("stores events that only ever came from the server", async () => {
		await putServerEvents(MATCH, [tackle("e9", 9000)]);

		expect((await localEvents(MATCH)).map((e) => e.id)).toEqual(["e9"]);
		expect(await unsyncedEvents(MATCH, 10)).toEqual([]);
	});
});

describe("device identity", () => {
	// Every event carries it, so a reload that minted a new one would make one
	// iPad look like two in the log.
	it("is generated once and persisted", async () => {
		const first = await deviceId();
		const second = await deviceId();

		expect(first).toBe(second);
		expect(first).not.toBe("");
		// Persisted rather than held in memory, which is what survives a reload.
		expect(await db.meta.get("deviceId")).toEqual({
			key: "deviceId",
			value: first,
		});
	});
});

describe("the sync cursor and the caches", () => {
	it("round-trips the cursor per match", async () => {
		expect(await getCursor(MATCH)).toBe("");

		await setCursor(MATCH, "1758288000000000:e1");

		expect(await getCursor(MATCH)).toBe("1758288000000000:e1");
		expect(await getCursor("match-2")).toBe("");
	});

	it("round-trips the catalogue", async () => {
		const entries: CatalogueEntry[] = [
			{ kind: "tackle_made", label: "Tackle", group: "play", player: true },
			{ kind: "try", label: "Try", group: "play", player: true, points: 5 },
		];

		await cacheCatalogue(entries);

		expect(await cachedCatalogue()).toEqual(entries);
	});

	it("returns an empty catalogue before anything is cached", async () => {
		expect(await cachedCatalogue()).toEqual([]);
	});

	it("round-trips a match and its players", async () => {
		const match: Match = {
			id: MATCH,
			clubId: "club-1",
			teamId: "team-1",
			seasonId: "2026-27",
			opponent: "Castelnau RC",
			kickoffAt: "2026-03-14T15:00:00Z",
			competition: "Regional 1",
			venue: "home",
			status: "scheduled",
			lineup: { starters: [], bench: [] },
		};
		const players: Player[] = [
			{
				id: "player-7",
				clubId: "club-1",
				firstName: "First",
				lastName: "Last",
				dob: "2001-04-02",
				positions: ["openside"],
				teamIds: ["team-1"],
				status: "active",
			},
		];

		await cacheMatch(match);
		await cachePlayers(players);

		expect(await cachedMatch(MATCH)).toEqual(match);
		expect(await cachedPlayers()).toEqual(players);
	});

	it("returns undefined for a match it has never seen", async () => {
		expect(await cachedMatch("nope")).toBeUndefined();
	});
});
