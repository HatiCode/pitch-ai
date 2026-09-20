import { useState } from "react";
import { Link, useParams } from "react-router";
import { useAuth } from "../../lib/auth";
import type { Match } from "../../types/core";
import { usePlayers } from "../players/api";
import { useTeam } from "../teams/api";
import { useCreateMatch, useMatches, useSetLineup } from "./api";
import { LineupPicker } from "./LineupPicker";
import { MatchForm } from "./MatchForm";

function kickoffLabel(iso: string) {
	return new Date(iso).toLocaleString(undefined, {
		weekday: "short",
		day: "numeric",
		month: "short",
		hour: "2-digit",
		minute: "2-digit",
	});
}

export function MatchesPage() {
	const { teamId = "" } = useParams();
	const { membership } = useAuth();
	const team = useTeam(teamId);
	const matches = useMatches(teamId);
	const players = usePlayers();
	const createMatch = useCreateMatch(teamId);
	const setLineup = useSetLineup(teamId);

	const [adding, setAdding] = useState(false);
	const [selectingFor, setSelectingFor] = useState<string | null>(null);

	// Every role except a pure reader holds ActionEditMatch; the server decides.
	const canEdit = membership !== null;

	if (matches.isPending || players.isPending) {
		return <p className="text-slate-500">Loading fixtures…</p>;
	}
	if (matches.error) {
		return (
			<p className="text-red-700">
				Could not load fixtures: {(matches.error as Error).message}
			</p>
		);
	}

	const openMatch = matches.data?.find((m) => m.id === selectingFor);
	const squad = (players.data ?? []).filter(
		(p) => p.status === "active" && p.teamIds.includes(teamId),
	);

	function lineupSummary(match: Match) {
		const picked = match.lineup?.starters?.length ?? 0;
		if (picked === 15) {
			const bench = match.lineup?.bench?.length ?? 0;
			return bench > 0 ? `XV selected, ${bench} on the bench` : "XV selected";
		}
		return picked === 0 ? "no lineup yet" : `${picked} of 15 selected`;
	}

	return (
		<section className="space-y-5">
			<div>
				<Link
					to={`/squads/${teamId}`}
					className="text-sm text-slate-500 hover:text-slate-700"
				>
					← {team.data?.name ?? "Squad"}
				</Link>
				<div className="mt-2 flex items-center justify-between">
					<h1 className="text-xl font-bold text-slate-900">Fixtures</h1>
					{canEdit && !adding && (
						<button
							type="button"
							onClick={() => setAdding(true)}
							className="rounded bg-slate-900 px-4 py-2 text-sm font-semibold text-white"
						>
							Add fixture
						</button>
					)}
				</div>
			</div>

			{adding && (
				<MatchForm
					saving={createMatch.isPending}
					onSubmit={(match) =>
						createMatch.mutate(match, { onSuccess: () => setAdding(false) })
					}
					onCancel={() => setAdding(false)}
				/>
			)}

			{createMatch.isError && (
				<p
					role="alert"
					className="rounded bg-red-50 px-3 py-2 text-sm text-red-700"
				>
					{(createMatch.error as Error).message}
				</p>
			)}

			<ul className="divide-y divide-slate-200 rounded-xl bg-white shadow-sm">
				{matches.data?.map((match) => (
					<li key={match.id} className="flex items-center gap-4 px-5 py-3">
						<div className="flex-1">
							<p className="font-medium text-slate-900">
								{match.venue === "home" ? "vs" : "at"} {match.opponent}
							</p>
							<p className="text-sm text-slate-500">
								{kickoffLabel(match.kickoffAt)}
								{match.competition && ` · ${match.competition}`} ·{" "}
								{lineupSummary(match)}
							</p>
						</div>
						{canEdit && (
							<button
								type="button"
								onClick={() =>
									setSelectingFor(
										match.id === selectingFor ? null : (match.id ?? null),
									)
								}
								className="text-sm text-slate-600 hover:text-slate-900"
							>
								{match.id === selectingFor ? "Close" : "Select XV"}
							</button>
						)}
					</li>
				))}
				{matches.data?.length === 0 && (
					<li className="px-5 py-6 text-slate-500">
						No fixtures yet.{canEdit ? " Add one to start picking a side." : ""}
					</li>
				)}
			</ul>

			{openMatch && (
				<div className="space-y-3 rounded-xl bg-white p-5 shadow-sm">
					<h2 className="font-semibold text-slate-900">
						Lineup — {openMatch.opponent}
					</h2>
					{squad.length < 15 ? (
						<p className="text-sm text-slate-600">
							This squad has {squad.length} active{" "}
							{squad.length === 1 ? "player" : "players"}. A starting XV needs
							15, so add more on the{" "}
							<Link to={`/squads/${teamId}`} className="underline">
								squad page
							</Link>{" "}
							first.
						</p>
					) : (
						<LineupPicker
							players={squad}
							lineup={openMatch.lineup}
							saving={setLineup.isPending}
							onSave={(lineup) =>
								setLineup.mutate(
									{ matchId: openMatch.id ?? "", lineup },
									{ onSuccess: () => setSelectingFor(null) },
								)
							}
						/>
					)}
					{setLineup.isError && (
						<p
							role="alert"
							className="rounded bg-red-50 px-3 py-2 text-sm text-red-700"
						>
							{(setLineup.error as Error).message}
						</p>
					)}
				</div>
			)}
		</section>
	);
}
