import type {
	Event,
	EventGroup,
	EventKind,
	Match,
	MatchState,
	Position,
	PositionGroup,
	Possession,
	Slice,
	Zone,
} from "../types/core";
import { GROUP_OF_POSITION, POSITION_OF_JERSEY } from "../types/groups";

// The second implementation of internal/core/stats.go, deliberately mirroring it
// pass for pass and name for name. The iPad needs live counts with no network,
// which is why the fold exists twice; the golden fixtures in testdata/golden are
// what stop the two from drifting. Anything clever here costs more the first time
// a fixture disagrees and the two have to be read side by side.

export const SLICE_DURATION_MS = 20 * 60 * 1000;

// A yellow card's cost, spent in running clock rather than wall clock, so a bin
// cannot expire during a stoppage.
const SIN_BIN_MS = 10 * 60 * 1000;

// Go's zero value for an unset position, which is what a replacement has until
// they come on. The generated union has no member for it, and inventing one
// would make every exhaustive switch in the app carry a case that means "not
// applicable yet".
const NO_POSITION = "" as Position;
const NO_GROUP = "" as PositionGroup;

/**
 * The behavioural half of the catalogue: which group a kind belongs to, whether
 * it is attributed to a player, and what it scores.
 *
 * Duplicated from internal/core/catalogue.go on purpose. The served catalogue
 * is presentation — labels, tab order — and a client can be holding a stale
 * copy of it, so the fold's arithmetic must not depend on having fetched
 * anything. The golden fixtures are what keep the two tables honest.
 *
 * Typed as a total Record rather than a Partial: a kind added in Go regenerates
 * the EventKind union and breaks this file at compile time, which is the
 * failure everyone wants over a kind that silently tallies nothing.
 */
type KindRule = { group: EventGroup; player: boolean; points: number };

const KINDS: Record<EventKind, KindRule> = {
	tackle_made: { group: "play", player: true, points: 0 },
	tackle_missed: { group: "play", player: true, points: 0 },
	carry: { group: "play", player: true, points: 0 },
	linebreak: { group: "play", player: true, points: 0 },
	defender_beaten: { group: "play", player: true, points: 0 },
	offload: { group: "play", player: true, points: 0 },
	clearout: { group: "play", player: true, points: 0 },
	jackal_won: { group: "play", player: true, points: 0 },
	turnover_won: { group: "play", player: true, points: 0 },
	handling_error: { group: "play", player: true, points: 0 },
	kick: { group: "play", player: true, points: 0 },
	try: { group: "play", player: true, points: 5 },
	sub: { group: "play", player: false, points: 0 },

	scrum_won: { group: "set_piece", player: false, points: 0 },
	scrum_lost: { group: "set_piece", player: false, points: 0 },
	scrum_penalty_won: { group: "set_piece", player: false, points: 0 },
	scrum_penalty_conceded: { group: "set_piece", player: false, points: 0 },
	lineout_won: { group: "set_piece", player: true, points: 0 },
	lineout_lost: { group: "set_piece", player: true, points: 0 },
	throw_not_straight: { group: "set_piece", player: true, points: 0 },
	maul_from_lineout: { group: "set_piece", player: false, points: 0 },
	restart_received: { group: "set_piece", player: true, points: 0 },
	restart_lost: { group: "set_piece", player: false, points: 0 },

	offside: { group: "discipline", player: true, points: 0 },
	ruck_offence: { group: "discipline", player: true, points: 0 },
	high_tackle: { group: "discipline", player: true, points: 0 },
	not_releasing: { group: "discipline", player: true, points: 0 },
	scrum_offence: { group: "discipline", player: true, points: 0 },
	foul_play: { group: "discipline", player: true, points: 0 },
	yellow_card: { group: "discipline", player: true, points: 0 },
	red_card: { group: "discipline", player: true, points: 0 },

	conversion_made: { group: "score", player: true, points: 2 },
	conversion_missed: { group: "score", player: true, points: 0 },
	penalty_goal: { group: "score", player: true, points: 3 },
	drop_goal: { group: "score", player: true, points: 3 },
	opposition_try: { group: "score", player: false, points: 5 },
	opposition_conversion: { group: "score", player: false, points: 2 },
	opposition_penalty: { group: "score", player: false, points: 3 },
	opposition_drop: { group: "score", player: false, points: 3 },

	period_started: { group: "control", player: false, points: 0 },
	clock_paused: { group: "control", player: false, points: 0 },
	clock_resumed: { group: "control", player: false, points: 0 },
	period_ended: { group: "control", player: false, points: 0 },
	match_ended: { group: "control", player: false, points: 0 },
	zone_changed: { group: "control", player: false, points: 0 },
	void: { group: "control", player: false, points: 0 },
};

