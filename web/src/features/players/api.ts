import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { useAuth } from "../../lib/auth";
import type { Player } from "../../types/core";

const PLAYERS_KEY = ["players"];

export function usePlayers() {
	const { apiFetch } = useAuth();
	return useQuery({
		queryKey: PLAYERS_KEY,
		queryFn: () => apiFetch<Player[]>("/players"),
	});
}

export function useSavePlayer() {
	const { apiFetch } = useAuth();
	const queryClient = useQueryClient();

	return useMutation({
		mutationFn: (player: Player) =>
			player.id
				? apiFetch<Player>(`/players/${player.id}`, {
						method: "PUT",
						body: JSON.stringify(player),
					})
				: apiFetch<Player>("/players", {
						method: "POST",
						body: JSON.stringify(player),
					}),
		onSuccess: () => queryClient.invalidateQueries({ queryKey: PLAYERS_KEY }),
	});
}

export function useDeletePlayer() {
	const { apiFetch } = useAuth();
	const queryClient = useQueryClient();

	return useMutation({
		mutationFn: (playerID: string) =>
			apiFetch<void>(`/players/${playerID}`, { method: "DELETE" }),
		onSuccess: () => queryClient.invalidateQueries({ queryKey: PLAYERS_KEY }),
	});
}
