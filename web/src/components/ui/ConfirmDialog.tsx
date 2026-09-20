import { useEffect, useId, useRef } from "react";

type Props = {
	title: string;
	confirmLabel: string;
	busyLabel?: string;
	busy?: boolean;
	onConfirm: () => void;
	onCancel: () => void;
	children: React.ReactNode;
};

/**
 * A modal for destructive actions. The confirm button is never focused on open
 * and Escape cancels, so the dangerous outcome is never one stray keypress away.
 */
export function ConfirmDialog({
	title,
	confirmLabel,
	busyLabel,
	busy = false,
	onConfirm,
	onCancel,
	children,
}: Props) {
	const titleId = useId();
	const cancelRef = useRef<HTMLButtonElement>(null);

	useEffect(() => {
		cancelRef.current?.focus();
	}, []);

	useEffect(() => {
		function onKeyDown(event: KeyboardEvent) {
			if (event.key === "Escape" && !busy) {
				onCancel();
			}
		}
		document.addEventListener("keydown", onKeyDown);
		return () => document.removeEventListener("keydown", onKeyDown);
	}, [busy, onCancel]);

	return (
		<div className="fixed inset-0 z-50 grid place-items-center bg-slate-900/50 px-4">
			<div
				role="dialog"
				aria-modal="true"
				aria-labelledby={titleId}
				className="w-full max-w-md space-y-4 rounded-xl bg-white p-6 shadow-xl"
			>
				<h2 id={titleId} className="text-lg font-bold text-slate-900">
					{title}
				</h2>
				<div className="space-y-2 text-sm text-slate-600">{children}</div>
				<div className="flex justify-end gap-3 pt-2">
					<button
						ref={cancelRef}
						type="button"
						onClick={onCancel}
						disabled={busy}
						className="rounded px-4 py-2 text-sm font-medium text-slate-700 hover:bg-slate-100 disabled:opacity-50"
					>
						Cancel
					</button>
					<button
						type="button"
						onClick={onConfirm}
						disabled={busy}
						className="rounded bg-red-700 px-4 py-2 text-sm font-semibold text-white hover:bg-red-800 disabled:opacity-50"
					>
						{busy ? (busyLabel ?? "Working…") : confirmLabel}
					</button>
				</div>
			</div>
		</div>
	);
}
