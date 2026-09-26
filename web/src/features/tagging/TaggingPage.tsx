import { useEffect, useMemo, useState } from "react";
import { Link, useParams } from "react-router";
import { useAuth } from "../../lib/auth";
import { createSync, type SyncStatus } from "../../lib/sync";
import type { Match, Player } from "../../types/core";
import { ActionPane } from "./ActionPane";
import { useCatalogue, usePrimedMatch } from "./api";
import { RecentEvents } from "./RecentEvents";
import { SquadGrid } from "./SquadGrid";
import { TopStrip } from "./TopStrip";
import { useTagging } from "./useTagging";

// Stands in for the real match while the cache is still answering, so the
// hooks below keep their order. Nothing is rendered from it.
const NO_MATCH: Match = {
	seasonId: "",
	opponent: "",
	kickoffAt: new Date(0).toISOString(),
	competition: "",
	venue: "home",
	status: "scheduled",
	lineup: { starters: [], bench: [] },
};

const IDLE: SyncStatus = { state: "pending", pending: 0, error: null };

export function TaggingPage() {
	const { teamId = "", matchId = "" } = useParams();
	const { apiFetch } = useAuth();
	const catalogue = useCatalogue();
	const { match, players, deviceId, loading } = usePrimedMatch(teamId, matchId);

	const sync = useMemo(
		() => createSync({ apiFetch, teamId, matchId }),
		[apiFetch, teamId, matchId],
	);
	const [status, setStatus] = useState<SyncStatus>(IDLE);

	useEffect(() => {
		const unsubscribe = sync.subscribe(setStatus);
		sync.start();
		return () => {
			unsubscribe();
			sync.stop();
		};
	}, [sync]);

	const tagging = useTagging({
		matchId,
		match: match ?? NO_MATCH,
		catalogue,
		deviceId,
		// Draining immediately rather than waiting for the next cycle: with
		// signal, a tap is on the server before the coach looks up.
		onAppended: () => void sync.push(),
	});

	const byId = useMemo(
		() => new Map<string, Player>(players.map((p) => [p.id ?? "", p])),
		[players],
	);
	const scoreKinds = useMemo(
		() => catalogue.filter((entry) => entry.group === "score"),
		[catalogue],
	);

	if (loading) {
		return <p className="text-slate-500">Loading match…</p>;
	}
	if (!match) {
		return (
			<p className="text-slate-600">
				This match has not been opened on this device before, and there is no
				connection to fetch it.{" "}
				<Link to={`/squads/${teamId}/matches`} className="underline">
					Back to fixtures
				</Link>
			</p>
		);
	}

	const starters = [...match.lineup.starters].sort(
		(a, b) => a.jersey - b.jersey,
	);
	const bench = [...match.lineup.bench].sort((a, b) => a.jersey - b.jersey);

	return (
		<section className="flex h-[calc(100vh-7rem)] flex-col gap-3">
			<TopStrip
				state={tagging.state}
				clockMs={tagging.clockMs}
				running={tagging.running}
				period={tagging.period}
				zone={tagging.zone}
				possession={tagging.possession}
				scoreKinds={scoreKinds}
				sync={status}
				onToggleClock={tagging.toggleClock}
				onEndPeriod={tagging.endPeriod}
				onEndMatch={tagging.endMatch}
				onZone={tagging.setZone}
				onPossession={tagging.setPossession}
				onScore={(entry) => tagging.tapAction(entry.kind)}
			/>

			{status.error && (
				<p
					role="alert"
					className="rounded bg-red-50 px-3 py-2 text-sm text-red-700"
				>
					{status.error}
				</p>
			)}

			<div className="flex min-h-0 flex-1 gap-3">
				<div className="flex-1 overflow-y-auto">
					<SquadGrid
						starters={starters}
						bench={bench}
						players={byId}
						state={tagging.state}
						armedKind={tagging.armed.kind}
						armedPlayerId={tagging.armed.playerId}
						onTapPlayer={tagging.tapPlayer}
					/>
				</div>
				<div className="w-[22rem]">
					<ActionPane
						catalogue={catalogue}
						armedKind={tagging.armed.kind}
						onTapAction={tagging.tapAction}
					/>
				</div>
			</div>

			<RecentEvents
				events={tagging.events}
				state={tagging.state}
				catalogue={catalogue}
				players={byId}
				onUndo={tagging.undo}
			/>
		</section>
	);
}
