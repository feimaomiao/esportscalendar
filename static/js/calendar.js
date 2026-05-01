// Bootstraps the public /calendar page. Shares filter state with /fixtures
// (same `fixtures-selections-*` and `fixtures-toggled-games` sessionStorage
// keys), and adds month navigation + day-click modal on top.
(function init() {
	const STORAGE_PREFIX = 'fixtures-selections-';
	const TOGGLES_KEY = 'fixtures-toggled-games';
	const HIDE_SCORES_KEY = 'fixtures-hide-scores';
	const CURRENT_MONTH_KEY = 'calendar-current-month';
	const REFRESH_DEBOUNCE_MS = 300;

	const cardsContainer = document.getElementById('calendar-game-cards');
	const contentEl = document.getElementById('calendar-content');
	const monthLabelEl = document.getElementById('calendar-month-label');
	const monthIsoEl = document.getElementById('calendar-month-iso');
	const prevBtn = document.getElementById('calendar-prev');
	const nextBtn = document.getElementById('calendar-next');
	const todayBtn = document.getElementById('calendar-today');
	const hideScoresEl = document.getElementById('calendar-hide-scores');
	const drawer = document.getElementById('calendar-drawer');
	const backdrop = document.getElementById('calendar-backdrop');
	const tuneOpenBtn = document.getElementById('calendar-tune-open');
	const tuneCloseBtn = document.getElementById('calendar-tune-close');
	const dayModal = document.getElementById('calendar-day-modal');
	const dayModalTitle = document.getElementById('calendar-day-modal-title');
	const dayModalBody = document.getElementById('calendar-day-modal-body');
	const dayModalClose = document.getElementById('calendar-day-modal-close');
	if (!cardsContainer || !contentEl || !monthIsoEl) return;
	if (!prevBtn || !nextBtn || !dayModal || !dayModalBody || !dayModalTitle) return;

	if (window.__calendarAbort) window.__calendarAbort.abort();
	const controller = new AbortController();
	window.__calendarAbort = controller;
	const { signal } = controller;

	const cardWraps = Array.from(cardsContainer.querySelectorAll('[data-calendar-card]'));
	const initialized = new Set();
	let lastDayTrigger = null;

	let currentYear = parseInt(monthIsoEl.dataset.year, 10);
	let currentMonth = parseInt(monthIsoEl.dataset.month, 10);
	const minYear = parseInt(prevBtn.dataset.minYear, 10);
	const minMonth = parseInt(prevBtn.dataset.minMonth, 10);
	const maxYear = parseInt(nextBtn.dataset.maxYear, 10);
	const maxMonth = parseInt(nextBtn.dataset.maxMonth, 10);

	// Restore last-viewed month if user revisits within the session.
	try {
		const saved = sessionStorage.getItem(CURRENT_MONTH_KEY);
		if (saved) {
			const [y, m] = saved.split('-').map((s) => parseInt(s, 10));
			if (Number.isFinite(y) && Number.isFinite(m)) {
				const clamped = clampMonth(y, m);
				if (clamped.year !== currentYear || clamped.month !== currentMonth) {
					currentYear = clamped.year;
					currentMonth = clamped.month;
				}
			}
		}
	} catch {}

	function clampMonth(y, m) {
		const target = y * 12 + (m - 1);
		const min = minYear * 12 + (minMonth - 1);
		const max = maxYear * 12 + (maxMonth - 1);
		const clamped = Math.max(min, Math.min(max, target));
		return { year: Math.floor(clamped / 12), month: (clamped % 12) + 1 };
	}

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
		const wrap = cardsContainer.querySelector(`[data-calendar-card="${gameId}"]`);
		if (wrap) wrap.classList.toggle('is-off', !active);
		chipsForGame(gameId).forEach((btn) => {
			btn.setAttribute('data-active', String(active));
			btn.setAttribute('aria-pressed', String(active));
		});
		document.querySelectorAll(`input[data-calendar-switch][data-game-id="${gameId}"]`).forEach((sw) => {
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

	const persisted = loadToggleState();
	allGameIds().forEach((gameId) => {
		const active = persisted ? persisted[gameId] !== false : true;
		applyToggleVisibility(gameId, active);
	});

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
			const sw = e.target.closest('input[data-calendar-switch][data-game-id]');
			if (!sw) return;
			setActive(sw.getAttribute('data-game-id'), sw.checked);
		},
		{ signal },
	);

	if (hideScoresEl) {
		try {
			const saved = sessionStorage.getItem(HIDE_SCORES_KEY);
			if (saved === '0') hideScoresEl.checked = false;
		} catch {}
		hideScoresEl.addEventListener(
			'change',
			() => {
				try {
					sessionStorage.setItem(HIDE_SCORES_KEY, hideScoresEl.checked ? '1' : '0');
				} catch {}
				scheduleRefresh();
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
			if (e.key === 'Escape') {
				if (!dayModal?.hidden) {
					e.preventDefault();
					closeDayModal();
					return;
				}
				if (drawer?.classList.contains('is-open')) {
					e.preventDefault();
					closeDrawer();
				}
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
			if (leagues.length > 0 || teams.length > 0) {
				leagues = leagues.slice().sort((a, b) => a - b);
				teams = teams.slice().sort((a, b) => a - b);
				selections[gameId] = { leagues, teams, maxTier };
			}
		});
		return selections;
	}

	function updateNavButtons() {
		const target = currentYear * 12 + (currentMonth - 1);
		const min = minYear * 12 + (minMonth - 1);
		const max = maxYear * 12 + (maxMonth - 1);
		prevBtn.disabled = target <= min;
		nextBtn.disabled = target >= max;
	}

	function setMonthLabel() {
		const date = new Date(currentYear, currentMonth - 1, 1);
		monthLabelEl.textContent = date.toLocaleDateString(undefined, { month: 'long', year: 'numeric' });
		monthIsoEl.textContent = `${currentYear}-${String(currentMonth).padStart(2, '0')}`;
		monthIsoEl.dataset.year = String(currentYear);
		monthIsoEl.dataset.month = String(currentMonth);
		try {
			sessionStorage.setItem(CURRENT_MONTH_KEY, `${currentYear}-${currentMonth}`);
		} catch {}
		updateNavButtons();
	}

	let inflight = null;
	async function refresh() {
		const selections = collectSelections();
		if (Object.keys(selections).length === 0) {
			contentEl.innerHTML = `
				<div class="border border-warning/40 bg-warning/10 px-4 py-3 rounded-sm flex items-center gap-3" role="alert">
					<span class="hud-mono text-sm">Pick at least one game with a league or team to see the calendar.</span>
				</div>`;
			return;
		}
		if (inflight) inflight.abort();
		const reqController = new AbortController();
		inflight = reqController;

		contentEl.classList.add('opacity-60');
		try {
			const response = await fetch('/api/calendar', {
				method: 'POST',
				headers: { 'Content-Type': 'application/json' },
				body: JSON.stringify({
					selections,
					year: currentYear,
					month: currentMonth,
					hideScores: !!(hideScoresEl && hideScoresEl.checked),
				}),
				signal: reqController.signal,
			});
			if (!response.ok) {
				window.showToast?.(`Calendar failed (${response.status}).`, 'error');
				return;
			}
			const html = await response.text();
			contentEl.innerHTML = html;
			window.convertMatchTimesIn?.(contentEl);
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
	let suppressRefresh = true;
	function scheduleRefresh() {
		if (suppressRefresh) return;
		clearTimeout(refreshTimer);
		refreshTimer = setTimeout(refresh, REFRESH_DEBOUNCE_MS);
	}

	function navigateMonth(delta) {
		const total = currentYear * 12 + (currentMonth - 1) + delta;
		const min = minYear * 12 + (minMonth - 1);
		const max = maxYear * 12 + (maxMonth - 1);
		if (total < min || total > max) return;
		currentYear = Math.floor(total / 12);
		currentMonth = (total % 12) + 1;
		setMonthLabel();
		// Force refresh even during initial suppression — month nav is explicit.
		clearTimeout(refreshTimer);
		refresh();
	}
	prevBtn.addEventListener('click', () => navigateMonth(-1), { signal });
	nextBtn.addEventListener('click', () => navigateMonth(1), { signal });
	todayBtn?.addEventListener(
		'click',
		() => {
			const now = new Date();
			const clamped = clampMonth(now.getFullYear(), now.getMonth() + 1);
			currentYear = clamped.year;
			currentMonth = clamped.month;
			setMonthLabel();
			clearTimeout(refreshTimer);
			refresh();
		},
		{ signal },
	);

	const exportBtn = document.getElementById('calendar-export-btn');
	exportBtn?.addEventListener(
		'click',
		() => {
			const selections = collectSelections();
			if (Object.keys(selections).length === 0) {
				window.showToast?.('Pick at least one league or team before exporting.', 'warning');
				return;
			}
			const hideScores = !!(hideScoresEl && hideScoresEl.checked);
			window.exportCalendarPayload?.({ selections, hideScores }, exportBtn);
		},
		{ signal },
	);

	function openDayModal(dateKey) {
		const tpl = contentEl.querySelector(`template[data-day-modal-content="${dateKey}"]`);
		if (!tpl) return;
		dayModalBody.innerHTML = '';
		dayModalBody.appendChild(tpl.content.cloneNode(true));
		const date = new Date(`${dateKey}T00:00:00`);
		dayModalTitle.textContent = date.toLocaleDateString(undefined, {
			weekday: 'long',
			month: 'long',
			day: 'numeric',
			year: 'numeric',
		});
		dayModal.hidden = false;
		document.body.classList.add('drawer-open');
		window.convertMatchTimesIn?.(dayModalBody);
		setTimeout(() => dayModalClose?.focus(), 50);
	}
	function closeDayModal() {
		if (!dayModal) return;
		dayModal.hidden = true;
		dayModalBody.innerHTML = '';
		document.body.classList.remove('drawer-open');
		if (lastDayTrigger && document.body.contains(lastDayTrigger)) {
			lastDayTrigger.focus();
		}
		lastDayTrigger = null;
	}
	dayModalClose?.addEventListener('click', closeDayModal, { signal });
	dayModal?.addEventListener(
		'click',
		(e) => {
			if (e.target === dayModal) closeDayModal();
		},
		{ signal },
	);
	contentEl.addEventListener(
		'click',
		(e) => {
			const cell = e.target.closest('.hud-cal-day[data-date]');
			if (!cell || cell.disabled) return;
			lastDayTrigger = cell;
			openDayModal(cell.getAttribute('data-date'));
		},
		{ signal },
	);

	function hasPriorState() {
		try {
			if (sessionStorage.getItem(TOGGLES_KEY)) return true;
			if (sessionStorage.getItem(HIDE_SCORES_KEY)) return true;
			for (let i = 0; i < sessionStorage.length; i++) {
				const key = sessionStorage.key(i);
				if (key && key.startsWith(STORAGE_PREFIX)) return true;
			}
		} catch {}
		return false;
	}

	setMonthLabel();

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

		const localize = () => window.convertMatchTimesIn?.(contentEl);
		if (document.readyState === 'loading') {
			document.addEventListener('DOMContentLoaded', localize, { once: true, signal });
		} else {
			localize();
		}

		// Same suppression strategy as fixtures.js: cold load already matches
		// the server-rendered grid, so we wait briefly to swallow tier-1 cascades;
		// warm load (with prior state) needs an immediate re-fetch to reflect it.
		// If the user navigated to a month other than the server-rendered one
		// (via CURRENT_MONTH_KEY), force a refresh regardless.
		const initialYear = parseInt(monthIsoEl.dataset.year, 10);
		const initialMonth = parseInt(monthIsoEl.dataset.month, 10);
		if (currentYear !== initialYear || currentMonth !== initialMonth) {
			suppressRefresh = false;
			refresh();
		} else if (hasPriorState()) {
			suppressRefresh = false;
		} else {
			setTimeout(() => {
				suppressRefresh = false;
			}, 1500);
		}
	});
})();
