import type { Position, PositionGroup } from "../../types/core";

// Record<Position, string> makes a missing entry a TypeScript error: the union
// is generated from the Go constants, so adding a position there breaks the
// build here until it is labelled.
export const POSITION_LABELS: Record<Position, string> = {
	loosehead: "Loosehead prop (1)",
	hooker: "Hooker (2)",
	tighthead: "Tighthead prop (3)",
	lock: "Lock (4/5)",
	blindside: "Blindside flanker (6)",
	openside: "Openside flanker (7)",
	number_eight: "Number 8",
	scrum_half: "Scrum-half (9)",
	fly_half: "Fly-half (10)",
	wing: "Wing (11/14)",
	inside_centre: "Inside centre (12)",
	outside_centre: "Outside centre (13)",
	fullback: "Fullback (15)",
};

export const POSITION_GROUP_LABELS: Record<PositionGroup, string> = {
	front_row: "Front row",
	second_row: "Second row",
	back_row: "Back row",
	half_backs: "Half backs",
	centres: "Centres",
	back_three: "Back three",
};
