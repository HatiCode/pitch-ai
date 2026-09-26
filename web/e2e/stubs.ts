import type { Route } from "@playwright/test";
import type { CatalogueEntry, Event, Match, Player } from "../src/types/core";

export const TEAM_ID = "team-e2e";
export const MATCH_ID = "match-e2e";

// A cut-down catalogue: enough kinds to tag a passage of play, few enough that
// the test reads like the sequence a coach would actually tap.
export const CATALOGUE: CatalogueEntry[] = [
	{ kind: "tackle_made", label: "Tackle made", group: "play", player: true },
	{ kind: "carry", label: "Carry", group: "play", player: true },
	{ kind: "try", label: "Try", group: "play", player: true, points: 5 },
	{ kind: "sub", label: "Substitution", group: "play", player: false },
	{ kind: "scrum_won", label: "Scrum won", group: "set_piece", player: false },
	{
		kind: "conversion_made",
		label: "Conversion",
		group: "score",
		player: true,
		points: 2,
	},
	{
		kind: "yellow_card",
		label: "Yellow card",
		group: "discipline",
		player: true,
	},
];

export const PLAYERS: Player[] = Array.from({ length: 23 }, (_, i) => ({
	id: `p${i + 1}`,
	clubId: "club-e2e",
	firstName: "First",
	lastName: `Player${i + 1}`,
	dob: "2001-04-02",
	positions: ["openside"],
	teamIds: [TEAM_ID],
	status: "active",
}));

export const MATCH: Match = {
	id: MATCH_ID,
	clubId: "club-e2e",
	teamId: TEAM_ID,
	seasonId: "2026-27",
	opponent: "Lansdowne",
	kickoffAt: "2026-09-19T14:00:00Z",
	competition: "Regional 1",
	venue: "home",
	status: "scheduled",
	lineup: {
		starters: Array.from({ length: 15 }, (_, i) => ({
			jersey: i + 1,
			playerId: `p${i + 1}`,
		})),
		bench: Array.from({ length: 8 }, (_, i) => ({
			jersey: i + 16,
			playerId: `p${i + 16}`,
		})),
	},
};

export type Appended = { events: Event[] };

/**
 * Stands in for the Go server, recording every event the outbox sends.
 *
 * The server's own behaviour is covered by its tests; what this has to be is
 * honest about what arrived, so the test can assert the log the coach produced
 * reached it exactly once.
 */
export function createApiStub() {
	const appended: Event[] = [];
	let failing = false;

	async function handle(route: Route): Promise<void> {
		const url = new URL(route.request().url());
		const path = url.pathname.replace(/^\/api/, "");

		if (failing) {
			await route.abort("internetdisconnected");
			return;
		}

		if (path === "/catalogue") {
			await json(route, CATALOGUE);
			return;
		}
		if (path === "/players") {
			await json(route, PLAYERS);
			return;
		}
		if (path === `/teams/${TEAM_ID}/matches/${MATCH_ID}`) {
			await json(route, MATCH);
			return;
		}
		if (path === `/teams/${TEAM_ID}/matches/${MATCH_ID}/events`) {
			if (route.request().method() === "POST") {
				const body = route.request().postDataJSON() as Appended;
				appended.push(...body.events);
				await json(route, {});
				return;
			}
			// Nothing comes back down: this test is about what leaves the device.
			await json(route, { events: [], cursor: "" });
			return;
		}
		await json(route, {});
	}

	return {
		handle,
		appended,
		setFailing: (next: boolean) => {
			failing = next;
		},
	};
}

async function json(route: Route, body: unknown): Promise<void> {
	await route.fulfill({
		status: 200,
		contentType: "application/json",
		body: JSON.stringify(body),
	});
}
