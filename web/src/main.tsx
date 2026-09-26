import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { StrictMode } from "react";
import { createRoot } from "react-dom/client";
import { createBrowserRouter, Navigate, RouterProvider } from "react-router";
import { AppShell } from "./AppShell";
import { MatchesPage } from "./features/matches/MatchesPage";
import { SquadDetailPage } from "./features/teams/SquadDetailPage";
import { SquadsPage } from "./features/teams/SquadsPage";
import { RouteError } from "./RouteError";
import "./index.css";
import { AuthProvider } from "./lib/auth";
import { registerServiceWorker } from "./lib/pwa";

registerServiceWorker();

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
			{ path: "squads/:teamId/matches", element: <MatchesPage /> },
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
