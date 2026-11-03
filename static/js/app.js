// Shared site bootstrap. Loaded on every page (defer).

function applyLabelCheckedState(checkbox) {
	const label = checkbox.closest('label');
	if (!label) return;
	label.classList.toggle('label-checked', checkbox.checked);
}

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
});
