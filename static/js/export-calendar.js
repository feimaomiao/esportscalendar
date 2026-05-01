// Shared helper for the Export Calendar action. Used by /calendar.
// POSTs the supplied {selections, hideScores} payload to /export and shows a
// modal with the returned subscription URL. The user copies via the Copy
// button (no auto-copy on open).
//
// Public API: window.exportCalendarPayload(payload, button)
//   payload — object with shape { selections: {...}, hideScores: boolean }
//             matching the format /api/calendar and /api/fixtures already use.
//   button  — the trigger element. Used with window.setButtonLoading so the
//             button shows a spinner while the request is in flight.
//
// Failure modes (each surfaces a toast and returns without throwing):
//   * Non-2xx response: shows the server's error message when present.
//   * Network error: shows the JS error message.
//   * Clipboard rejected on user click: status line tells the user to copy
//     manually with the keyboard shortcut.
(function init() {
	if (typeof window.exportCalendarPayload === 'function') return;

	function showLinkModal(url) {
		const dialog = document.createElement('dialog');
		dialog.className = 'modal';
		dialog.innerHTML = `
			<div class="modal-box max-w-lg">
				<h3 class="font-bold text-lg mb-2">Calendar Link Created</h3>
				<p class="text-sm text-base-content/70 mb-4">Add this URL to your calendar app (Google Calendar → "From URL", Apple Calendar → "New Calendar Subscription", etc.) — it stays in sync as new matches are scheduled.</p>
				<input type="text" readonly class="input input-bordered w-full font-mono text-xs" id="calendar-url-input">
				<p class="text-xs mt-2 h-4" id="calendar-url-status" aria-live="polite"></p>
				<div class="flex justify-end gap-2 mt-4">
					<form method="dialog" class="contents">
						<button type="submit" class="btn">Done</button>
					</form>
					<button type="button" class="btn btn-primary" id="calendar-url-copy">Copy</button>
				</div>
			</div>
			<form method="dialog" class="modal-backdrop">
				<button type="submit" aria-label="Close">close</button>
			</form>
		`;
		const input = dialog.querySelector('#calendar-url-input');
		const copyBtn = dialog.querySelector('#calendar-url-copy');
		const status = dialog.querySelector('#calendar-url-status');

		input.value = url;
		input.addEventListener('focus', () => input.select());
		input.addEventListener('click', () => input.select());

		function setStatus(text, kind) {
			status.textContent = text;
			status.className = 'text-xs mt-2 h-4 ' + (kind === 'error' ? 'text-error' : 'text-success');
		}

		copyBtn.addEventListener('click', async () => {
			try {
				await navigator.clipboard.writeText(url);
				setStatus('Link copied to clipboard.', 'success');
				copyBtn.classList.add('hud-copy-success');
			} catch {
				input.focus();
				input.select();
				setStatus('Press ⌘C / Ctrl+C to copy.', 'error');
			}
		});

		dialog.addEventListener('close', () => dialog.remove());
		document.body.appendChild(dialog);
		dialog.showModal();
	}

	async function exportCalendarPayload(payload, button) {
		if (!payload || typeof payload !== 'object') {
			window.showToast?.('No selections to export.', 'warning');
			return;
		}

		if (button) window.setButtonLoading?.(button, true);

		try {
			const response = await fetch('/export', {
				method: 'POST',
				headers: { 'Content-Type': 'application/json' },
				body: JSON.stringify(payload),
			});

			if (!response.ok) {
				let message = 'Unknown error';
				try {
					const errorData = await response.json();
					if (errorData && errorData.error) message = errorData.error;
				} catch {}
				window.showToast?.('Failed to export calendar: ' + message, 'error');
				return;
			}

			const data = await response.json();
			showLinkModal(data.url);
		} catch (err) {
			window.showToast?.('Error exporting calendar: ' + err.message, 'error');
		} finally {
			if (button) window.setButtonLoading?.(button, false);
		}
	}

	window.exportCalendarPayload = exportCalendarPayload;
})();
