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

export function AuthProvider({ children }: { children: React.ReactNode }) {
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
