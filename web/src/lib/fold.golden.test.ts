import { readdirSync, readFileSync } from "node:fs";
import { dirname, join } from "node:path";
import { fileURLToPath } from "node:url";
import { describe, expect, it } from "vitest";
import type { Event, Match } from "../types/core";
import { fold } from "./fold";

// The same fixtures the Go tests run. Drift between the two folds fails CI here,
// which is the only reason it is safe to have written the fold twice.
//
// Resolved to a plain path rather than handed to fs as a URL: these tests run in
// jsdom, whose URL class is not the one node:fs recognises.
const goldenDir = join(
	dirname(fileURLToPath(import.meta.url)),
	"../../../testdata/golden",
);

type Fixture = {
	name: string;
	match: Match;
	events: Event[];
	expected: unknown;
};

const files = readdirSync(goldenDir)
	.filter((name) => name.endsWith(".json"))
	.sort();

describe("the golden fixtures", () => {
	// A suite that silently runs zero cases is worse than no suite: it reports
	// the same green as one that proved something.
	it("are found", () => {
		expect(files.length).toBeGreaterThan(0);
	});
});

describe.each(files)("%s", (file) => {
	const fixture = JSON.parse(
		readFileSync(join(goldenDir, file), "utf8"),
	) as Fixture;

	it(fixture.name, () => {
		const state = fold(fixture.match, fixture.events);

		// Round-tripping is not ceremony. It is what makes an undefined where Go
		// writes a key, or a Map where Go writes an object, fail here rather than
		// in a season report three milestones from now.
		expect(JSON.parse(JSON.stringify(state))).toEqual(fixture.expected);
	});
});
