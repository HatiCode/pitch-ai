import { useLiveQuery } from "dexie-react-hooks";
import { useCallback, useEffect, useMemo, useRef, useState } from "react";
import { appendEvent, localEvents } from "../../lib/db";
import { fold } from "../../lib/fold";
import { uuidv7 } from "../../lib/uuid";
import type {
	CatalogueEntry,
	Event,
	EventKind,
	Match,
	MatchState,
	Possession,
	Zone,
} from "../../types/core";

/**
 * Two taps in either order, because on a sideline the coach's eyes are on the
 * match. An action stays armed after it fires so repeated taps on players log
 * repeated events; a player does not, because the next action is usually
 * different.
 */
export type Armed = { kind: EventKind | null; playerId: string | null };

export type Tagging = {
	state: MatchState;
	armed: Armed;
	clockMs: number;
	running: boolean;
	period: number;
	zone: Zone;
	possession: Possession;
	events: Event[];
	tapAction(kind: EventKind): void;
	tapPlayer(playerId: string): void;
	undo(): void;
	toggleClock(): void;
	endPeriod(): void;
	endMatch(): void;
	setZone(zone: Zone): void;
	setPossession(possession: Possession): void;
};

export type TaggingOptions = {
	matchId: string;
	match: Match;
	catalogue: CatalogueEntry[];
	deviceId: string;
	/** Called after every tap, so the outbox can drain without waiting. */
	onAppended?: () => void;
	/**
	 * The wall clock the match clock is measured against. Injectable so a test
	 * can move time without simulating forty minutes of it.
	 */
	now?: () => number;
};

// The clock stops itself at the end of each half. A referee's clock does, it
// removes a tap at the busiest moment, and it is what keeps clockMs monotonic
// when a half plays on past 40:00.
const HALF_BOUNDARIES_MS = [40 * 60 * 1000, 80 * 60 * 1000];

// The kinds that move the clock. The anchor is whichever of these is last in
// the merged log, not whichever this device happened to tap.
const CLOCK_KINDS = new Set<EventKind>([
	"period_started",
	"clock_paused",
	"clock_resumed",
	"period_ended",
	"match_ended",
]);

const RUNS_CLOCK = new Set<EventKind>(["period_started", "clock_resumed"]);

