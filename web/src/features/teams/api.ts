import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { useAuth } from "../../lib/auth";
import type { Team } from "../../types/core";

const TEAMS_KEY = ["teams"];

export function useTeams() {
	const { apiFetch } = useAuth();
	return useQuery({
		queryKey: TEAMS_KEY,
		queryFn: () => apiFetch<Team[]>("/teams"),
	});
}

export function useTeam(teamId: string | undefined) {
	const { apiFetch } = useAuth();
	return useQuery({
		queryKey: ["teams", teamId],
		queryFn: () => apiFetch<Team>(`/teams/${teamId}`),
		enabled: Boolean(teamId),
	});
}

export function useSaveTeam() {
	const { apiFetch } = useAuth();
	const queryClient = useQueryClient();

	return useMutation({
		mutationFn: (team: Team) =>
			team.id
				? apiFetch<Team>(`/teams/${team.id}`, {
						method: "PUT",
						body: JSON.stringify(team),
					})
				: apiFetch<Team>("/teams", {
						method: "POST",
						body: JSON.stringify(team),
					}),
		onSuccess: () => queryClient.invalidateQueries({ queryKey: TEAMS_KEY }),
	});
}

export function useDeleteTeam() {
	const { apiFetch } = useAuth();
	const queryClient = useQueryClient();

	return useMutation({
		mutationFn: (teamId: string) =>
			apiFetch<void>(`/teams/${teamId}`, { method: "DELETE" }),
		onSuccess: () => {
			queryClient.invalidateQueries({ queryKey: TEAMS_KEY });
			// Squad membership changes for every player who was in it.
			queryClient.invalidateQueries({ queryKey: ["players"] });
		},
	});
}
