import Dexie, { type Table } from "dexie";
import type { CatalogueEntry, Event, Match, Player } from "../types/core";
import { uuidv7 } from "./uuid";

// IndexedDB is the application's state, not a cache in front of it. Every tap
// writes here first and the UI renders from here, so the app behaves the same
// way with signal and without.

// LocalEvent is an event plus the two things only the device needs to know:
// which match it belongs to, and whether the server has it yet.
//
// synced is 0 or 1 rather than a boolean because IndexedDB cannot index
// booleans, and the outbox query is an index lookup on [matchId+synced].
export type LocalEvent = Event & { matchId: string; synced: 0 | 1 };

type MetaRow = { key: string; value: string };

class PitchDB extends Dexie {
	events!: Table<LocalEvent, string>;
	matches!: Table<Match, string>;
	players!: Table<Player, string>;
	meta!: Table<MetaRow, string>;

	constructor() {
		super("pitch-ai");
		this.version(1).stores({
			// There is no outbox table: the synced flag on the event row is the
			// outbox. One table means one write per tap and no chance of the two
			// drifting apart.
			events: "id, matchId, [matchId+synced]",
			matches: "id",
			players: "id",
			meta: "key",
		});
	}
}

export const db = new PitchDB();

const DEVICE_ID_KEY = "deviceId";
const CATALOGUE_KEY = "catalogue";
const cursorKey = (matchId: string) => `cursor:${matchId}`;

/**
 * Returns this device's identifier, minting one on first use.
 *
 * It is stored rather than held in memory because every event carries it: a
 * reload that produced a new one would make one iPad look like two devices in
 * a log that is meant to show who tagged what.
 */
export async function deviceId(): Promise<string> {
	const stored = await db.meta.get(DEVICE_ID_KEY);
	if (stored) {
		return stored.value;
	}

	const id = uuidv7();
	await db.meta.put({ key: DEVICE_ID_KEY, value: id });
	return id;
}

/** Records a tap. It is unsynced until the outbox says otherwise. */
export async function appendEvent(
	matchId: string,
	event: Event,
): Promise<void> {
	await db.events.put({ ...event, matchId, synced: 0 });
}

/**
 * Returns every event this device knows about for a match, oldest first.
 *
 * Sorting by ID is sorting by time: event IDs are UUIDv7, so tap order is
 * recoverable without storing a separate sequence, including across the two
 * devices that may have written the log.
 *
 * Synced events stay here: the log is append-only and the fold reads all of
 * it, so syncing changes only whether an event still needs sending.
 */
export async function localEvents(matchId: string): Promise<Event[]> {
	const rows = await db.events.where("matchId").equals(matchId).sortBy("id");
	return rows.map(toEvent);
}

/** Returns the next events to push, oldest first. */
export async function unsyncedEvents(
	matchId: string,
	limit: number,
): Promise<Event[]> {
	const rows = await db.events
		.where("[matchId+synced]")
		.equals([matchId, 0])
		.sortBy("id");
	return rows.slice(0, limit).map(toEvent);
}

/** Marks a pushed batch as no longer owed to the server. */
export async function markSynced(ids: string[]): Promise<void> {
	await db.events.where("id").anyOf(ids).modify({ synced: 1 });
}

/**
 * Stores events pulled from the server.
 *
 * Keyed by event ID, so a device's own tap coming back to it collapses onto
 * the row it already had instead of becoming a second one that would double
 * every count it feeds.
 */
export async function putServerEvents(
	matchId: string,
	events: Event[],
): Promise<void> {
	await db.events.bulkPut(
		events.map((event) => ({ ...event, matchId, synced: 1 as const })),
	);
}

export async function getCursor(matchId: string): Promise<string> {
	const row = await db.meta.get(cursorKey(matchId));
	return row?.value ?? "";
}

export async function setCursor(
	matchId: string,
	cursor: string,
): Promise<void> {
	await db.meta.put({ key: cursorKey(matchId), value: cursor });
}

export async function cacheMatch(match: Match): Promise<void> {
	await db.matches.put(match);
}

export async function cachedMatch(matchId: string): Promise<Match | undefined> {
	return db.matches.get(matchId);
}

export async function cachePlayers(players: Player[]): Promise<void> {
	await db.players.bulkPut(players);
}

export async function cachedPlayers(): Promise<Player[]> {
	return db.players.toArray();
}

/**
 * The catalogue is one document rather than a table: it is read whole, written
 * whole, and never queried by kind.
 */
export async function cacheCatalogue(entries: CatalogueEntry[]): Promise<void> {
	await db.meta.put({ key: CATALOGUE_KEY, value: JSON.stringify(entries) });
}

export async function cachedCatalogue(): Promise<CatalogueEntry[]> {
	const row = await db.meta.get(CATALOGUE_KEY);
	if (!row) {
		return [];
	}
	return JSON.parse(row.value) as CatalogueEntry[];
}

/** Strips the device-only columns, so callers see the event the server sees. */
function toEvent({ matchId: _matchId, synced: _synced, ...event }: LocalEvent) {
	return event;
}
