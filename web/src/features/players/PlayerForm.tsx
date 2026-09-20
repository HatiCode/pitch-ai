import { type FormEvent, useState } from "react";
import type { Player, Position } from "../../types/core";
import { POSITIONS } from "../../types/groups";
import { POSITION_LABELS } from "./positions";

type Props = {
	player?: Player;
	onSubmit: (player: Player) => void;
	onCancel: () => void;
};

export function PlayerForm({ player, onSubmit, onCancel }: Props) {
	const [firstName, setFirstName] = useState(player?.firstName ?? "");
	const [lastName, setLastName] = useState(player?.lastName ?? "");
	const [dob, setDob] = useState(player?.dob ?? "");
	const [positions, setPositions] = useState<Position[]>(
		player?.positions ?? [],
	);
	const [error, setError] = useState<string | null>(null);

	function togglePosition(position: Position) {
		setPositions((current) =>
			current.includes(position)
				? current.filter((p) => p !== position)
				: // Keep jersey order regardless of click order, so the stored list
					// reads the way a coach would say it.
					POSITIONS.filter((p) => p === position || current.includes(p)),
		);
	}

	function handleSubmit(event: FormEvent) {
		event.preventDefault();

		if (!firstName.trim() || !lastName.trim()) {
			setError("First and last name are required.");
			return;
		}
		if (positions.length === 0) {
			setError("Select at least one position.");
			return;
		}

		setError(null);
		onSubmit({
			id: player?.id ?? "",
			clubId: player?.clubId ?? "",
			firstName: firstName.trim(),
			lastName: lastName.trim(),
			dob,
			positions,
			teamIds: player?.teamIds ?? [],
			status: player?.status ?? "active",
		});
	}

	return (
		<form
			onSubmit={handleSubmit}
			className="space-y-4 rounded-xl bg-white p-5 shadow-sm"
		>
			{error && (
				<p
					role="alert"
					className="rounded bg-red-50 px-3 py-2 text-sm text-red-700"
				>
					{error}
				</p>
			)}

			<div className="grid gap-4 sm:grid-cols-2">
				<label className="block text-sm">
					<span className="mb-1 block font-medium">First name</span>
					<input
						value={firstName}
						onChange={(e) => setFirstName(e.target.value)}
						className="w-full rounded border border-slate-300 px-3 py-2"
					/>
				</label>
				<label className="block text-sm">
					<span className="mb-1 block font-medium">Last name</span>
					<input
						value={lastName}
						onChange={(e) => setLastName(e.target.value)}
						className="w-full rounded border border-slate-300 px-3 py-2"
					/>
				</label>
			</div>

			<label className="block text-sm">
				<span className="mb-1 block font-medium">Date of birth (optional)</span>
				<input
					type="date"
					value={dob}
					onChange={(e) => setDob(e.target.value)}
					className="rounded border border-slate-300 px-3 py-2"
				/>
			</label>

			<fieldset>
				<legend className="mb-2 text-sm font-medium">Positions</legend>
				<div className="grid gap-2 sm:grid-cols-3">
					{POSITIONS.map((position) => (
						<label key={position} className="flex items-center gap-2 text-sm">
							<input
								type="checkbox"
								checked={positions.includes(position)}
								onChange={() => togglePosition(position)}
							/>
							{POSITION_LABELS[position]}
						</label>
					))}
				</div>
			</fieldset>

			<div className="flex gap-3">
				<button
					type="submit"
					className="rounded bg-slate-900 px-4 py-2 font-semibold text-white"
				>
					Save
				</button>
				<button
					type="button"
					onClick={onCancel}
					className="rounded px-4 py-2 text-slate-600"
				>
					Cancel
				</button>
			</div>
		</form>
	);
}