// The one kind of scoring event that belongs to nobody, derived the way Go
// derives it rather than kept as a second list of names: this application never
// holds the other team's squad.
function isOppositionScore(rule: KindRule): boolean {
	return rule.group === "score" && !rule.player;
}

const ZONES = new Set<string>(["our_22", "our_half", "their_half", "their_22"]);
const POSSESSIONS = new Set<string>(["us", "them"]);

/**
 * Derives the whole state of a match from its event log. Pure and total: no
 * clock, no I/O, no throw. The same events in any order return the same state,
 * which is what makes two coaches tagging one match, and a correction after the
 * fact, both safe.
 *
 * Time accumulates up to the clock of the last event in the log. For a finished
 * match that is exact; during live tagging per-player minutes therefore lag by
 * one tap, which no coach is reading mid-match.
 */
export function fold(match: Match, events: Event[]): MatchState {
	const voided = voidedIds(events);
	const cancelled = new Set(voided);

	const state = newState(match);
	state.voidedIds = voided;

	const folder = new Folder(state, match);
	for (const event of sortedLive(events, cancelled)) {
		folder.advanceTo(event.clockMs);
		folder.apply(event);
	}
	folder.finish();

	return state;
}

/**
 * Collects every cancellation in the log. A void that is itself voided still
 * counts: the tagging screen only ever voids live events, and a
 * cancel-the-cancel rule would make the log's meaning depend on read order.
 */
function voidedIds(events: Event[]): string[] {
	const ids = new Set<string>();
	for (const event of events) {
		if (event.kind === "void" && event.voidsId) {
			ids.add(event.voidsId);
		}
	}
	return [...ids].sort();
}

/**
 * Drops cancelled events and the voids themselves, then orders what is left by
 * clock with the event ID breaking ties. IDs are UUIDv7 and so ordered by
 * creation time, which gives two devices tagging the same instant one stable
 * order. The input array is never touched — the caller is handing over a live
 * Dexie query result.
 */
function sortedLive(events: Event[], cancelled: Set<string>): Event[] {
	const live = events.filter(
		(event) => event.kind !== "void" && !cancelled.has(event.id),
	);

	// Numeric on the clock and lexicographic on the ID. JavaScript's default
	// sort would compare 600000 and 60000 as strings and put them the wrong way
	// round.
	return live.sort((a, b) => {
		if (a.clockMs !== b.clockMs) {
			return a.clockMs - b.clockMs;
		}
		return a.id < b.id ? -1 : a.id > b.id ? 1 : 0;
	});
}

/**
 * Seeds the state from the selection. Every collection is initialised, because
 * Go emits `{}` for an empty map and `[]` for an empty slice, and this JSON has
 * to match that exactly.
 */
function newState(match: Match): MatchState {
	const state: MatchState = {
		status: "scheduled",
		period: 0,
		clockMs: 0,
		running: false,
		score: { us: 0, them: 0 },
		zone: "our_half",
		possession: "us",
		territoryMs: {},
		possessionMs: {},
		counts: {},
		players: {},
		slices: [],
		voidedIds: [],
	};

	for (const slot of match.lineup.starters ?? []) {
		// A starting jersey names a position; a bench number does not, so a
		// replacement's position is filled in when they come on.
		const position = POSITION_OF_JERSEY[slot.jersey] ?? NO_POSITION;
		state.players[slot.playerId] = {
			playerId: slot.playerId,
			jersey: slot.jersey,
			position,
			group: GROUP_OF_POSITION[position] ?? NO_GROUP,
			onPitch: false,
			minutesMs: 0,
			counts: {},
		};
	}
	for (const slot of match.lineup.bench ?? []) {
		state.players[slot.playerId] = {
			playerId: slot.playerId,
			jersey: slot.jersey,
			position: NO_POSITION,
			group: NO_GROUP,
			onPitch: false,
			minutesMs: 0,
			counts: {},
		};
	}
	return state;
}

