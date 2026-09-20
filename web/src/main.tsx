import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { StrictMode } from "react";
import { createRoot } from "react-dom/client";
import { createBrowserRouter, Navigate, RouterProvider } from "react-router";
import { AppShell } from "./AppShell";
import { SquadDetailPage } from "./features/teams/SquadDetailPage";
import { SquadsPage } from "./features/teams/SquadsPage";
import { RouteError } from "./RouteError";
import "./index.css";
import { AuthProvider } from "./lib/auth";

const queryClient = new QueryClient();

const router = createBrowserRouter([
	{
		path: "/",
		element: <AppShell />,
		errorElement: <RouteError />,
		children: [
			{ index: true, element: <Navigate to="/squads" replace /> },
			{ path: "squads", element: <SquadsPage /> },
			{ path: "squads/:teamId", element: <SquadDetailPage /> },
			{
				path: "squads/:teamId/matches",
				// Task 15 replaces this with the real fixtures page.
				element: (
					<p className="text-slate-500">
						Fixtures arrive in the next milestone.
					</p>
				),
			},
		],
	},
]);

const rootElement = document.getElementById("root");
if (!rootElement) {
	throw new Error("index.html is missing the #root element");
}

createRoot(rootElement).render(
	<StrictMode>
		<QueryClientProvider client={queryClient}>
			<AuthProvider>
				<RouterProvider router={router} />
			</AuthProvider>
		</QueryClientProvider>
	</StrictMode>,
);
