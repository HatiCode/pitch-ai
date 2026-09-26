import { useLiveQuery } from "dexie-react-hooks";
import { useEffect, useState } from "react";
import { useAuth } from "../../lib/auth";
import {
	cacheCatalogue,
	cachedCatalogue,
	cachedMatch,
	cachedPlayers,
	cacheMatch,
	cachePlayers,
	deviceId as readDeviceId,
} from "../../lib/db";
import type { CatalogueEntry, Match, Player } from "../../types/core";

// The tagging screen renders from IndexedDB and only from IndexedDB. These
// hooks fetch when there is signal and write what they get straight into the
// cache; the components never see the network.
//
// A failed fetch is not an error here. Arriving at a pitch with no signal and
// opening a match tagged from the car park is the case this whole milestone
// exists for, so a cached copy rendering after a failed request is success.

export function useCatalogue(): CatalogueEntry[] {
	const { apiFetch } = useAuth();
	const cached = useLiveQuery(() => cachedCatalogue(), [], []);

	useEffect(() => {
		let cancelled = false;
		void (async () => {
			try {
				const entries = await apiFetch<CatalogueEntry[]>("/catalogue");
				if (!cancelled) {
					await cacheCatalogue(entries);
				}
			} catch {
				// Offline. Whatever was cached last time is what a coach tags with.
			}
		})();
		return () => {
			cancelled = true;
		};
	}, [apiFetch]);

	return cached;
}

export type PrimedMatch = {
	match: Match | null | undefined;
	players: Player[];
	deviceId: string;
	/** True until the cache lookup has answered one way or the other. */
	loading: boolean;
};

/**
 * Loads everything a match needs to be tagged, from the cache first.
 *
 * Priming happens whenever the screen is opened with signal, so the last
 * fixture a coach looked at indoors is the one they can tag on the pitch.
 */
export function usePrimedMatch(teamId: string, matchId: string): PrimedMatch {
	const { apiFetch } = useAuth();
	// null once the lookup has run and found nothing, so the page can tell
	// "still loading" from "never cached, and there is no signal to fetch it".
	const match = useLiveQuery(
		async () => (await cachedMatch(matchId)) ?? null,
		[matchId],
	);
	const players = useLiveQuery(() => cachedPlayers(), [], []);
	// Not a live query: deviceId() mints an id on first use, and a write is
	// refused inside the read-only transaction a live query runs in. It never
	// changes once minted, so reading it once is the whole requirement.
	const [device, setDevice] = useState("");
	useEffect(() => {
		void readDeviceId().then(setDevice);
	}, []);

	useEffect(() => {
		if (!teamId || !matchId) {
			return;
		}
		void (async () => {
			try {
				const [fresh, squad] = await Promise.all([
					apiFetch<Match>(`/teams/${teamId}/matches/${matchId}`),
					apiFetch<Player[]>("/players"),
				]);
				await Promise.all([cacheMatch(fresh), cachePlayers(squad)]);
			} catch {
				// Offline: the cached copy below is the whole point.
			}
		})();
	}, [apiFetch, teamId, matchId]);

	return {
		match,
		players,
		deviceId: device,
		loading: match === undefined,
	};
}
