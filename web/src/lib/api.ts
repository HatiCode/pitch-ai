export class ApiError extends Error {
	readonly status: number;

	constructor(status: number, message: string) {
		super(message);
		this.name = "ApiError";
		this.status = status;
	}
}

export type ApiFetch = <T>(path: string, init?: RequestInit) => Promise<T>;

/**
 * Builds a fetch wrapper for the Go API. Every request carries the caller's
 * Firebase ID token; the server resolves the club from it, so no request ever
 * names a club itself.
 */
export function createApiClient(
	getToken: () => Promise<string | null>,
): ApiFetch {
	return async function apiFetch<T>(
		path: string,
		init: RequestInit = {},
	): Promise<T> {
		const headers = new Headers(init.headers);

		const token = await getToken();
		if (token) {
			headers.set("Authorization", `Bearer ${token}`);
		}
		if (init.body !== undefined && !headers.has("Content-Type")) {
			headers.set("Content-Type", "application/json");
		}

		const response = await fetch(`/api${path}`, { ...init, headers });

		if (response.status === 204) {
			return undefined as T;
		}

		const text = await response.text();
		// A proxy or load balancer can return HTML on failure, so parsing is
		// best-effort: a broken body must not mask the status code.
		let body: unknown = null;
		if (text) {
			try {
				body = JSON.parse(text);
			} catch {
				body = null;
			}
		}

		if (!response.ok) {
			const message =
				(body as { error?: string } | null)?.error ??
				response.statusText ??
				"request failed";
			throw new ApiError(response.status, message);
		}
		return body as T;
	};
}