/**
 * Carries the working state of one walk over the log. These fields are how the
 * state is derived rather than part of it, which is why they do not live on
 * MatchState.
 */
class Folder {
	private running = false;
	private lastClock = 0;
	private zone: Zone;
	private possession: Possession;
	private onPitch = new Set<string>();
	private binBudget = new Map<string, number>();
	private sentOff = new Set<string>();

	// Assigned in the body rather than declared as a parameter property, which
	// tsconfig's erasableSyntaxOnly rules out.
	private readonly state: MatchState;

	constructor(state: MatchState, match: Match) {
		this.state = state;
		this.zone = state.zone;
		this.possession = state.possession;
		for (const slot of match.lineup.starters ?? []) {
			this.onPitch.add(slot.playerId);
		}
	}

	/**
	 * Attributes the span between the last event and this one. A stopped clock
	 * contributes nothing — that is the whole of the pause behaviour, and it is
	 * why a stoppage cannot leak into a single denominator.
	 *
	 * Accumulating span by span, rather than opening and closing an interval per
	 * player, is what keeps stoppages, substitutions and sin-bins from needing
	 * three separate rules.
	 */
	advanceTo(clockMs: number): void {
		const from = this.lastClock;
		const to = clockMs;
		if (to > from) {
			this.lastClock = to;
		}
		if (!this.running || to <= from) {
			return;
		}

		const span = to - from;
		add(this.state.territoryMs, this.zone, span);
		add(this.state.possessionMs, this.possession, span);
		addSpan(from, to, (index, ms) => {
			const bucket = this.slice(index);
			add(bucket.territoryMs, this.zone, ms);
			add(bucket.possessionMs, this.possession, ms);
		});

		for (const playerId of this.onPitch) {
			this.addMinutes(playerId, span);
		}
		this.expireBins(span);
	}

	/**
	 * Spends running clock against every sin-bin. A bin that runs out part way
	 * through a span puts its player back on for the remainder, which is why the
	 * budget is milliseconds owed rather than a return time.
	 */
	private expireBins(span: number): void {
		for (const [playerId, remaining] of [...this.binBudget]) {
			if (remaining > span) {
				this.binBudget.set(playerId, remaining - span);
				continue;
			}

			this.binBudget.delete(playerId);
			this.onPitch.add(playerId);
			this.addMinutes(playerId, span - remaining);
		}
	}

	apply(event: Event): void {
		const rule = KINDS[event.kind];
		if (!rule) {
			// A kind this build does not know about, the same way the Go fold
			// skips one that fails its catalogue lookup.
			return;
		}
		if (this.state.status === "scheduled") {
			this.state.status = "in_progress";
		}

		// Control events drive the clock and the toggles. None of them is an
		// occurrence a coach tallies, so none reaches the counters below.
		if (rule.group === "control") {
			this.applyControl(event);
			return;
		}

		if (rule.player) {
			if (!(event.playerId && event.playerId in this.state.players)) {
				// Tagged against someone since removed from the lineup.
				// Attributing it anywhere would be a guess, so it is dropped.
				return;
			}
			this.countForPlayer(event.playerId, event.kind);
		}
		this.count(event.kind, event.clockMs);
		this.score(rule, event.clockMs);

		switch (event.kind) {
			case "sub":
				this.substitute(event);
				break;
			case "yellow_card":
				this.sinBin(event.playerId);
				break;
			case "red_card":
				this.sendOff(event.playerId);
				break;
		}
	}

	private applyControl(event: Event): void {
		switch (event.kind) {
			case "period_started":
			case "clock_resumed":
				this.running = true;
				this.state.period = event.period;
				break;
			case "clock_paused":
			case "period_ended":
				this.running = false;
				break;
			case "match_ended":
				this.running = false;
				this.state.status = "completed";
				break;
			case "zone_changed": {
				// The toggle carries its whole state rather than a delta, so an
				// unknown value leaves the current setting alone instead of
				// corrupting it.
				const zone = event.payload?.zone;
				if (zone && ZONES.has(zone)) {
					this.zone = zone as Zone;
				}
				const possession = event.payload?.possession;
				if (possession && POSSESSIONS.has(possession)) {
					this.possession = possession as Possession;
				}
				break;
			}
		}
	}

