import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { useAuth } from "../../lib/auth";
import type { Lineup, Match } from "../../types/core";

const matchesKey = (teamId: string) => ["matches", teamId];

export function useMatches(teamId: string) {
	const { apiFetch } = useAuth();
	return useQuery({
		queryKey: matchesKey(teamId),
		queryFn: () => apiFetch<Match[]>(`/teams/${teamId}/matches`),
		enabled: Boolean(teamId),
	});
}

export function useCreateMatch(teamId: string) {
	const { apiFetch } = useAuth();
	const queryClient = useQueryClient();

	return useMutation({
		mutationFn: (match: Match) =>
			apiFetch<Match>(`/teams/${teamId}/matches`, {
				method: "POST",
				body: JSON.stringify(match),
			}),
		onSuccess: () =>
			queryClient.invalidateQueries({ queryKey: matchesKey(teamId) }),
	});
}

export function useSetLineup(teamId: string) {
	const { apiFetch } = useAuth();
	const queryClient = useQueryClient();

	return useMutation({
		mutationFn: ({ matchId, lineup }: { matchId: string; lineup: Lineup }) =>
			apiFetch<Match>(`/teams/${teamId}/matches/${matchId}/lineup`, {
				method: "PUT",
				body: JSON.stringify(lineup),
			}),
		onSuccess: () =>
			queryClient.invalidateQueries({ queryKey: matchesKey(teamId) }),
	});
}
