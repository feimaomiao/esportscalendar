// Bootstraps the "Leagues & Teams" page. Initializes each game card and wires
// up the submit-for-preview button. Re-runs when the inner partial is swapped.
(function init() {
	const cards = document.querySelectorAll('[data-game-id]');
	if (cards.length === 0) return;

	// Each IIFE invocation owns its event listeners. Aborting any prior
	// controller cleans up listeners attached on a previous swap.
	if (window.__ltsAbort) window.__ltsAbort.abort();
	const controller = new AbortController();
	window.__ltsAbort = controller;
	const { signal } = controller;

	function ensureGameSelectionThen(callback) {
		if (typeof initGameSelection === 'function') {
			callback();
			return;
		}
		const script = document.createElement('script');
		script.src = '/static/js/game-selection.js';
		script.onload = callback;
		script.onerror = () => {
			window.showToast?.('Failed to load selection module.', 'error');
		};
		document.head.appendChild(script);
	}

	ensureGameSelectionThen(() => {
		cards.forEach((card) => {
			const gameId = card.getAttribute('data-game-id');
			if (gameId) initGameSelection(gameId);
		});
		// Allow the async league/team fetches a moment to settle before checking
		// whether the submit button should enable.
		setTimeout(() => {
			if (typeof checkAndUpdateSubmitButton === 'function') {
				checkAndUpdateSubmitButton();
			}
		}, 1000);
	});

	const backBtn = document.getElementById('back-to-options-btn');
	if (backBtn) {
		backBtn.addEventListener('click', () => {
			// Hard nav — full page reload to "/" — the only reliable way to
			// guarantee a clean DOM state. Cheap on localhost; small payload.
			window.location.assign('/');
		}, { signal });
	}

	const submitBtn = document.getElementById('submit-selection-btn');
	if (!submitBtn) return;

	function collectSelections() {
		const selections = {};
		document.querySelectorAll('[data-game-id]').forEach((card) => {
			const gameId = card.getAttribute('data-game-id');
			const container = card.querySelector(`#selected-combined-${gameId}`);
			if (!container) return;

			const leagues = [];
			const teams = [];
			container.querySelectorAll('.badge').forEach((badge) => {
				if (badge.classList.contains('badge-primary') && badge.hasAttribute('data-league-id')) {
					leagues.push(parseInt(badge.getAttribute('data-league-id'), 10));
				} else if (badge.classList.contains('badge-secondary') && badge.hasAttribute('data-team-id')) {
					teams.push(parseInt(badge.getAttribute('data-team-id'), 10));
				}
			});

			let maxTier = 2;
			try {
				const raw = sessionStorage.getItem('lts-selections-' + gameId);
				if (raw) {
					const parsed = JSON.parse(raw);
					if (parsed && typeof parsed.maxTier === 'number') maxTier = parsed.maxTier;
				}
			} catch {}

			if (leagues.length > 0 || teams.length > 0) {
				leagues.sort((a, b) => a - b);
				teams.sort((a, b) => a - b);
				selections[gameId] = { leagues, teams, maxTier };
			}
		});

		// Sort by gameId for stable cache keys.
		const sorted = {};
		Object.keys(selections)
			.sort((a, b) => parseInt(a, 10) - parseInt(b, 10))
			.forEach((k) => { sorted[k] = selections[k]; });
		return sorted;
	}

	async function submitPreview() {
		const selections = collectSelections();
		if (Object.keys(selections).length === 0) {
			window.showToast?.('Please select at least one league or team before submitting.', 'warning');
			return;
		}

		const hideScores = !!document.getElementById('hide-scores-checkbox')?.checked;
		const payload = { selections, hideScores };
		sessionStorage.setItem('preview-selections', JSON.stringify(payload));

		window.setButtonLoading?.(submitBtn, true);

		try {
			const response = await fetch('/preview', {
				method: 'POST',
				headers: {
					'Content-Type': 'application/json',
					'HX-Request': 'true',
					'HX-Target': 'page-content',
				},
				body: JSON.stringify(payload),
			});

			if (!response.ok) {
				window.showToast?.(`Preview failed (${response.status}).`, 'error');
				return;
			}

			const html = await response.text();
			const target = document.getElementById('page-content');
			target.innerHTML = html;
			target.classList.add('fade-in');
			window.executeScriptsIn?.(target);
			if (typeof htmx !== 'undefined') htmx.process(target);

			history.pushState({}, '', '/preview');
			document.title = 'Preview - EsportsCalendar';
		} catch (err) {
			window.showToast?.('Network error: ' + err.message, 'error');
		} finally {
			window.setButtonLoading?.(submitBtn, false);
		}
	}

	submitBtn.addEventListener('click', submitPreview, { signal });

	// Enter on this page submits unless focus is inside a search input (where
	// Enter is used to toggle the highlighted dropdown row). Auto-removed on
	// next swap via the abort controller.
	document.addEventListener('keydown', (e) => {
		if (e.key !== 'Enter') return;
		if (submitBtn.disabled) return;
		const id = (document.activeElement && document.activeElement.id) || '';
		if (id.startsWith('search-') || id.startsWith('search-teams-')) return;
		e.preventDefault();
		submitPreview();
	}, { signal });
})();
