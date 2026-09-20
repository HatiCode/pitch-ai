import { initializeApp } from "firebase/app";
import { GoogleAuthProvider, getAuth } from "firebase/auth";

const config = {
	apiKey: import.meta.env.VITE_FIREBASE_API_KEY,
	authDomain: import.meta.env.VITE_FIREBASE_AUTH_DOMAIN,
	projectId: import.meta.env.VITE_FIREBASE_PROJECT_ID,
};

if (!config.apiKey || !config.projectId) {
	// Vite inlines these at build time, so a missing value means the build was
	// run without .env.local — fail loudly rather than showing a broken login.
	throw new Error(
		"Firebase config missing: copy web/.env.example to web/.env.local and fill it in",
	);
}

export const firebaseApp = initializeApp(config);
export const firebaseAuth = getAuth(firebaseApp);
export const googleProvider = new GoogleAuthProvider();
