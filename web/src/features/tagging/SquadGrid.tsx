import type {
	EventKind,
	LineupSlot,
	MatchState,
	Player,
} from "../../types/core";

type SquadGridProps = {
	starters: LineupSlot[];
	bench: LineupSlot[];
	players: Map<string, Player>;
	state: MatchState;
	armedKind: EventKind | null;
	armedPlayerId: string | null;
	onTapPlayer: (playerId: string) => void;
};

/**
 * The squad as numbered tiles, in fixed positions. Nothing reflows as the match
 * goes on: a coach's thumb learns where a player is and that has to keep being
 * true in the 78th minute.
 */
export function SquadGrid({
	starters,
	bench,
	players,
	state,
	armedKind,
	armedPlayerId,
	onTapPlayer,
}: SquadGridProps) {
	function tile(slot: LineupSlot) {
		const stats = state.players[slot.playerId];
		const player = players.get(slot.playerId);
		const off = !(stats?.onPitch ?? false);
		const armed = armedPlayerId === slot.playerId;
		const count = armedKind ? (stats?.counts?.[armedKind] ?? 0) : 0;

		return (
			<button
				key={slot.playerId}
				type="button"
				onClick={() => onTapPlayer(slot.playerId)}
				aria-pressed={armed}
				data-testid={`player-${slot.playerId}`}
				className={`flex h-20 flex-col items-center justify-center rounded-lg border-2 ${
					armed
						? "border-slate-900 bg-slate-200"
						: "border-transparent bg-white"
				} ${off ? "opacity-40" : ""}`}
			>
				<span className="text-2xl font-bold leading-none text-slate-900">
					{slot.jersey}
				</span>
				<span className="mt-1 max-w-full truncate px-1 text-xs text-slate-600">
					{player?.lastName ?? slot.playerId}
				</span>
				{/* The live count of whatever is armed, so a coach can see the tap
				    landed without leaving the screen they are on. */}
				<span className="text-xs font-semibold text-slate-500">
					{armedKind ? count : " "}
				</span>
			</button>
		);
	}

	return (
		<div className="space-y-3">
			<div className="grid grid-cols-5 gap-2">{starters.map(tile)}</div>
			{bench.length > 0 && (
				<div>
					<p className="mb-1 text-xs font-semibold uppercase tracking-wide text-slate-500">
						Bench
					</p>
					<div className="grid grid-cols-5 gap-2">{bench.map(tile)}</div>
				</div>
			)}
		</div>
	);
}
