import type { SyncStatus } from "../../lib/sync";
import type {
	CatalogueEntry,
	MatchState,
	Possession,
	Zone,
} from "../../types/core";

const ZONES: { zone: Zone; label: string }[] = [
	{ zone: "our_22", label: "Our 22" },
	{ zone: "our_half", label: "Our half" },
	{ zone: "their_half", label: "Their half" },
	{ zone: "their_22", label: "Their 22" },
];

const POSSESSIONS: { possession: Possession; label: string }[] = [
	{ possession: "us", label: "Us" },
	{ possession: "them", label: "Them" },
];

const SYNC_LABELS: Record<SyncStatus["state"], string> = {
	offline: "Offline",
	pending: "Pending",
	syncing: "Syncing",
	synced: "Synced",
	error: "Sync error",
};

export function clockLabel(clockMs: number): string {
	const total = Math.max(Math.floor(clockMs / 1000), 0);
	const minutes = Math.floor(total / 60);
	const seconds = total % 60;
	return `${String(minutes).padStart(2, "0")}:${String(seconds).padStart(2, "0")}`;
}

type TopStripProps = {
	state: MatchState;
	clockMs: number;
	running: boolean;
	period: number;
	zone: Zone;
	possession: Possession;
	scoreKinds: CatalogueEntry[];
	sync: SyncStatus;
	onToggleClock: () => void;
	onEndPeriod: () => void;
	onEndMatch: () => void;
	onZone: (zone: Zone) => void;
	onPossession: (possession: Possession) => void;
	onScore: (entry: CatalogueEntry) => void;
};

export function TopStrip({
	state,
	clockMs,
	running,
	period,
	zone,
	possession,
	scoreKinds,
	sync,
	onToggleClock,
	onEndPeriod,
	onEndMatch,
	onZone,
	onPossession,
	onScore,
}: TopStripProps) {
	return (
		<header className="flex items-center gap-4 rounded-xl bg-slate-900 px-4 py-3 text-white">
			<div className="flex items-center gap-3">
				{/*
				 * A stopped clock has to be unmistakable at a glance. Every
				 * percentage downstream is denominated in running clock, so a coach
				 * who believes it is running when it is not corrupts the match
				 * quietly.
				 */}
				<div>
					<p
						data-testid="clock"
						className={`font-mono text-4xl font-bold tabular-nums ${
							running ? "text-white" : "text-amber-300"
						}`}
					>
						{clockLabel(clockMs)}
					</p>
					<p className="text-xs uppercase tracking-wide text-slate-300">
						Period {period} · {running ? "running" : "stopped"}
					</p>
				</div>
				{/*
				 * One tap, one fixed position, never behind a tab: a referee stops
				 * time at no notice and a control that takes two taps to reach will
				 * not be used.
				 */}
				<button
					type="button"
					onClick={onToggleClock}
					className={`h-16 w-24 rounded-lg text-base font-bold ${
						running
							? "bg-amber-400 text-slate-900"
							: "bg-emerald-400 text-slate-900"
					}`}
				>
					{running ? "Stop" : "Start"}
				</button>
			</div>

			<div className="flex items-center gap-2">
				<p className="font-mono text-3xl font-bold tabular-nums">
					{state.score.us}–{state.score.them}
				</p>
				<div className="grid grid-cols-4 gap-1">
					{scoreKinds.map((entry) => (
						<button
							key={entry.kind}
							type="button"
							onClick={() => onScore(entry)}
							className="rounded bg-slate-700 px-2 py-1 text-xs font-semibold hover:bg-slate-600"
						>
							{entry.label}
						</button>
					))}
				</div>
			</div>

			<div className="flex flex-1 items-center justify-end gap-3">
				<div className="flex overflow-hidden rounded-lg">
					{ZONES.map((option) => (
						<button
							key={option.zone}
							type="button"
							onClick={() => onZone(option.zone)}
							aria-pressed={zone === option.zone}
							className={`px-3 py-2 text-sm font-semibold ${
								zone === option.zone
									? "bg-white text-slate-900"
									: "bg-slate-700 text-slate-200"
							}`}
						>
							{option.label}
						</button>
					))}
				</div>
				<div className="flex overflow-hidden rounded-lg">
					{POSSESSIONS.map((option) => (
						<button
							key={option.possession}
							type="button"
							onClick={() => onPossession(option.possession)}
							aria-pressed={possession === option.possession}
							className={`px-3 py-2 text-sm font-semibold ${
								possession === option.possession
									? "bg-white text-slate-900"
									: "bg-slate-700 text-slate-200"
							}`}
						>
							{option.label}
						</button>
					))}
				</div>

				<div className="flex flex-col gap-1">
					<button
						type="button"
						onClick={onEndPeriod}
						className="rounded bg-slate-700 px-3 py-1 text-xs font-semibold hover:bg-slate-600"
					>
						End period
					</button>
					<button
						type="button"
						onClick={onEndMatch}
						className="rounded bg-slate-700 px-3 py-1 text-xs font-semibold hover:bg-slate-600"
					>
						End match
					</button>
				</div>

				<p
					data-testid="sync-status"
					className={`w-28 text-right text-xs font-semibold ${
						sync.state === "error" ? "text-red-300" : "text-slate-300"
					}`}
				>
					{SYNC_LABELS[sync.state]}
					{sync.pending > 0 && ` · ${sync.pending}`}
				</p>
			</div>
		</header>
	);
}
