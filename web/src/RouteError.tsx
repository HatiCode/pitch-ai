import { isRouteErrorResponse, Link, useRouteError } from "react-router";

export function RouteError() {
	const error = useRouteError();

	const title = isRouteErrorResponse(error)
		? `${error.status} ${error.statusText}`
		: "Something went wrong";
	const detail = isRouteErrorResponse(error)
		? "That page does not exist."
		: error instanceof Error
			? error.message
			: "An unexpected error occurred.";

	return (
		<main className="grid min-h-dvh place-items-center bg-slate-50 px-6">
			<div className="max-w-md space-y-3 text-center">
				<h1 className="text-2xl font-bold text-slate-900">{title}</h1>
				<p className="text-slate-500">{detail}</p>
				<Link
					to="/squad"
					className="inline-block text-sm text-slate-700 underline"
				>
					Back to the squad
				</Link>
			</div>
		</main>
	);
}
