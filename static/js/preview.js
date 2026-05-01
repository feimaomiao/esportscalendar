// Bootstraps the "Preview" page. Converts UTC times to local, and wires up
// the Export Calendar button. Re-runs when the inner partial is swapped.
(function init() {
	const exportBtn = document.getElementById('export-calendar-btn');
	if (!exportBtn) return;

	// Each IIFE invocation owns its event listeners. Aborting any prior
	// controller cleans up listeners attached on a previous swap.
	if (window.__previewAbort) window.__previewAbort.abort();
	const controller = new AbortController();
	window.__previewAbort = controller;
	const { signal } = controller;

	// app.js is deferred; on cold load convertMatchTimesIn isn't defined yet.
	// htmx-driven swaps land here with readyState already 'complete'.
	const localize = () => window.convertMatchTimesIn?.(document);
	if (document.readyState === 'loading') {
		document.addEventListener('DOMContentLoaded', localize, { once: true, signal });
	} else {
		localize();
	}

	function showLinkModal(url, autoCopied) {
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
			} catch {
				input.focus();
				input.select();
				setStatus('Press ⌘C / Ctrl+C to copy.', 'error');
			}
		});

		if (autoCopied) {
			setStatus('Link copied to clipboard.', 'success');
		}

		dialog.addEventListener('close', () => dialog.remove());
		document.body.appendChild(dialog);
		dialog.showModal();
	}

	async function exportCalendar() {
		const previewSelections = sessionStorage.getItem('preview-selections');
		if (!previewSelections) {
			window.showToast?.('No selections found. Please go back and make your selections again.', 'warning');
			return;
		}

		window.setButtonLoading?.(exportBtn, true);

		try {
			const response = await fetch('/export', {
				method: 'POST',
				headers: { 'Content-Type': 'application/json' },
				body: previewSelections,
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
			let autoCopied = false;
			try {
				await navigator.clipboard.writeText(data.url);
				autoCopied = true;
			} catch {}
			showLinkModal(data.url, autoCopied);
		} catch (err) {
			window.showToast?.('Error exporting calendar: ' + err.message, 'error');
		} finally {
			window.setButtonLoading?.(exportBtn, false);
		}
	}

	exportBtn.addEventListener('click', exportCalendar, { signal });

	// "Back to Selection" — re-POST stored game IDs to /lts so the leagues
	// & teams page is rebuilt server-side with the user's selections intact.
	const backBtn = document.getElementById('back-to-selection-btn');
	if (backBtn) {
		backBtn.addEventListener(
			'click',
			async () => {
				let stored = [];
				try {
					const raw = sessionStorage.getItem('selectedGameOptions');
					if (raw) stored = JSON.parse(raw);
				} catch {}

				if (!Array.isArray(stored) || stored.length === 0) {
					history.pushState({}, '', '/');
					location.assign('/');
					return;
				}

				backBtn.disabled = true;
				try {
					const response = await fetch('/lts', {
						method: 'POST',
						headers: {
							'Content-Type': 'application/json',
							'HX-Request': 'true',
							'HX-Target': 'page-content',
						},
						body: JSON.stringify({ options: stored }),
					});
					if (!response.ok) {
						window.showToast?.(`Navigation failed (${response.status}).`, 'error');
						return;
					}
					const html = await response.text();
					const target = document.getElementById('page-content');
					target.innerHTML = html;
					target.classList.add('fade-in');
					window.executeScriptsIn?.(target);
					if (typeof htmx !== 'undefined') htmx.process(target);
					history.pushState({}, '', '/lts');
					document.title = 'Leagues & Teams - EsportsCalendar';
				} catch (err) {
					window.showToast?.('Network error: ' + err.message, 'error');
				} finally {
					backBtn.disabled = false;
				}
			},
			{ signal },
		);
	}

	document.addEventListener(
		'keydown',
		(e) => {
			if (e.key !== 'Enter') return;
			const tag = ((document.activeElement && document.activeElement.tagName) || '').toLowerCase();
			if (tag === 'input' || tag === 'textarea') return;
			e.preventDefault();
			exportCalendar();
		},
		{ signal },
	);
})();
