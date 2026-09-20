import { useState } from "react";
import type { Lineup, Player } from "../../types/core";

const STARTER_JERSEYS = Array.from({ length: 15 }, (_, i) => i + 1);
const BENCH_JERSEYS = Array.from({ length: 8 }, (_, i) => i + 16);

type Props = {
	players: Player[];
	lineup: Lineup;
	onSave: (lineup: Lineup) => void;
	saving?: boolean;
};

export function LineupPicker({
	players,
	lineup,
	onSave,
	saving = false,
}: Props) {
	const [selection, setSelection] = useState<Record<number, string>>(() => {
		const initial: Record<number, string> = {};
		for (const slot of [...(lineup.starters ?? []), ...(lineup.bench ?? [])]) {
			initial[slot.jersey] = slot.playerId;
		}
		return initial;
	});
	const [error, setError] = useState<string | null>(null);

	const taken = new Set(Object.values(selection).filter(Boolean));

	function choose(jersey: number, playerId: string) {
		setSelection((current) => {
			const next = { ...current };
			if (playerId) {
				next[jersey] = playerId;
			} else {
				delete next[jersey];
			}
			return next;
		});
	}

	function handleSave() {
		const missing = STARTER_JERSEYS.filter((jersey) => !selection[jersey]);
		if (missing.length > 0) {
			setError(
				`${missing.length} starting ${missing.length === 1 ? "jersey is" : "jerseys are"} still unfilled: ${missing.join(", ")}.`,
			);
			return;
		}

		setError(null);
		onSave({
			starters: STARTER_JERSEYS.map((jersey) => ({
				jersey,
				playerId: selection[jersey],
			})),
			bench: BENCH_JERSEYS.filter((jersey) => selection[jersey]).map(
				(jersey) => ({
					jersey,
					playerId: selection[jersey],
				}),
			),
		});
	}

	function slot(jersey: number) {
		const chosen = selection[jersey] ?? "";
		// Filtering out players picked elsewhere makes the server's "selected
		// twice" error unreachable through the UI.
		const options = players.filter(
			(p) => p.id === chosen || !taken.has(p.id ?? ""),
		);

		return (
			<label key={jersey} className="flex items-center gap-2 text-sm">
				<span className="w-20 shrink-0 font-semibold text-slate-700">
					Jersey {jersey}
				</span>
				<select
					value={chosen}
					onChange={(e) => choose(jersey, e.target.value)}
					className="min-w-0 flex-1 rounded border border-slate-300 px-2 py-1"
				>
					<option value="">—</option>
					{options.map((p) => (
						<option key={p.id} value={p.id}>
							{p.firstName} {p.lastName}
						</option>
					))}
				</select>
			</label>
		);
	}

	return (
		<div className="space-y-5">
			{error && (
				<p
					role="alert"
					className="rounded bg-red-50 px-3 py-2 text-sm text-red-700"
				>
					{error}
				</p>
			)}

			<div className="grid gap-5 sm:grid-cols-2">
				<fieldset className="space-y-2">
					<legend className="mb-2 font-semibold text-slate-900">
						Starting XV
					</legend>
					{STARTER_JERSEYS.map(slot)}
				</fieldset>
				<fieldset className="space-y-2">
					<legend className="mb-2 font-semibold text-slate-900">
						Replacements
					</legend>
					{BENCH_JERSEYS.map(slot)}
				</fieldset>
			</div>

			<button
				type="button"
				onClick={handleSave}
				disabled={saving}
				className="rounded bg-slate-900 px-4 py-2 font-semibold text-white disabled:opacity-50"
			>
				{saving ? "Saving…" : "Save lineup"}
			</button>
		</div>
	);
}
