// Bootstraps the "Select games" page. Re-runs each time the inner partial is
// loaded (whether via initial page render or HTMX swap).
(function init() {
	const form = document.getElementById('game-form');
	if (!form) return;

	const continueBtn = form.querySelector('#continue-btn');
	const checkboxes = form.querySelectorAll('.game-checkbox');
	const counter = document.getElementById('game-counter');

	function updateButtonState() {
		const checkedCount = Array.from(checkboxes).filter((cb) => cb.checked).length;
		continueBtn.disabled = checkedCount === 0;
		if (counter) counter.textContent = String(checkedCount).padStart(2, '0');
	}

	try {
		const saved = sessionStorage.getItem('selectedGameOptions');
		if (saved) {
			const ids = JSON.parse(saved);
			checkboxes.forEach((cb) => {
				if (ids.includes(cb.value)) cb.checked = true;
			});
		}
	} catch {}

	checkboxes.forEach((cb) => cb.addEventListener('change', updateButtonState));
	updateButtonState();

	// Save selections to sessionStorage before HTMX submits, so the user can
	// hit "Back to Options" later and see them restored.
	form.addEventListener('htmx:configRequest', () => {
		const selected = Array.from(checkboxes)
			.filter((cb) => cb.checked)
			.map((cb) => cb.value);
		sessionStorage.setItem('selectedGameOptions', JSON.stringify(selected));
	});

	// Enter inside the form submits when at least one option is checked.
	form.addEventListener('keydown', (e) => {
		if (e.key !== 'Enter' || continueBtn.disabled) return;
		e.preventDefault();
		form.requestSubmit(continueBtn);
	});
})();
