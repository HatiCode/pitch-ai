import {
	onAuthStateChanged,
	signInWithPopup,
	signOut,
	type User,
} from "firebase/auth";
import { createContext, useContext, useEffect, useMemo, useState } from "react";
import type { Membership } from "../types/auth";
import { type ApiFetch, createApiClient } from "./api";
import { firebaseAuth, googleProvider } from "./firebase";

type AuthState = {
	user: User | null;
	membership: Membership | null;
	loading: boolean;
	error: string | null;
	apiFetch: ApiFetch;
	signIn: () => Promise<void>;
	signOutUser: () => Promise<void>;
};

const AuthContext = createContext<AuthState | null>(null);

/**
 * True only for the end-to-end build. VITE_E2E is set by the test:e2e script
 * and by nothing else, and Vite inlines it at build time, so the production
 * bundle keeps the real branch and carries none of the stub below.
 */
const E2E = import.meta.env.VITE_E2E === "1";

const E2E_MEMBERSHIP: Membership = {
	uid: "e2e-coach",
	clubId: "club-e2e",
	teamIds: ["team-e2e"],
	role: "coach",
};

/**
 * Stands in for Firebase when the end-to-end test runs. That test is about the
 * outbox surviving a dead network, and driving a real sign-in popup to prove
 * it would test Google's availability instead.
 */
function useStubbedAuth(): AuthState {
	const apiFetch = useMemo(() => createApiClient(async () => null), []);
	return {
		// The shell renders a sign-in screen without a user, and only reads the
		// address off it, so that is all the stub carries.
		user: { email: "coach@example.test" } as User,
		membership: E2E_MEMBERSHIP,
		loading: false,
		error: null,
		apiFetch,
		signIn: async () => {},
		signOutUser: async () => {},
	};
}

export function AuthProvider({ children }: { children: React.ReactNode }) {
	if (E2E) {
		return <E2EAuthProvider>{children}</E2EAuthProvider>;
	}
	return <FirebaseAuthProvider>{children}</FirebaseAuthProvider>;
}

function E2EAuthProvider({ children }: { children: React.ReactNode }) {
	const value = useStubbedAuth();
	return <AuthContext.Provider value={value}>{children}</AuthContext.Provider>;
}

function FirebaseAuthProvider({ children }: { children: React.ReactNode }) {
	const [user, setUser] = useState<User | null>(null);
	const [membership, setMembership] = useState<Membership | null>(null);
	const [loading, setLoading] = useState(true);
	const [error, setError] = useState<string | null>(null);

	const apiFetch = useMemo(
		() =>
			createApiClient(
				() => firebaseAuth.currentUser?.getIdToken() ?? Promise.resolve(null),
			),
		[],
	);

	useEffect(() => {
		return onAuthStateChanged(firebaseAuth, async (next) => {
			setUser(next);
			setError(null);

			if (!next) {
				setMembership(null);
				setLoading(false);
				return;
			}

			try {
				setMembership(await apiFetch<Membership>("/me"));
			} catch {
				// A valid Google account that belongs to no club lands here: the
				// server authenticated it (200 on the token) but returned 403.
				setMembership(null);
				setError("This account is not a member of any club yet.");
			} finally {
				setLoading(false);
			}
		});
	}, [apiFetch]);

	const value: AuthState = {
		user,
		membership,
		loading,
		error,
		apiFetch,
		signIn: async () => {
			await signInWithPopup(firebaseAuth, googleProvider);
		},
		signOutUser: async () => {
			await signOut(firebaseAuth);
		},
	};

	return <AuthContext.Provider value={value}>{children}</AuthContext.Provider>;
}

export function useAuth(): AuthState {
	const ctx = useContext(AuthContext);
	if (!ctx) {
		throw new Error("useAuth must be used inside <AuthProvider>");
	}
	return ctx;
}
