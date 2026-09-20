import { useState } from "react";
import { Link, useNavigate, useParams } from "react-router";
import { ConfirmDialog } from "../../components/ui/ConfirmDialog";
import { useAuth } from "../../lib/auth";
import type { Player } from "../../types/core";
import { usePlayers, useSavePlayer } from "../players/api";
import { PlayerForm } from "../players/PlayerForm";
import { POSITION_LABELS } from "../players/positions";
import { useDeleteTeam, useTeam } from "./api";

export function SquadDetailPage() {
	const { teamId = "" } = useParams();
	const { membership } = useAuth();
	const team = useTeam(teamId);
	const players = usePlayers();
	const savePlayer = useSavePlayer();
	const deleteTeam = useDeleteTeam();
	const navigate = useNavigate();
	const [editing, setEditing] = useState<Player | null>(null);
	const [adding, setAdding] = useState(false);
	const [confirmingDelete, setConfirmingDelete] = useState(false);

	const canManage = membership?.role === "admin";

	if (team.isPending || players.isPending) {
		return <p className="text-slate-500">Loading squad…</p>;
	}
	if (team.error) {
		return (
			<p className="text-red-700">
				Could not load this squad: {(team.error as Error).message}
			</p>
		);
	}

	const all = players.data ?? [];
	const inSquad = all.filter((p) => p.teamIds.includes(teamId));
	const available = all.filter((p) => !p.teamIds.includes(teamId));

	function setMembership(player: Player, member: boolean) {
		const teamIds = member
			? [...player.teamIds, teamId]
			: player.teamIds.filter((id) => id !== teamId);
		savePlayer.mutate({ ...player, teamIds });
	}

	function handleSubmit(player: Player) {
		// A player created from inside a squad joins it straight away.
		const teamIds = player.teamIds.includes(teamId)
			? player.teamIds
			: [...player.teamIds, teamId];
		savePlayer.mutate(
			{ ...player, teamIds },
			{
				onSuccess: () => {
					setEditing(null);
					setAdding(false);
				},
			},
		);
	}

	return (
		<section className="space-y-6">
			<div>
				<Link
					to="/squads"
					className="text-sm text-slate-500 hover:text-slate-700"
				>
					← All squads
				</Link>
				<div className="mt-2 flex items-center justify-between">
					<h1 className="text-xl font-bold text-slate-900">
						{team.data?.name}
					</h1>
					{canManage && !adding && !editing && (
						<div className="flex gap-3">
							<button
								type="button"
								onClick={() => setAdding(true)}
								className="rounded bg-slate-900 px-4 py-2 text-sm font-semibold text-white"
							>
								Add player
							</button>
							<button
								type="button"
								onClick={() => setConfirmingDelete(true)}
								className="rounded border border-red-700 px-4 py-2 text-sm font-semibold text-red-700 hover:bg-red-50"
							>
								Delete squad
							</button>
						</div>
					)}
				</div>
				<p className="text-sm text-slate-500">
					{inSquad.length} {inSquad.length === 1 ? "player" : "players"}
				</p>
			</div>

			{(adding || editing) && (
				<PlayerForm
					player={editing ?? undefined}
					onSubmit={handleSubmit}
					onCancel={() => {
						setEditing(null);
						setAdding(false);
					}}
				/>
			)}

			{savePlayer.isError && (
				<p
					role="alert"
					className="rounded bg-red-50 px-3 py-2 text-sm text-red-700"
				>
					{(savePlayer.error as Error).message}
				</p>
			)}

			<ul className="divide-y divide-slate-200 rounded-xl bg-white shadow-sm">
				{inSquad.map((player) => (
					<li key={player.id} className="flex items-center gap-4 px-5 py-3">
						<div className="flex-1">
							<p className="font-medium text-slate-900">
								{player.firstName} {player.lastName}
							</p>
							<p className="text-sm text-slate-500">
								{player.positions.map((p) => POSITION_LABELS[p]).join(" · ")}
							</p>
						</div>
						{canManage && (
							<>
								<button
									type="button"
									onClick={() => setEditing(player)}
									className="text-sm text-slate-600 hover:text-slate-900"
								>
									Edit
								</button>
								<button
									type="button"
									onClick={() => setMembership(player, false)}
									className="text-sm text-red-700 hover:text-red-900"
								>
									Remove from squad
								</button>
							</>
						)}
					</li>
				))}
				{inSquad.length === 0 && (
					<li className="px-5 py-6 text-slate-500">
						Nobody in this squad yet.
					</li>
				)}
			</ul>

			{canManage && available.length > 0 && (
				<div className="space-y-2">
					<h2 className="text-sm font-semibold text-slate-700">
						Other players at the club
					</h2>
					<p className="text-sm text-slate-500">
						A player can be in several squads at once — adding them here does
						not remove them from anywhere else.
					</p>
					<ul className="divide-y divide-slate-200 rounded-xl bg-white shadow-sm">
						{available.map((player) => (
							<li key={player.id} className="flex items-center gap-4 px-5 py-3">
								<div className="flex-1">
									<p className="text-slate-900">
										{player.firstName} {player.lastName}
									</p>
									<p className="text-sm text-slate-500">
										{player.positions
											.map((p) => POSITION_LABELS[p])
											.join(" · ")}
									</p>
								</div>
								<button
									type="button"
									onClick={() => setMembership(player, true)}
									className="text-sm text-slate-700 hover:text-slate-900"
								>
									Add to squad
								</button>
							</li>
						))}
					</ul>
				</div>
			)}

			{confirmingDelete && (
				<ConfirmDialog
					title={`Delete “${team.data?.name}”?`}
					confirmLabel="Delete squad"
					busyLabel="Deleting…"
					busy={deleteTeam.isPending}
					onCancel={() => setConfirmingDelete(false)}
					onConfirm={() =>
						deleteTeam.mutate(teamId, { onSuccess: () => navigate("/squads") })
					}
				>
					<p>
						<strong>This cannot be undone.</strong> Deleting this squad
						permanently removes:
					</p>
					<ul className="list-disc space-y-1 pl-5">
						<li>the squad itself</li>
						<li>every fixture belonging to it, with their lineups</li>
						<li>all match events recorded for those fixtures</li>
						<li>all statistics derived from them</li>
					</ul>
					<p>
						<strong>Players are not deleted.</strong> All {inSquad.length}{" "}
						member
						{inSquad.length === 1 ? "" : "s"} stay on the club's books — they
						simply stop being in this squad, keeping their membership of any
						other squad.
					</p>
					{deleteTeam.isError && (
						<p
							role="alert"
							className="rounded bg-red-50 px-3 py-2 text-red-700"
						>
							{(deleteTeam.error as Error).message}
						</p>
					)}
				</ConfirmDialog>
			)}
		</section>
	);
}
