// Shared site bootstrap. Loaded on every page (defer).

function applyLabelCheckedState(checkbox) {
	const label = checkbox.closest('label');
	if (!label) return;
	label.classList.toggle('label-checked', checkbox.checked);
}

// Re-execute <script> tags inside a container after an innerHTML swap.
// innerHTML insertion does not run scripts (per HTML spec), so we have to
// recreate each script element to trigger evaluation.
//
// Defense-in-depth: only inline scripts and same-origin /static/ scripts are
// re-evaluated. A compromised template that injected a third-party src would
// be silently dropped here.
window.executeScriptsIn = function executeScriptsIn(container) {
	if (!container) return;
	container.querySelectorAll('script').forEach((oldScript) => {
		const src = oldScript.getAttribute('src');
		if (src) {
			let url;
			try {
				url = new URL(src, window.location.origin);
			} catch {
				oldScript.remove();
				return;
			}
			const sameOrigin = url.origin === window.location.origin;
			const allowedPath = url.pathname.startsWith('/static/');
			if (!sameOrigin || !allowedPath) {
				oldScript.remove();
				return;
			}
		}
		const newScript = document.createElement('script');
		for (const attr of oldScript.attributes) {
			newScript.setAttribute(attr.name, attr.value);
		}
		newScript.textContent = oldScript.textContent;
		oldScript.parentNode.replaceChild(newScript, oldScript);
	});
};

// Convert any `.match-time[data-utc-time]` nodes inside `root` from server-side
// UTC to the viewer's local timezone. Both /fixtures and /calendar emit the same
// markup; this lives here so they share the formatting and can re-run after
// each innerHTML swap.
window.convertMatchTimesIn = function convertMatchTimesIn(root) {
	const scope = root || document;
	const dateOpts = { month: 'short', day: '2-digit', year: 'numeric' };
	const timeOpts = { hour: '2-digit', minute: '2-digit', hour12: false };
	scope.querySelectorAll('.match-time[data-utc-time]').forEach((el) => {
		if (el.dataset.localized === '1') return;
		const utc = el.getAttribute('data-utc-time');
		const date = new Date(utc);
		if (!utc || isNaN(date.getTime())) return;
		const dateSpan = el.querySelector('.match-date');
		const hourSpan = el.querySelector('.match-hour');
		if (dateSpan) dateSpan.textContent = date.toLocaleDateString(undefined, dateOpts);
		if (hourSpan) hourSpan.textContent = date.toLocaleTimeString(undefined, timeOpts);
		el.dataset.localized = '1';
	});
};

// Show/hide a small spinner inside a button while an async action is pending.
// DaisyUI v5 expects a `.loading` span inside the button rather than a class
// on the button itself.
window.setButtonLoading = function setButtonLoading(btn, isLoading) {
	if (!btn) return;
	if (isLoading) {
		if (!btn.dataset.originalContent) {
			btn.dataset.originalContent = btn.innerHTML;
		}
		btn.disabled = true;
		btn.innerHTML = '<span class="loading loading-spinner loading-sm"></span>';
	} else {
		if (btn.dataset.originalContent) {
			btn.innerHTML = btn.dataset.originalContent;
			delete btn.dataset.originalContent;
		}
		btn.disabled = false;
	}
};

function bindCheckboxLabels(root) {
	root.querySelectorAll('input[type="checkbox"]').forEach((cb) => {
		applyLabelCheckedState(cb);
		cb.addEventListener('change', () => applyLabelCheckedState(cb));
	});
}

// Delegated handler for inline match expansion. The trigger lives inside
// HUDMatchRow (`[data-match-toggle]`) and controls the sibling
// `.hud-match-detail` panel. Delegation lets it work on server-rendered
// fixtures, calendar day-modal content, and any future swap target.
function toggleMatchDetail(trigger) {
	const wrapper = trigger.closest('.hud-match-wrapper');
	if (!wrapper) return;
	const detail = wrapper.querySelector('.hud-match-detail');
	if (!detail) return;
	const isExpanded = trigger.getAttribute('aria-expanded') === 'true';
	trigger.setAttribute('aria-expanded', String(!isExpanded));
	detail.hidden = isExpanded;
}
document.addEventListener('click', (e) => {
	const trigger = e.target.closest('[data-match-toggle]');
	if (!trigger) return;
	// Ignore clicks on links or other interactive children.
	if (e.target.closest('a, button:not([data-match-toggle])')) return;
	toggleMatchDetail(trigger);
});
document.addEventListener('keydown', (e) => {
	if (e.key !== 'Enter' && e.key !== ' ') return;
	const trigger = e.target.closest('[data-match-toggle]');
	if (!trigger || trigger !== e.target) return;
	e.preventDefault();
	toggleMatchDetail(trigger);
});

document.addEventListener('DOMContentLoaded', () => {
	bindCheckboxLabels(document);

	if (typeof htmx === 'undefined') return;

	htmx.config.defaultSwapStyle = 'innerHTML';
	// Disable HTMX history cache. Restoring a stale full-body snapshot on top
	// of a fresh swap was causing duplicate footers/navs after navigation.
	htmx.config.historyCacheSize = 0;
	htmx.config.refreshOnHistoryMiss = true;
	try {
		sessionStorage.removeItem('htmx-history-cache');
	} catch {}

	htmx.on('htmx:afterSwap', (e) => {
		const target = e.detail.target;
		if (target) {
			target.classList.add('fade-in');
			bindCheckboxLabels(target);
		}
	});

	htmx.on('htmx:responseError', (e) => {
		const status = e.detail.xhr ? e.detail.xhr.status : '?';
		window.showToast?.(`Request failed (${status}). Please try again.`, 'error');
	});

	htmx.on('htmx:sendError', () => {
		window.showToast?.('Network error. Please check your connection.', 'error');
	});

	htmx.on('page:title', (e) => {
		const t = e.detail && e.detail.title;
		if (t) document.title = t;
	});
});
