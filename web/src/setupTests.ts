// The /vitest entry point both registers the matchers with Vitest's expect and
// augments its Assertion type, so `toBeChecked()` and friends type-check.
import "@testing-library/jest-dom/vitest";
// jsdom has no IndexedDB, and IndexedDB is where the app keeps its state.
import "fake-indexeddb/auto";
