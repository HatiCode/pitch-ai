import { expect, type Page, test } from "@playwright/test";
import { createApiStub, MATCH_ID, TEAM_ID } from "./stubs";

// The claim this milestone makes, end to end: a coach tags a full passage of
// play with no network, the screen keeps counting, a reload does not lose it,
// and everything reaches the server once signal returns — exactly once.

const TAGGING_URL = `/squads/${TEAM_ID}/matches/${MATCH_ID}/tag`;

async function tile(page: Page, playerId: string) {
	return page.getByTestId(`player-${playerId}`);
}

test("tags a passage of play offline and syncs it when the network returns", async ({
	page,
	context,
}) => {
	const api = createApiStub();
	await page.route("**/api/**", api.handle);

	await page.goto(TAGGING_URL);
	await expect(page.getByTestId("player-p1")).toBeVisible();
	await expect(page.getByRole("button", { name: "Tackle made" })).toBeVisible();

	// The reload later in this test is served by the service worker, so wait
	// until it is actually controlling the page. Cutting the network before it
	// claims leaves the browser with nowhere to fetch the shell from.
	await page.waitForFunction(
		() => Boolean(navigator.serviceWorker.controller),
		{
			timeout: 30_000,
		},
	);

	// Everything the screen needs is now cached, which is what makes the rest
	// of this test possible.
	await context.setOffline(true);
	api.setFailing(true);

	await page.getByRole("button", { name: "Start" }).click();
	await expect(page.getByText("Period 1 · running")).toBeVisible();

	// Ten tackles across three players, in the two-tap rhythm the screen is
	// built around: arm once, then tap players.
	await page.getByRole("button", { name: "Tackle made" }).click();
	for (const playerId of [
		"p7",
		"p7",
		"p7",
		"p7",
		"p8",
		"p8",
		"p8",
		"p9",
		"p9",
		"p9",
	]) {
		await (await tile(page, playerId)).click();
	}

	await expect(await tile(page, "p7")).toContainText("4");
	await expect(await tile(page, "p8")).toContainText("3");
	await expect(await tile(page, "p9")).toContainText("3");

	// A try, its conversion, and a substitution.
	await page.getByRole("button", { name: "Try", exact: true }).click();
	await (await tile(page, "p11")).click();
	await page.getByRole("button", { name: "Conversion" }).click();
	await (await tile(page, "p10")).click();
	await page.getByRole("button", { name: "Substitution" }).click();
	await (await tile(page, "p7")).click();
	await (await tile(page, "p16")).click();

	await expect(page.getByText("7–0")).toBeVisible();

	// One mis-tap, undone.
	await page.getByRole("button", { name: "Carry" }).click();
	await (await tile(page, "p9")).click();
	await expect(await tile(page, "p9")).toContainText("1");
	await page.getByRole("button", { name: "Undo" }).click();
	await expect(await tile(page, "p9")).toContainText("0");

	// Nothing has reached the server, and the screen has been counting anyway.
	expect(api.appended).toHaveLength(0);

	// IndexedDB, not memory: the counts have to survive the page going away.
	await page.reload();
	await expect(page.getByTestId("player-p7")).toBeVisible();
	await page.getByRole("button", { name: "Tackle made" }).click();
	await expect(await tile(page, "p7")).toContainText("4");
	await expect(await tile(page, "p8")).toContainText("3");
	await expect(page.getByText("7–0")).toBeVisible();

	await context.setOffline(false);
	api.setFailing(false);

	await expect(page.getByTestId("sync-status")).toHaveText("Synced", {
		timeout: 30_000,
	});

	const kinds = api.appended.map((event) => event.kind).sort();
	expect(kinds).toEqual(
		[
			"period_started",
			...Array(10).fill("tackle_made"),
			"try",
			"conversion_made",
			"sub",
			"carry",
			"void",
		].sort(),
	);

	// Exactly once. The endpoint is idempotent, but a client that sent the same
	// tap twice would still be a client with a bug.
	const ids = api.appended.map((event) => event.id);
	expect(new Set(ids).size).toBe(ids.length);
});
