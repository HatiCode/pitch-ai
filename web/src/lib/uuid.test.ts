import { afterEach, describe, expect, it, vi } from "vitest";
import { uuidv7 } from "./uuid";

const UUIDV7 =
	/^[0-9a-f]{8}-[0-9a-f]{4}-7[0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}$/;

afterEach(() => {
	vi.useRealTimers();
});

describe("uuidv7", () => {
	it("has the version 7 and RFC variant bits", () => {
		expect(uuidv7()).toMatch(UUIDV7);
	});

	it("does not repeat", () => {
		const ids = new Set<string>();
		for (let i = 0; i < 10_000; i++) {
			ids.add(uuidv7());
		}
		expect(ids.size).toBe(10_000);
	});

	// The ordering property is the one that matters: it is what lets the fold
	// sort one log written by two devices that never coordinated.
	it("sorts lexicographically in time order", () => {
		vi.useFakeTimers();

		vi.setSystemTime(new Date("2026-03-14T15:00:00Z"));
		const early = uuidv7();
		vi.setSystemTime(new Date("2026-03-14T15:00:01Z"));
		const late = uuidv7();

		expect(early < late).toBe(true);
	});

	// The counter has twelve bits. A burst past that has to borrow from the next
	// millisecond rather than wrap, or a fast tagger's IDs stop sorting.
	it("survives a burst longer than the counter", () => {
		vi.useFakeTimers();
		vi.setSystemTime(new Date("2026-03-14T15:00:00Z"));

		const ids = Array.from({ length: 5000 }, () => uuidv7());

		expect(new Set(ids).size).toBe(5000);
		expect([...ids].sort()).toEqual(ids);
	});

	// A clock correction mid-match must not rewind the log's order.
	it("does not go backwards when the clock does", () => {
		vi.useFakeTimers();

		vi.setSystemTime(new Date("2026-03-14T15:00:05Z"));
		const before = uuidv7();
		vi.setSystemTime(new Date("2026-03-14T15:00:00Z"));
		const after = uuidv7();

		expect(after > before).toBe(true);
	});

	it("stays distinct and ordered within one millisecond", () => {
		vi.useFakeTimers();
		vi.setSystemTime(new Date("2026-03-14T15:00:00Z"));

		const ids = Array.from({ length: 100 }, () => uuidv7());

		expect(new Set(ids).size).toBe(100);
		expect([...ids].sort()).toEqual(ids);
	});
});
