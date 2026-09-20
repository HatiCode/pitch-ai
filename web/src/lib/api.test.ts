import { afterEach, describe, expect, it, vi } from "vitest";
import { ApiError, createApiClient } from "./api";

function mockFetch(response: Response) {
	const spy = vi.fn().mockResolvedValue(response);
	vi.stubGlobal("fetch", spy);
	return spy;
}

afterEach(() => {
	vi.unstubAllGlobals();
});

describe("createApiClient", () => {
	it("prefixes /api and attaches the bearer token", async () => {
		const spy = mockFetch(
			new Response(JSON.stringify({ id: "p1" }), {
				status: 200,
				headers: { "Content-Type": "application/json" },
			}),
		);
		const apiFetch = createApiClient(async () => "token-abc");

		const result = await apiFetch<{ id: string }>("/players");

		expect(result).toEqual({ id: "p1" });
		const [url, init] = spy.mock.calls[0];
		expect(url).toBe("/api/players");
		expect(new Headers(init.headers).get("Authorization")).toBe(
			"Bearer token-abc",
		);
	});

	it("omits the header when there is no token", async () => {
		const spy = mockFetch(new Response("{}", { status: 200 }));
		const apiFetch = createApiClient(async () => null);

		await apiFetch("/players");

		const [, init] = spy.mock.calls[0];
		expect(new Headers(init.headers).get("Authorization")).toBeNull();
	});

	it("sets a JSON content type when sending a body", async () => {
		const spy = mockFetch(new Response("{}", { status: 200 }));
		const apiFetch = createApiClient(async () => "token-abc");

		await apiFetch("/players", {
			method: "POST",
			body: JSON.stringify({ a: 1 }),
		});

		const [, init] = spy.mock.calls[0];
		expect(new Headers(init.headers).get("Content-Type")).toBe(
			"application/json",
		);
	});

	it("returns undefined for 204 responses", async () => {
		mockFetch(new Response(null, { status: 204 }));
		const apiFetch = createApiClient(async () => "token-abc");

		await expect(
			apiFetch("/players/p1", { method: "DELETE" }),
		).resolves.toBeUndefined();
	});

	it("throws ApiError carrying the status and server message", async () => {
		mockFetch(
			new Response(JSON.stringify({ error: "lastName: must not be empty" }), {
				status: 400,
			}),
		);
		const apiFetch = createApiClient(async () => "token-abc");

		await expect(
			apiFetch("/players", { method: "POST", body: "{}" }),
		).rejects.toMatchObject({
			status: 400,
			message: "lastName: must not be empty",
		});
	});

	it("throws ApiError instances so callers can narrow on status", async () => {
		mockFetch(new Response(JSON.stringify({ error: "nope" }), { status: 403 }));
		const apiFetch = createApiClient(async () => "token-abc");

		const error = await apiFetch("/me").catch((e: unknown) => e);
		expect(error).toBeInstanceOf(ApiError);
		expect((error as ApiError).status).toBe(403);
	});

	it("falls back to the status text when the body is not JSON", async () => {
		mockFetch(
			new Response("<html>gateway blew up</html>", {
				status: 502,
				statusText: "Bad Gateway",
			}),
		);
		const apiFetch = createApiClient(async () => "token-abc");

		const error = await apiFetch("/players").catch((e: unknown) => e);
		expect(error).toBeInstanceOf(ApiError);
		expect((error as ApiError).status).toBe(502);
	});
});
