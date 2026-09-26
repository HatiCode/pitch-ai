import type {
	CatalogueEntry,
	Event,
	MatchState,
	Player,
} from "../../types/core";
import { clockLabel } from "./TopStrip";

const SHOWN = 8;

type RecentEventsProps = {
	events: Event[];
	state: MatchState;
	catalogue: CatalogueEntry[];
	players: Map<string, Player>;
	onUndo: () => void;
};

/**
 * The last few taps, newest first, with one Undo.
 *
 * Voided events and the voids themselves are left out: a coach correcting a
 * mis-tap wants the list to show what the match now says, not a history of
 * their own corrections.
 */
export function RecentEvents({
	events,
	state,
	catalogue,
	players,
	onUndo,
}: RecentEventsProps) {
	const voided = new Set(state.voidedIds);
	const labels = new Map(catalogue.map((entry) => [entry.kind, entry.label]));

	const recent = events
		.filter((event) => event.kind !== "void" && !voided.has(event.id))
		.slice(-SHOWN)
		.reverse();

	return (
		<footer className="flex items-center gap-3 rounded-xl bg-white px-4 py-2">
			<button
				type="button"
				onClick={onUndo}
				className="rounded-lg bg-red-600 px-5 py-3 text-sm font-bold text-white"
			>
				Undo
			</button>
			<ul className="flex flex-1 gap-2 overflow-hidden">
				{recent.map((event) => (
					<li
						key={event.id}
						className="whitespace-nowrap rounded bg-slate-100 px-2 py-1 text-xs text-slate-700"
					>
						<span className="font-mono">{clockLabel(event.clockMs)}</span>{" "}
						{labels.get(event.kind) ?? event.kind}
						{event.playerId &&
							` · ${players.get(event.playerId)?.lastName ?? event.playerId}`}
					</li>
				))}
				{recent.length === 0 && (
					<li className="px-2 py-1 text-xs text-slate-400">
						Nothing tagged yet
					</li>
				)}
			</ul>
		</footer>
	);
}
