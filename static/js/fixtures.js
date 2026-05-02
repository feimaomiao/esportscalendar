// Bootstraps the public /fixtures page. Initializes game-selection cards
// (storage prefix `fixtures-selections-`) and refetches the match list
// (debounced) whenever the user touches a chip, dropdown, or tier slider.
(function init() {
	const STORAGE_PREFIX = 'fixtures-selections-';
	const TOGGLES_KEY = 'fixtures-toggled-games';
	const REFRESH_DEBOUNCE_MS = 300;

	const cardsContainer = document.getElementById('fixtures-game-cards');
	const contentEl = document.getElementById('fixtures-content');
	const hideScoresEl = document.getElementById('fixtures-hide-scores');
	const drawer = document.getElementById('fixtures-drawer');
	const backdrop = document.getElementById('fixtures-backdrop');
	const tuneOpenBtn = document.getElementById('fixtures-tune-open');
	const tuneCloseBtn = document.getElementById('fixtures-tune-close');
	if (!cardsContainer || !contentEl) return;

	if (window.__fixturesAbort) window.__fixturesAbort.abort();
	const controller = new AbortController();
	window.__fixturesAbort = controller;
	const { signal } = controller;

	const cardWraps = Array.from(cardsContainer.querySelectorAll('[data-fixtures-card]'));
	const initialized = new Set();

	function chipsForGame(gameId) {
		return Array.from(document.querySelectorAll(`.hud-game-chip[data-game-id="${gameId}"]`));
	}
	function allGameIds() {
		const ids = new Set();
		document.querySelectorAll('.hud-game-chip[data-game-id]').forEach((btn) => {
			ids.add(btn.getAttribute('data-game-id'));
		});
		return Array.from(ids);
	}

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

	function loadToggleState() {
		try {
			const raw = sessionStorage.getItem(TOGGLES_KEY);
			if (!raw) return null;
			const parsed = JSON.parse(raw);
			if (parsed && typeof parsed === 'object') return parsed;
		} catch {}
		return null;
	}

	function saveToggleState() {
		const state = {};
		allGameIds().forEach((gameId) => {
			const sample = chipsForGame(gameId)[0];
			state[gameId] = sample ? sample.getAttribute('data-active') === 'true' : true;
		});
		try {
			sessionStorage.setItem(TOGGLES_KEY, JSON.stringify(state));
		} catch {}
	}

	function applyToggleVisibility(gameId, active) {
		const wrap = cardsContainer.querySelector(`[data-fixtures-card="${gameId}"]`);
		if (wrap) wrap.classList.toggle('is-off', !active);
		chipsForGame(gameId).forEach((btn) => {
			btn.setAttribute('data-active', String(active));
			btn.setAttribute('aria-pressed', String(active));
		});
		document.querySelectorAll(`input[data-fixtures-switch][data-game-id="${gameId}"]`).forEach((sw) => {
			if (sw.checked !== active) sw.checked = active;
		});
		const label = wrap?.querySelector('.hud-fixtures-card-switch-label');
		if (label) label.textContent = active ? 'ON' : 'OFF';
	}

	function setActive(gameId, active) {
		applyToggleVisibility(gameId, active);
		if (active && !initialized.has(gameId)) {
			initGameSelection(gameId, STORAGE_PREFIX);
			initialized.add(gameId);
		}
		saveToggleState();
		scheduleRefresh();
	}

	// Apply persisted toggle state. If no saved state, default all-on so first
	// visit shows matches (initGameSelection auto-picks tier-1 leagues).
	const persisted = loadToggleState();
	allGameIds().forEach((gameId) => {
		const active = persisted ? persisted[gameId] !== false : true;
		applyToggleVisibility(gameId, active);
	});

	// Two toggle surfaces share one path. Chips dispatch on click (button), the
	// per-card switches dispatch on change (input). Both feed setActive, which
	// then mirrors state across every chip and switch for that gameId.
	document.addEventListener(
		'click',
		(e) => {
			const chip = e.target.closest('.hud-game-chip[data-game-id]');
			if (!chip) return;
			const gameId = chip.getAttribute('data-game-id');
			const active = chip.getAttribute('data-active') !== 'true';
			setActive(gameId, active);
		},
		{ signal },
	);
	document.addEventListener(
		'change',
		(e) => {
			const sw = e.target.closest('input[data-fixtures-switch][data-game-id]');
			if (!sw) return;
			setActive(sw.getAttribute('data-game-id'), sw.checked);
		},
		{ signal },
	);

	// Spoiler block defaults to ON. Server already renders with scores hidden,
	// so we only flip the checkbox if the user previously turned it off.
	if (hideScoresEl) {
		try {
			const saved = sessionStorage.getItem('fixtures-hide-scores');
			if (saved === '0') hideScoresEl.checked = false;
		} catch {}
		hideScoresEl.addEventListener(
			'change',
			() => {
				try {
					sessionStorage.setItem('fixtures-hide-scores', hideScoresEl.checked ? '1' : '0');
				} catch {}
				// Spoiler is a presentational toggle — the visible match list
				// doesn't change, only score vs "Final" rendering. Skip the
				// scroll-to-ongoing reflow so the user stays where they are.
				scheduleRefresh({ preserveScroll: true });
			},
			{ signal },
		);
	}

	function openDrawer() {
		if (!drawer) return;
		drawer.setAttribute('aria-hidden', 'false');
		drawer.classList.add('is-open');
		if (backdrop) {
			backdrop.hidden = false;
			requestAnimationFrame(() => backdrop.classList.add('is-open'));
		}
		document.body.classList.add('drawer-open');
		// Lazy-init any cards belonging to active games but not yet initialized.
		// These exist if the user toggles a game on while the drawer is closed
		// — we want the card ready by the time they look at it.
		activeGameIds().forEach((gameId) => {
			if (!initialized.has(gameId)) {
				initGameSelection(gameId, STORAGE_PREFIX);
				initialized.add(gameId);
			}
		});
		setTimeout(() => tuneCloseBtn?.focus(), 50);
	}

	function closeDrawer() {
		if (!drawer) return;
		drawer.setAttribute('aria-hidden', 'true');
		drawer.classList.remove('is-open');
		if (backdrop) {
			backdrop.classList.remove('is-open');
			setTimeout(() => {
				backdrop.hidden = true;
			}, 200);
		}
		document.body.classList.remove('drawer-open');
		tuneOpenBtn?.focus();
	}

	tuneOpenBtn?.addEventListener('click', openDrawer, { signal });
	tuneCloseBtn?.addEventListener('click', closeDrawer, { signal });
	backdrop?.addEventListener('click', closeDrawer, { signal });
	document.addEventListener(
		'keydown',
		(e) => {
			if (e.key === 'Escape' && drawer?.classList.contains('is-open')) {
				e.preventDefault();
				closeDrawer();
			}
		},
		{ signal },
	);

	function activeGameIds() {
		return allGameIds().filter((gameId) => {
			const sample = chipsForGame(gameId)[0];
			return sample && sample.getAttribute('data-active') === 'true';
		});
	}

	function collectSelections() {
		const selections = {};
		activeGameIds().forEach((gameId) => {
			let leagues = [];
			let teams = [];
			let maxTier = 2;
			try {
				const raw = sessionStorage.getItem(STORAGE_PREFIX + gameId);
				if (raw) {
					const parsed = JSON.parse(raw);
					if (Array.isArray(parsed.leagues)) leagues = parsed.leagues;
					if (Array.isArray(parsed.teams)) teams = parsed.teams;
					if (typeof parsed.maxTier === 'number') maxTier = parsed.maxTier;
				}
			} catch {}
			// Include the game if it contributes any rows: explicit league/team
			// picks, or tier auto-include (maxTier > 0). A game with neither is
			// a no-op for the server query, so skip it to keep the payload tight.
			if (leagues.length > 0 || teams.length > 0 || maxTier > 0) {
				leagues = leagues.slice().sort((a, b) => a - b);
				teams = teams.slice().sort((a, b) => a - b);
				selections[gameId] = { leagues, teams, maxTier };
			}
		});
		return selections;
	}

	function scrollToUpcoming(behavior) {
		// The server places dividers between sections: past/ongoing and ongoing/upcoming.
		// Prefer scrolling to the ongoing divider (so currently-live matches are
		// at the top); fall back to the upcoming divider if no ongoing matches
		// exist; otherwise the first match.
		const dividers = Array.from(contentEl.querySelectorAll('.hud-now-divider'));
		let target = null;
		if (dividers.length > 0) {
			const ongoingDivider = dividers.find((d) => d.textContent.includes('ongoing'));
			const upcomingDivider = dividers.find((d) => d.textContent.includes('upcoming'));
			target = ongoingDivider || upcomingDivider || dividers[0];
		}
		target = target || contentEl.firstElementChild;
		if (!target) return;
		const navOffset = 64; // .navbar (fixed)
		const barEl = document.querySelector('.hud-control-bar');
		const barOffset = barEl ? barEl.getBoundingClientRect().height + 12 : 80;
		const top = target.getBoundingClientRect().top + window.scrollY - navOffset - barOffset;
		window.scrollTo({ top: Math.max(0, top), behavior: behavior || 'auto' });
	}

	let inflight = null;
	async function refresh(opts) {
		const preserveScroll = !!(opts && opts.preserveScroll);
		const selections = collectSelections();
		if (Object.keys(selections).length === 0) {
			contentEl.innerHTML = `
				<div class="border border-warning/40 bg-warning/10 px-4 py-3 rounded-sm flex items-center gap-3" role="alert">
					<span class="hud-mono text-sm">Pick at least one game with a league or team to see the fixtures.</span>
				</div>`;
			return;
		}

		if (inflight) inflight.abort();
		const reqController = new AbortController();
		inflight = reqController;

		contentEl.classList.add('opacity-60');
		try {
			const response = await fetch('/api/fixtures', {
				method: 'POST',
				headers: { 'Content-Type': 'application/json' },
				body: JSON.stringify({ selections, hideScores: !!(hideScoresEl && hideScoresEl.checked) }),
				signal: reqController.signal,
			});
			if (!response.ok) {
				window.showToast?.(`Fixtures failed (${response.status}).`, 'error');
				return;
			}
			const html = await response.text();
			contentEl.innerHTML = html;
			window.convertMatchTimesIn?.(contentEl);
			if (!preserveScroll) scrollToUpcoming('smooth');
		} catch (err) {
			if (err.name !== 'AbortError') {
				window.showToast?.('Network error: ' + err.message, 'error');
			}
		} finally {
			contentEl.classList.remove('opacity-60');
			if (inflight === reqController) inflight = null;
		}
	}

	let refreshTimer = null;
	let suppressRefresh = true; // suppress during hydration
	function scheduleRefresh(opts) {
		if (suppressRefresh) return;
		clearTimeout(refreshTimer);
		refreshTimer = setTimeout(() => refresh(opts), REFRESH_DEBOUNCE_MS);
	}

	function hasPriorState() {
		try {
			if (sessionStorage.getItem(TOGGLES_KEY)) return true;
			if (sessionStorage.getItem('fixtures-hide-scores')) return true;
			for (let i = 0; i < sessionStorage.length; i++) {
				const key = sessionStorage.key(i);
				if (key && key.startsWith(STORAGE_PREFIX)) return true;
			}
		} catch {}
		return false;
	}

	ensureGameSelectionThen(() => {
		activeGameIds().forEach((gameId) => {
			initGameSelection(gameId, STORAGE_PREFIX);
			initialized.add(gameId);
		});
		const observer = new MutationObserver(() => scheduleRefresh());
		cardWraps.forEach((wrap) => {
			observer.observe(wrap, { subtree: true, childList: true });
		});
		controller.signal.addEventListener('abort', () => observer.disconnect());

		cardsContainer.addEventListener(
			'input',
			(e) => {
				if (e.target && e.target.matches('input[type="range"], input[type="checkbox"]')) {
					scheduleRefresh();
				}
			},
			{ signal },
		);

		// app.js is deferred, so on cold load it hasn't defined
		// convertMatchTimesIn yet when this IIFE runs. Wait for DOMContentLoaded
		// in that case; on htmx swaps readyState is already 'complete'.
		const localizeAndScroll = () => {
			window.convertMatchTimesIn?.(contentEl);
			scrollToUpcoming('auto');
		};
		if (document.readyState === 'loading') {
			document.addEventListener('DOMContentLoaded', localizeAndScroll, { once: true, signal });
		} else {
			localizeAndScroll();
		}

		// On warm visits the MutationObserver fires when initGameSelection
		// repopulates badges from sessionStorage — that single debounced refresh
		// is enough to apply saved filters. On cold visits the server already
		// rendered the matching default state, so we hold suppression long
		// enough to swallow the tier-1 auto-select cascade and skip a wasted
		// roundtrip.
		if (hasPriorState()) {
			suppressRefresh = false;
		} else {
			setTimeout(() => {
				suppressRefresh = false;
			}, 1500);
		}
	});
})();
