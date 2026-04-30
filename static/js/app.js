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