	private substitute(event: Event): void {
		const on = event.payload?.onPlayerId ?? "";
		const off = event.payload?.offPlayerId ?? "";
		if (!(on in this.state.players)) {
			return;
		}

		// The outgoing player may already be off — carded, or replaced earlier —
		// and the incoming player still comes on.
		this.onPitch.delete(off);
		this.binBudget.delete(off);
		this.onPitch.add(on);
		this.inheritPosition(on, off);
	}

	/**
	 * Gives a replacement the position of whoever they came on for. A bench slot
	 * only carries a number, so without this a replacement has no comparison
	 * group and drops out of every position-group ranking. It applies only while
	 * the position is unset, so a player who comes on twice keeps the first.
	 */
	private inheritPosition(on: string, off: string): void {
		const outgoing = this.state.players[off];
		if (!outgoing || outgoing.position === NO_POSITION) {
			return;
		}

		const incoming = this.state.players[on];
		if (incoming.position !== NO_POSITION) {
			return;
		}
		incoming.position = outgoing.position;
		incoming.group = outgoing.group;
	}

	private sinBin(playerId: string | undefined): void {
		if (!playerId || this.sentOff.has(playerId)) {
			return;
		}
		this.onPitch.delete(playerId);
		this.binBudget.set(playerId, SIN_BIN_MS);
	}

	private sendOff(playerId: string | undefined): void {
		if (!playerId) {
			return;
		}
		this.onPitch.delete(playerId);
		this.binBudget.delete(playerId);
		this.sentOff.add(playerId);
	}

	private count(kind: EventKind, clockMs: number): void {
		add(this.state.counts, kind, 1);
		add(this.slice(sliceIndex(clockMs)).counts, kind, 1);
	}

	private score(rule: KindRule, clockMs: number): void {
		if (rule.points === 0) {
			return;
		}

		const bucket = this.slice(sliceIndex(clockMs));
		if (isOppositionScore(rule)) {
			this.state.score.them += rule.points;
			bucket.score.them += rule.points;
			return;
		}
		this.state.score.us += rule.points;
		bucket.score.us += rule.points;
	}

	private countForPlayer(playerId: string, kind: EventKind): void {
		const stats = this.state.players[playerId];
		if (!stats) {
			return;
		}
		add(stats.counts, kind, 1);
	}

	private addMinutes(playerId: string, ms: number): void {
		const stats = this.state.players[playerId];
		if (!stats) {
			return;
		}
		stats.minutesMs += ms;
	}

	/**
	 * Returns the 20-minute bucket at index, growing the list so it stays
	 * contiguous from kick-off: a report renders the blocks in order and cannot
	 * have a hole in the middle.
	 */
	private slice(index: number): Slice {
		while (this.state.slices.length <= index) {
			const start = this.state.slices.length * SLICE_DURATION_MS;
			this.state.slices.push({
				fromMs: start,
				toMs: start + SLICE_DURATION_MS,
				score: { us: 0, them: 0 },
				territoryMs: {},
				possessionMs: {},
				counts: {},
			});
		}
		return this.state.slices[index];
	}

	finish(): void {
		this.state.clockMs = this.lastClock;
		this.state.running = this.running;
		this.state.zone = this.zone;
		this.state.possession = this.possession;

		for (const [playerId, stats] of Object.entries(this.state.players)) {
			stats.onPitch = this.onPitch.has(playerId);
		}
	}
}

/**
 * Attributes a duration to every 20-minute slice it overlaps, so a span running
 * from 19:00 to 22:00 lands one minute in one bucket and two in the next.
 */
function addSpan(
	fromMs: number,
	toMs: number,
	apply: (index: number, ms: number) => void,
): void {
	let cursor = Math.max(fromMs, 0);

	while (cursor < toMs) {
		const index = sliceIndex(cursor);
		const end = Math.min((index + 1) * SLICE_DURATION_MS, toMs);
		apply(index, end - cursor);
		cursor = end;
	}
}

// Go divides integers; JavaScript does not.
function sliceIndex(clockMs: number): number {
	return Math.floor(clockMs / SLICE_DURATION_MS);
}

/**
 * Increments a counter, creating it on first use. Zero-valued entries are never
 * written, because Go's fold does not write them either and the two serialised
 * states are compared key for key.
 */
function add<K extends string>(
	counts: Partial<Record<K, number>>,
	key: K,
	amount: number,
): void {
	counts[key] = ((counts[key] ?? 0) + amount) as Partial<Record<K, number>>[K];
}
