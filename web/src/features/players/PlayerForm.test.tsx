import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { describe, expect, it, vi } from "vitest";
import { PlayerForm } from "./PlayerForm";

describe("PlayerForm", () => {
	it("submits the entered player", async () => {
		const onSubmit = vi.fn();
		render(<PlayerForm onSubmit={onSubmit} onCancel={() => {}} />);

		await userEvent.type(screen.getByLabelText(/first name/i), "Thomas");
		await userEvent.type(screen.getByLabelText(/last name/i), "Lefevre");
		await userEvent.click(screen.getByLabelText(/openside flanker/i));
		await userEvent.click(screen.getByRole("button", { name: /save/i }));

		expect(onSubmit).toHaveBeenCalledWith(
			expect.objectContaining({
				firstName: "Thomas",
				lastName: "Lefevre",
				positions: ["openside"],
				status: "active",
			}),
		);
	});

	it("refuses to submit without a position", async () => {
		const onSubmit = vi.fn();
		render(<PlayerForm onSubmit={onSubmit} onCancel={() => {}} />);

		await userEvent.type(screen.getByLabelText(/first name/i), "Thomas");
		await userEvent.type(screen.getByLabelText(/last name/i), "Lefevre");
		await userEvent.click(screen.getByRole("button", { name: /save/i }));

		expect(onSubmit).not.toHaveBeenCalled();
		expect(screen.getByRole("alert")).toHaveTextContent(/position/i);
	});

	it("refuses to submit without a name", async () => {
		const onSubmit = vi.fn();
		render(<PlayerForm onSubmit={onSubmit} onCancel={() => {}} />);

		await userEvent.click(screen.getByLabelText(/openside flanker/i));
		await userEvent.click(screen.getByRole("button", { name: /save/i }));

		expect(onSubmit).not.toHaveBeenCalled();
		expect(screen.getByRole("alert")).toHaveTextContent(/name/i);
	});

	it("allows more than one position", async () => {
		const onSubmit = vi.fn();
		render(<PlayerForm onSubmit={onSubmit} onCancel={() => {}} />);

		await userEvent.type(screen.getByLabelText(/first name/i), "Marie");
		await userEvent.type(screen.getByLabelText(/last name/i), "Girard");
		await userEvent.click(screen.getByLabelText(/blindside flanker/i));
		await userEvent.click(screen.getByLabelText(/number 8/i));
		await userEvent.click(screen.getByRole("button", { name: /save/i }));

		expect(onSubmit.mock.calls[0][0].positions).toEqual([
			"blindside",
			"number_eight",
		]);
	});

	it("pre-fills when editing", () => {
		render(
			<PlayerForm
				player={{
					id: "p1",
					clubId: "club-1",
					firstName: "Marie",
					lastName: "Girard",
					dob: "2001-02-03",
					positions: ["lock"],
					teamIds: ["team-1"],
					status: "active",
				}}
				onSubmit={() => {}}
				onCancel={() => {}}
			/>,
		);

		expect(screen.getByLabelText(/first name/i)).toHaveValue("Marie");
		expect(screen.getByLabelText(/lock/i)).toBeChecked();
		expect(screen.getByLabelText(/openside flanker/i)).not.toBeChecked();
	});

	it("keeps the id and teamIds of the player being edited", async () => {
		const onSubmit = vi.fn();
		render(
			<PlayerForm
				player={{
					id: "p1",
					clubId: "club-1",
					firstName: "Marie",
					lastName: "Girard",
					dob: "",
					positions: ["lock"],
					teamIds: ["team-1", "team-2"],
					status: "active",
				}}
				onSubmit={onSubmit}
				onCancel={() => {}}
			/>,
		);

		await userEvent.click(screen.getByRole("button", { name: /save/i }));

		expect(onSubmit).toHaveBeenCalledWith(
			expect.objectContaining({ id: "p1", teamIds: ["team-1", "team-2"] }),
		);
	});
});
