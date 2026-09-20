import { type FormEvent, useState } from "react";
import type { Match, Venue } from "../../types/core";

// Record<Venue, string> is exhaustive: the union is generated from the Go
// constants, so a new venue breaks this build until it is labelled.
const VENUE_LABELS: Record<Venue, string> = {
	home: "Home",
	away: "Away",
	neutral: "Neutral",
};

type Props = {
	onSubmit: (match: Match) => void;
	onCancel: () => void;
	saving?: boolean;
};

export function MatchForm({ onSubmit, onCancel, saving = false }: Props) {
	const [opponent, setOpponent] = useState("");
	const [kickoff, setKickoff] = useState("");
	const [competition, setCompetition] = useState("");
	const [seasonId, setSeasonId] = useState("");
	const [venue, setVenue] = useState<Venue>("home");
	const [error, setError] = useState<string | null>(null);

	function handleSubmit(event: FormEvent) {
		event.preventDefault();

		if (!opponent.trim() || !kickoff || !seasonId.trim()) {
			setError("Opponent, kick-off and season are required.");
			return;
		}

		setError(null);
		onSubmit({
			id: "",
			clubId: "",
			teamId: "",
			seasonId: seasonId.trim(),
			opponent: opponent.trim(),
			// datetime-local gives local wall time; the server stores an instant.
			kickoffAt: new Date(kickoff).toISOString(),
			competition: competition.trim(),
			venue,
			status: "scheduled",
			lineup: { starters: [], bench: [] },
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
					<span className="mb-1 block font-medium">Opponent</span>
					<input
						value={opponent}
						onChange={(e) => setOpponent(e.target.value)}
						className="w-full rounded border border-slate-300 px-3 py-2"
					/>
				</label>
				<label className="block text-sm">
					<span className="mb-1 block font-medium">Kick-off</span>
					<input
						type="datetime-local"
						value={kickoff}
						onChange={(e) => setKickoff(e.target.value)}
						className="w-full rounded border border-slate-300 px-3 py-2"
					/>
				</label>
				<label className="block text-sm">
					<span className="mb-1 block font-medium">Competition</span>
					<input
						value={competition}
						onChange={(e) => setCompetition(e.target.value)}
						placeholder="Regional 1"
						className="w-full rounded border border-slate-300 px-3 py-2"
					/>
				</label>
				<label className="block text-sm">
					<span className="mb-1 block font-medium">Season</span>
					<input
						value={seasonId}
						onChange={(e) => setSeasonId(e.target.value)}
						placeholder="2026-27"
						className="w-full rounded border border-slate-300 px-3 py-2"
					/>
				</label>
				<label className="block text-sm">
					<span className="mb-1 block font-medium">Venue</span>
					<select
						value={venue}
						onChange={(e) => setVenue(e.target.value as Venue)}
						className="w-full rounded border border-slate-300 px-3 py-2"
					>
						{Object.entries(VENUE_LABELS).map(([value, label]) => (
							<option key={value} value={value}>
								{label}
							</option>
						))}
					</select>
				</label>
			</div>

			<div className="flex gap-3">
				<button
					type="submit"
					disabled={saving}
					className="rounded bg-slate-900 px-4 py-2 font-semibold text-white disabled:opacity-50"
				>
					{saving ? "Saving…" : "Save"}
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
