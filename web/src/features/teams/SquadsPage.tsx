import { type FormEvent, useState } from "react";
import { Link } from "react-router";
import { useAuth } from "../../lib/auth";
import { useSaveTeam, useTeams } from "./api";

function NewSquadForm({ onDone }: { onDone: () => void }) {
	const saveTeam = useSaveTeam();
	const [name, setName] = useState("");
	const [shortName, setShortName] = useState("");
	const [error, setError] = useState<string | null>(null);

	function handleSubmit(event: FormEvent) {
		event.preventDefault();
		if (!name.trim()) {
			setError("A squad needs a name.");
			return;
		}
		setError(null);
		saveTeam.mutate(
			{
				id: "",
				clubId: "",
				name: name.trim(),
				shortName: shortName.trim(),
				active: true,
			},
			{ onSuccess: onDone },
		);
	}

	return (
		<form
			onSubmit={handleSubmit}
			className="space-y-4 rounded-xl bg-white p-5 shadow-sm"
		>
			{(error || saveTeam.isError) && (
				<p
					role="alert"
					className="rounded bg-red-50 px-3 py-2 text-sm text-red-700"
				>
					{error ?? (saveTeam.error as Error).message}
				</p>
			)}
			<div className="grid gap-4 sm:grid-cols-2">
				<label className="block text-sm">
					<span className="mb-1 block font-medium">Squad name</span>
					<input
						value={name}
						onChange={(e) => setName(e.target.value)}
						placeholder="1st XV"
						className="w-full rounded border border-slate-300 px-3 py-2"
					/>
				</label>
				<label className="block text-sm">
					<span className="mb-1 block font-medium">Short name (optional)</span>
					<input
						value={shortName}
						onChange={(e) => setShortName(e.target.value)}
						placeholder="1XV"
						maxLength={8}
						className="w-full rounded border border-slate-300 px-3 py-2"
					/>
				</label>
			</div>
			<div className="flex gap-3">
				<button
					type="submit"
					className="rounded bg-slate-900 px-4 py-2 font-semibold text-white"
				>
					Create squad
				</button>
				<button
					type="button"
					onClick={onDone}
					className="rounded px-4 py-2 text-slate-600"
				>
					Cancel
				</button>
			</div>
		</form>
	);
}

export function SquadsPage() {
	const { membership } = useAuth();
	const { data: teams, isPending, error } = useTeams();
	const [adding, setAdding] = useState(false);

	const canManage = membership?.role === "admin";

	if (isPending) {
		return <p className="text-slate-500">Loading squads…</p>;
	}
	if (error) {
		return (
			<p className="text-red-700">
				Could not load squads: {(error as Error).message}
			</p>
		);
	}

	return (
		<section className="space-y-5">
			<div className="flex items-center justify-between">
				<h1 className="text-xl font-bold text-slate-900">Squads</h1>
				{canManage && !adding && (
					<button
						type="button"
						onClick={() => setAdding(true)}
						className="rounded bg-slate-900 px-4 py-2 text-sm font-semibold text-white"
					>
						New squad
					</button>
				)}
			</div>

			{adding && <NewSquadForm onDone={() => setAdding(false)} />}

			<ul className="divide-y divide-slate-200 rounded-xl bg-white shadow-sm">
				{teams?.map((team) => (
					<li key={team.id}>
						<Link
							to={`/squads/${team.id}`}
							className="flex items-center gap-4 px-5 py-4 hover:bg-slate-50"
						>
							<div className="flex-1">
								<p className="font-medium text-slate-900">{team.name}</p>
								{team.shortName && (
									<p className="text-sm text-slate-500">{team.shortName}</p>
								)}
							</div>
							<span aria-hidden="true" className="text-slate-400">
								→
							</span>
						</Link>
					</li>
				))}
				{teams?.length === 0 && (
					<li className="px-5 py-6 text-slate-500">
						No squads yet.
						{canManage ? " Create one to start adding players." : ""}
					</li>
				)}
			</ul>
		</section>
	);
}
