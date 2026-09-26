import { render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { MemoryRouter, Route, Routes } from "react-router";
import { beforeEach, describe, expect, it, vi } from "vitest";
import { cacheCatalogue, cacheMatch, cachePlayers, db } from "../../lib/db";
import type { CatalogueEntry, Match, Player } from "../../types/core";
import { TaggingPage } from "./TaggingPage";

const MATCH = "match-1";
const TEAM = "team-1";

// The page reads the club through useAuth; the whole point of this screen is
// that it needs nothing from the network once the cache is warm.
//
// apiFetch is created once, as the real provider's useMemo does. A fresh
// function per render would rebuild the sync on every status change, which
// restarts it, which sets the status again.
const offlineFetch = vi.fn().mockRejectedValue(new Error("offline"));

vi.mock("../../lib/auth", () => ({
	useAuth: () => ({
		apiFetch: offlineFetch,
		membership: { uid: "u1", clubId: "club-1", teamIds: [TEAM], role: "coach" },
	}),
}));

const CATALOGUE: CatalogueEntry[] = [
	{ kind: "tackle_made", label: "Tackle made", group: "play", player: true },
	{ kind: "carry", label: "Carry", group: "play", player: true },
	{ kind: "scrum_won", label: "Scrum won", group: "set_piece", player: false },
];

function player(jersey: number): Player {
	return {
		id: `p${jersey}`,
		clubId: "club-1",
		firstName: "First",
		lastName: `Last${jersey}`,
		dob: "2001-04-02",
		positions: ["openside"],
		teamIds: [TEAM],
		status: "active",
	};
}

const match: Match = {
	id: MATCH,
	clubId: "club-1",
	teamId: TEAM,
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
		bench: Array.from({ length: 8 }, (_, i) => ({
			jersey: i + 16,
			playerId: `p${i + 16}`,
		})),
	},
};

beforeEach(async () => {
	await Promise.all(db.tables.map((table) => table.clear()));
	await cacheMatch(match);
	await cachePlayers(Array.from({ length: 23 }, (_, i) => player(i + 1)));
	await cacheCatalogue(CATALOGUE);
});

function renderPage() {
	return render(
		<MemoryRouter initialEntries={[`/squads/${TEAM}/matches/${MATCH}/tag`]}>
			<Routes>
				<Route
					path="/squads/:teamId/matches/:matchId/tag"
					element={<TaggingPage />}
				/>
			</Routes>
		</MemoryRouter>,
	);
}

describe("the tagging screen", () => {
	it("renders the whole squad from the cache with no network", async () => {
		renderPage();

		await waitFor(() =>
			expect(screen.getByTestId("player-p1")).toBeInTheDocument(),
		);
		for (let jersey = 1; jersey <= 23; jersey++) {
			expect(screen.getByTestId(`player-p${jersey}`)).toBeInTheDocument();
		}
		expect(screen.getByTestId("clock")).toHaveTextContent("00:00");
	});

	it("counts a tackle on the tile it was tagged against", async () => {
		const user = userEvent.setup();
		renderPage();
		await waitFor(() =>
			expect(screen.getByTestId("player-p7")).toBeInTheDocument(),
		);

		await user.click(screen.getByRole("button", { name: "Tackle made" }));
		await user.click(screen.getByTestId("player-p7"));

		await waitFor(() =>
			expect(screen.getByTestId("player-p7")).toHaveTextContent("1"),
		);
		expect(screen.getByTestId("player-p8")).toHaveTextContent("0");
	});
});
