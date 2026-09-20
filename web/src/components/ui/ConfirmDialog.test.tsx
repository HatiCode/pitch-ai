import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { describe, expect, it, vi } from "vitest";
import { ConfirmDialog } from "./ConfirmDialog";

function renderDialog(
	overrides: Partial<Parameters<typeof ConfirmDialog>[0]> = {},
) {
	const props = {
		title: "Delete “Colts”?",
		confirmLabel: "Delete squad",
		busyLabel: "Deleting…",
		onConfirm: vi.fn(),
		onCancel: vi.fn(),
		children: <p>This cannot be undone.</p>,
		...overrides,
	};
	render(<ConfirmDialog {...props} />);
	return props;
}

describe("ConfirmDialog", () => {
	it("shows the title and the consequences", () => {
		renderDialog();

		expect(screen.getByRole("dialog")).toBeInTheDocument();
		expect(screen.getByText(/Delete “Colts”\?/)).toBeInTheDocument();
		expect(screen.getByText(/cannot be undone/i)).toBeInTheDocument();
	});

	it("calls onConfirm when the confirm button is pressed", async () => {
		const props = renderDialog();

		await userEvent.click(
			screen.getByRole("button", { name: /delete squad/i }),
		);

		expect(props.onConfirm).toHaveBeenCalledTimes(1);
		expect(props.onCancel).not.toHaveBeenCalled();
	});

	it("calls onCancel when cancelled", async () => {
		const props = renderDialog();

		await userEvent.click(screen.getByRole("button", { name: /cancel/i }));

		expect(props.onCancel).toHaveBeenCalledTimes(1);
		expect(props.onConfirm).not.toHaveBeenCalled();
	});

	it("cancels on Escape, so the destructive action is never the default", async () => {
		const props = renderDialog();

		await userEvent.keyboard("{Escape}");

		expect(props.onCancel).toHaveBeenCalledTimes(1);
		expect(props.onConfirm).not.toHaveBeenCalled();
	});

	it("disables both buttons while the action is in flight", () => {
		renderDialog({ busy: true });

		expect(screen.getByRole("button", { name: /deleting/i })).toBeDisabled();
		expect(screen.getByRole("button", { name: /cancel/i })).toBeDisabled();
	});

	it("labels the dialog for screen readers", () => {
		renderDialog();

		expect(screen.getByRole("dialog")).toHaveAccessibleName(/Delete “Colts”\?/);
	});
});
