import { useState } from "react";
import type { CatalogueEntry, EventGroup, EventKind } from "../../types/core";

const TABS: { group: EventGroup; label: string }[] = [
	{ group: "play", label: "Play" },
	{ group: "set_piece", label: "Set piece" },
	{ group: "discipline", label: "Discipline" },
];

type ActionPaneProps = {
	catalogue: CatalogueEntry[];
	armedKind: EventKind | null;
	onTapAction: (kind: EventKind) => void;
};

/**
 * The actions, three tabs deep, in the order the server served them.
 *
 * Buttons are never re-ordered by how often they are used: a predictable
 * position is the feature, and a grid that rearranges itself mid-match is how
 * a coach taps the wrong thing.
 */
export function ActionPane({
	catalogue,
	armedKind,
	onTapAction,
}: ActionPaneProps) {
	const [tab, setTab] = useState<EventGroup>("play");
	const actions = catalogue.filter((entry) => entry.group === tab);

	return (
		<div className="flex h-full flex-col gap-2">
			<div className="flex gap-1">
				{TABS.map((option) => (
					<button
						key={option.group}
						type="button"
						onClick={() => setTab(option.group)}
						aria-pressed={tab === option.group}
						className={`flex-1 rounded-lg px-3 py-2 text-sm font-semibold ${
							tab === option.group
								? "bg-slate-900 text-white"
								: "bg-white text-slate-700"
						}`}
					>
						{option.label}
					</button>
				))}
			</div>

			<div className="grid flex-1 grid-cols-3 gap-2">
				{actions.map((entry) => (
					<button
						key={entry.kind}
						type="button"
						onClick={() => onTapAction(entry.kind)}
						aria-pressed={armedKind === entry.kind}
						className={`rounded-lg px-2 py-4 text-sm font-semibold ${
							armedKind === entry.kind
								? "bg-slate-900 text-white"
								: "bg-white text-slate-800"
						}`}
					>
						{entry.label}
					</button>
				))}
			</div>
		</div>
	);
}