export function useTagging({
	matchId,
	match,
	catalogue,
	deviceId,
	onAppended,
	now = Date.now,
}: TaggingOptions): Tagging {
	const events = useLiveQuery(() => localEvents(matchId), [matchId], []);
	const [armed, setArmed] = useState<Armed>({ kind: null, playerId: null });

	const state = useMemo(() => fold(match, events), [match, events]);
	const anchor = useMemo(() => clockAnchor(events), [events]);

	// wallAt is when the anchoring event was observed, in this device's own
	// time. It never leaves the device: clockMs on each event is the only
	// timing that crosses the wire.
	const [wallAt, setWallAt] = useState(() => now());
	const [, setTick] = useState(0);

	// The wall time of a clock event this device tapped, captured at the tap.
	// The write takes a moment to come back through the live query, and
	// anchoring to its arrival instead would lose that moment from the clock
	// on every start and resume.
	const localAnchor = useRef<{ id: string; wallAt: number } | null>(null);

	// Re-anchor whenever the last clock event changes — including one that
	// arrived from the other coach's iPad on the last pull, which is what makes
	// their pause stop this device's clock too. A remote event can only be
	// anchored to when it was seen; its clockMs is what makes that harmless.
	const anchorId = anchor?.id ?? "";
	useEffect(() => {
		setWallAt(
			localAnchor.current?.id === anchorId ? localAnchor.current.wallAt : now(),
		);
	}, [anchorId, now]);

	const anchorClockMs = anchor?.clockMs ?? 0;
	const running = anchor ? RUNS_CLOCK.has(anchor.kind) : false;

	const stampClock = useCallback((): number => {
		if (!running) {
			return anchorClockMs;
		}
		return anchorClockMs + Math.max(now() - wallAt, 0);
	}, [anchorClockMs, running, wallAt, now]);

	// One tick a second, for the display only.
	useEffect(() => {
		if (!running) {
			return;
		}
		const timer = setInterval(() => setTick((n) => n + 1), 1000);
		return () => clearInterval(timer);
	}, [running]);

	const buildEvent = useCallback(
		(kind: EventKind, extra: Partial<Event> = {}): Event => ({
			id: uuidv7(),
			kind,
			clockMs: stampClock(),
			period: state.period === 0 ? 1 : state.period,
			deviceId,
			...extra,
		}),
		[deviceId, state.period, stampClock],
	);

	const writeEvent = useCallback(
		async (event: Event): Promise<void> => {
			if (CLOCK_KINDS.has(event.kind)) {
				localAnchor.current = { id: event.id, wallAt: now() };
			}
			await appendEvent(matchId, event);
			onAppended?.();
		},
		[matchId, onAppended, now],
	);

	const append = useCallback(
		async (kind: EventKind, extra: Partial<Event> = {}): Promise<Event> => {
			const event = buildEvent(kind, extra);
			await writeEvent(event);
			return event;
		},
		[buildEvent, writeEvent],
	);

	// Stop at 40:00 and 80:00, once each. A pause already at or past the
	// boundary is how the guard knows it has fired, which survives a reload and
	// lets the coach resume for the overrun. The ref covers the gap between
	// appending and the write coming back through the live query, where the log
	// still says no pause has happened.
	const stopped = useRef(new Set<number>());
	useEffect(() => {
		if (!running) {
			return;
		}
		const boundary = HALF_BOUNDARIES_MS.find(
			(edge) =>
				stampClock() >= edge &&
				!stopped.current.has(edge) &&
				!pausedAtOrAfter(events, edge),
		);
		if (boundary === undefined) {
			return;
		}
		stopped.current.add(boundary);
		void append("clock_paused", { clockMs: boundary });
	});

	// A toggle this device has written but not yet read back. State rather than
	// a ref, because the next tap has to see it: a ref would not re-render and
	// the handler would close over the stale pair again. It is dropped as soon
	// as the log carries it, so a later change from the other iPad wins the way
	// the fold says it should.
	const [pendingToggle, setPendingToggle] = useState<{
		id: string;
		zone: Zone;
		possession: Possession;
	} | null>(null);

	const applied =
		pendingToggle !== null && events.some((e) => e.id === pendingToggle.id);
	const toggles =
		pendingToggle && !applied
			? { zone: pendingToggle.zone, possession: pendingToggle.possession }
			: { zone: state.zone, possession: state.possession };

	useEffect(() => {
		if (applied) {
			setPendingToggle(null);
		}
	}, [applied]);

	const playerKinds = useMemo(() => playerAttributed(catalogue), [catalogue]);

	const onPitch = useCallback(
		(playerId: string) => state.players[playerId]?.onPitch ?? false,
		[state.players],
	);

	const tapAction = useCallback(
		(kind: EventKind) => {
			if (!playerKinds.has(kind)) {
				if (kind === "sub") {
					// A substitution needs two players, so it arms rather than fires.
					setArmed({ kind: "sub", playerId: null });
					return;
				}
				void append(kind);
				return;
			}

			if (armed.playerId) {
				void append(kind, { playerId: armed.playerId });
				setArmed({ kind, playerId: null });
				return;
			}
			setArmed({ kind: armed.kind === kind ? null : kind, playerId: null });
		},
		[append, armed, playerKinds],
	);

	/**
	 * The second half of a substitution: the player coming off is armed first,
	 * then the one coming on. The pair is collected before anything is written
	 * because a sub carrying only one of them would be refused by the server.
	 */
	const substitute = useCallback(
		(playerId: string) => {
			if (!armed.playerId) {
				// Whoever is coming off has to be on the pitch to come off it.
				if (onPitch(playerId)) {
					setArmed({ kind: "sub", playerId });
				}
				return;
			}
			if (playerId === armed.playerId) {
				return;
			}
			void append("sub", {
				payload: { offPlayerId: armed.playerId, onPlayerId: playerId },
			});
			setArmed({ kind: null, playerId: null });
		},
		[append, armed.playerId, onPitch],
	);

	const tapPlayer = useCallback(
		(playerId: string) => {
			if (armed.kind === "sub") {
				substitute(playerId);
				return;
			}
			// A player in the bin, sent off, or still on the bench is not out
			// there to have done anything.
			if (!onPitch(playerId)) {
				return;
			}
			if (armed.kind) {
				void append(armed.kind, { playerId });
				setArmed({ kind: armed.kind, playerId: null });
				return;
			}
			setArmed({ kind: null, playerId });
		},
		[append, armed.kind, onPitch, substitute],
	);

	/**
	 * Cancels this device's last tag.
	 *
	 * Only this device's: voiding the other coach's tap from here would be a
	 * correction they never asked for and cannot see coming. Control events are
	 * skipped too — undoing a clock change is what the clock buttons are for.
	 */
	const undo = useCallback(() => {
		const target = [...events]
			.reverse()
			.find(
				(event) =>
					event.deviceId === deviceId &&
					event.kind !== "void" &&
					!CLOCK_KINDS.has(event.kind) &&
					event.kind !== "zone_changed" &&
					!isVoided(events, event.id),
			);
		if (!target) {
			return;
		}
		void append("void", { voidsId: target.id });
	}, [append, deviceId, events]);

	const toggleClock = useCallback(() => {
		if (running) {
			void append("clock_paused");
			return;
		}
		// A new period starts where the last one stopped, because clockMs runs
		// continuously from kick-off.
		const starting = anchor === null || anchor.kind === "period_ended";
		if (starting) {
			void append("period_started", {
				period: state.period === 0 ? 1 : state.period + 1,
			});
			return;
		}
		void append("clock_resumed");
	}, [anchor, append, running, state.period]);

	const endPeriod = useCallback(() => void append("period_ended"), [append]);
	const endMatch = useCallback(() => void append("match_ended"), [append]);

	/**
	 * Writes a toggle, carrying the whole zone-and-possession state rather than
	 * a delta, so an event that arrives out of order still describes the pitch
	 * completely.
	 *
	 * The other half of the pair comes from `toggles` rather than the folded
	 * state. A tap is in IndexedDB before the live query has brought it back,
	 * so a coach who moves play upfield and switches possession in the same
	 * second would otherwise have the second event quietly carry the old zone
	 * and undo the first.
	 */
	const setToggles = useCallback(
		(zone: Zone, possession: Possession) => {
			const event = buildEvent("zone_changed", {
				payload: { zone, possession },
			});
			// Recorded before the write, so a second tap in the same second
			// already sees it rather than racing the live query.
			setPendingToggle({ id: event.id, zone, possession });
			void writeEvent(event);
		},
		[buildEvent, writeEvent],
	);

	const setZone = useCallback(
		(zone: Zone) => setToggles(zone, toggles.possession),
		[setToggles, toggles.possession],
	);

	const setPossession = useCallback(
		(possession: Possession) => setToggles(toggles.zone, possession),
		[setToggles, toggles.zone],
	);

	return {
		state,
		armed,
		clockMs: stampClock(),
		running,
		period: state.period === 0 ? 1 : state.period,
		zone: toggles.zone,
		possession: toggles.possession,
		events,
		tapAction,
		tapPlayer,
		undo,
		toggleClock,
		endPeriod,
		endMatch,
		setZone,
		setPossession,
	};
}

/**
 * The clock event the display hangs off: the last one in the merged log by
 * (clockMs, id), which is the same ordering the fold uses. Taking the last one
 * *this device* tapped would leave two iPads disagreeing about the time the
 * moment one of them paused.
 */
function clockAnchor(events: Event[]): Event | null {
	const clockEvents = events
		.filter((event) => CLOCK_KINDS.has(event.kind))
		.sort((a, b) =>
			a.clockMs !== b.clockMs
				? a.clockMs - b.clockMs
				: a.id < b.id
					? -1
					: a.id > b.id
						? 1
						: 0,
		);
	return clockEvents.at(-1) ?? null;
}

function pausedAtOrAfter(events: Event[], boundaryMs: number): boolean {
	return events.some(
		(event) => event.kind === "clock_paused" && event.clockMs >= boundaryMs,
	);
}

function isVoided(events: Event[], id: string): boolean {
	return events.some((event) => event.kind === "void" && event.voidsId === id);
}

function playerAttributed(catalogue: CatalogueEntry[]): Set<EventKind> {
	return new Set(
		catalogue.filter((entry) => entry.player).map((entry) => entry.kind),
	);
}
