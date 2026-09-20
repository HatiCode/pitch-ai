import { render, screen, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { describe, expect, it, vi } from "vitest";
import type { Player } from "../../types/core";
import { LineupPicker } from "./LineupPicker";

function squad(n: number): Player[] {
	return Array.from({ length: n }, (_, i) => ({
		id: `player-${i + 1}`,
		clubId: "club-1",
		firstName: "First",
		lastName: `Last${i + 1}`,
		dob: "",
		positions: ["openside" as const],
		teamIds: ["team-1"],
		status: "active" as const,
	}));
}

const emptyLineup = { starters: [], bench: [] };

async function fillAllFifteen() {
	for (let jersey = 1; jersey <= 15; jersey++) {
		await userEvent.selectOptions(
			screen.getByLabelText(new RegExp(`^jersey ${jersey}$`, "i")),
			`player-${jersey}`,
		);
	}
}

describe("LineupPicker", () => {
	it("saves a complete starting XV", async () => {
		const onSave = vi.fn();
		render(
			<LineupPicker players={squad(20)} lineup={emptyLineup} onSave={onSave} />,
		);

		await fillAllFifteen();
		await userEvent.click(screen.getByRole("button", { name: /save lineup/i }));

		expect(onSave).toHaveBeenCalledTimes(1);
		expect(onSave.mock.calls[0][0].starters).toHaveLength(15);
	});

	it("refuses to save an incomplete XV and says how many are missing", async () => {
		const onSave = vi.fn();
		render(
			<LineupPicker players={squad(20)} lineup={emptyLineup} onSave={onSave} />,
		);

		await userEvent.selectOptions(
			screen.getByLabelText(/^jersey 1$/i),
			"player-1",
		);
		await userEvent.click(screen.getByRole("button", { name: /save lineup/i }));

		expect(onSave).not.toHaveBeenCalled();
		expect(screen.getByRole("alert")).toHaveTextContent(/14/);
	});

	it("does not offer a player who is already selected elsewhere", async () => {
		render(
			<LineupPicker
				players={squad(20)}
				lineup={emptyLineup}
				onSave={() => {}}
			/>,
		);

		await userEvent.selectOptions(
			screen.getByLabelText(/^jersey 1$/i),
			"player-1",
		);

		const jerseyTwo = screen.getByLabelText(/^jersey 2$/i);
		expect(
			within(jerseyTwo).queryByRole("option", { name: /Last1$/ }),
		).toBeNull();
	});

	it("keeps the current pick selectable in its own slot", async () => {
		render(
			<LineupPicker
				players={squad(20)}
				lineup={emptyLineup}
				onSave={() => {}}
			/>,
		);

		const jerseyOne = screen.getByLabelText(/^jersey 1$/i);
		await userEvent.selectOptions(jerseyOne, "player-1");

		expect(
			within(jerseyOne).queryByRole("option", { name: /Last1$/ }),
		).not.toBeNull();
		expect(jerseyOne).toHaveValue("player-1");
	});

	it("includes chosen replacements but allows an empty bench", async () => {
		const onSave = vi.fn();
		render(
			<LineupPicker players={squad(20)} lineup={emptyLineup} onSave={onSave} />,
		);

		await fillAllFifteen();
		await userEvent.selectOptions(
			screen.getByLabelText(/^jersey 16$/i),
			"player-16",
		);
		await userEvent.click(screen.getByRole("button", { name: /save lineup/i }));

		const saved = onSave.mock.calls[0][0];
		expect(saved.bench).toHaveLength(1);
		expect(saved.bench[0]).toEqual({ jersey: 16, playerId: "player-16" });
	});

	it("pre-fills an existing lineup", () => {
		render(
			<LineupPicker
				players={squad(20)}
				lineup={{
					starters: [{ jersey: 7, playerId: "player-7" }],
					bench: [{ jersey: 16, playerId: "player-16" }],
				}}
				onSave={() => {}}
			/>,
		);

		expect(screen.getByLabelText(/^jersey 7$/i)).toHaveValue("player-7");
		expect(screen.getByLabelText(/^jersey 16$/i)).toHaveValue("player-16");
	});

	it("can clear a slot again", async () => {
		render(
			<LineupPicker
				players={squad(20)}
				lineup={emptyLineup}
				onSave={() => {}}
			/>,
		);

		const jerseyOne = screen.getByLabelText(/^jersey 1$/i);
		await userEvent.selectOptions(jerseyOne, "player-1");
		await userEvent.selectOptions(jerseyOne, "");

		expect(jerseyOne).toHaveValue("");
		expect(
			within(screen.getByLabelText(/^jersey 2$/i)).queryByRole("option", {
				name: /Last1$/,
			}),
		).not.toBeNull();
	});
});
