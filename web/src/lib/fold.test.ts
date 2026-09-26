import { describe, expect, it } from "vitest";
import type { Event, Match } from "../types/core";
import { fold } from "./fold";

function match(starters: { jersey: number; playerId: string }[] = []): Match {
	return {
		id: "match-1",
		seasonId: "2026-27",
		opponent: "Castelnau RC",
		kickoffAt: "2026-03-14T15:00:00Z",
		competition: "Regional 1",
		venue: "home",
		status: "scheduled",
		lineup: { starters, bench: [] },
	};
}

describe("fold", () => {
	// The tagging screen renders before anyone has picked an XV, so an empty
	// selection has to produce a usable state rather than a hole to guard
	// against at every call site.
	it("returns a usable state for a match with no lineup", () => {
		const state = fold(match(), []);

		expect(state.status).toBe("scheduled");
		expect(state.clockMs).toBe(0);
		expect(state.running).toBe(false);
		expect(state.score).toEqual({ us: 0, them: 0 });
		expect(state.players).toEqual({});
		expect(state.slices).toEqual([]);
		expect(state.voidedIds).toEqual([]);
		expect(state.territoryMs).toEqual({});
		expect(state.counts).toEqual({});
	});

	// The caller passes a live Dexie query result straight in. Sorting it in
	// place would reorder someone else's array, and the bug would surface as a
	// list that reshuffles while a coach is reading it.
	it("does not mutate the events it is given", () => {
		const events: Event[] = [
			{
				id: "e2",
				kind: "tackle_made",
				playerId: "p7",
				clockMs: 120000,
				period: 1,
				deviceId: "d1",
			},
			{
				id: "e1",
				kind: "period_started",
				clockMs: 0,
				period: 1,
				deviceId: "d1",
			},
		];
		const before = [...events];

		fold(match([{ jersey: 7, playerId: "p7" }]), events);

		expect(events).toEqual(before);
		expect(events[0].id).toBe("e2");
	});

	it("ignores a kind it does not know rather than throwing", () => {
		const events = [
			{
				id: "e1",
				kind: "teleportation" as Event["kind"],
				playerId: "p7",
				clockMs: 1000,
				period: 1,
				deviceId: "d1",
			},
		];

		const state = fold(match([{ jersey: 7, playerId: "p7" }]), events);

		expect(state.counts).toEqual({});
		expect(state.status).toBe("scheduled");
	});
});
