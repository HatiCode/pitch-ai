import { NavLink, Outlet } from "react-router";
import { useAuth } from "./lib/auth";

const linkClass = ({ isActive }: { isActive: boolean }) =>
	isActive
		? "font-semibold text-slate-900"
		: "text-slate-500 hover:text-slate-700";

export function AppShell() {
	const { user, membership, loading, error, signIn, signOutUser } = useAuth();

	if (loading) {
		return <p className="p-8 text-slate-500">Loading…</p>;
	}

	if (!user) {
		return (
			<main className="grid min-h-dvh place-items-center bg-slate-50 px-6">
				<div className="space-y-5 text-center">
					<h1 className="text-3xl font-bold tracking-tight text-slate-900">
						pitch-ai
					</h1>
					<p className="text-slate-500">Rugby performance tracking</p>
					<button
						type="button"
						onClick={signIn}
						className="rounded-lg bg-slate-900 px-5 py-3 font-semibold text-white hover:bg-slate-800"
					>
						Sign in with Google
					</button>
				</div>
			</main>
		);
	}

	if (!membership) {
		return (
			<main className="grid min-h-dvh place-items-center bg-slate-50 px-6">
				<div className="max-w-md space-y-4 text-center">
					<p className="text-red-700">
						{error ?? "This account has no club membership."}
					</p>
					<p className="text-sm text-slate-500">
						Signed in as {user.email}. Ask an administrator to add you to a
						club.
					</p>
					<button
						type="button"
						onClick={signOutUser}
						className="text-sm text-slate-600 underline"
					>
						Sign out
					</button>
				</div>
			</main>
		);
	}

	return (
		<div className="min-h-dvh bg-slate-50">
			<header className="flex items-center gap-6 border-b border-slate-200 bg-white px-6 py-3">
				<span className="font-bold text-slate-900">pitch-ai</span>
				{/* Fixtures live under a squad, so there is no top-level Matches link. */}
				<nav className="flex gap-4 text-sm">
					<NavLink to="/squads" className={linkClass}>
						Squads
					</NavLink>
				</nav>
				<button
					type="button"
					onClick={signOutUser}
					className="ml-auto text-sm text-slate-500 hover:text-slate-700"
				>
					Sign out
				</button>
			</header>
			<main className="mx-auto max-w-5xl p-6">
				<Outlet />
			</main>
		</div>
	);
}
