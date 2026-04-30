(function () {
	const toggle = document.getElementById('theme-toggle');
	if (!toggle) return;

	const current = document.documentElement.getAttribute('data-theme') || 'light';
	toggle.checked = current === 'light';

	toggle.addEventListener('change', () => {
		const next = toggle.checked ? 'light' : 'dark';
		document.documentElement.setAttribute('data-theme', next);
		try {
			localStorage.setItem('theme', next);
		} catch {}
	});
})();
