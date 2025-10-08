// Index page form submission logic
(function() {
	const continueBtn = document.getElementById('continue-btn');
	const checkboxes = document.querySelectorAll('.game-checkbox');
	const form = document.getElementById('game-form');

	function updateButtonState() {
		const anyChecked = Array.from(checkboxes).some(cb => cb.checked);
		continueBtn.disabled = !anyChecked;
	}

	// Restore previously selected options from sessionStorage
	const savedSelections = sessionStorage.getItem('selectedGameOptions');
	if (savedSelections) {
		const selectedIds = JSON.parse(savedSelections);
		checkboxes.forEach(cb => {
			if (selectedIds.includes(cb.value)) {
				cb.checked = true;
			}
		});
	}

	// Update button state on checkbox change
	checkboxes.forEach(cb => {
		cb.addEventListener('change', updateButtonState);
	});

	// Initial check
	updateButtonState();

	// Handle Enter key to submit form if any checkbox is checked
	document.addEventListener('keydown', (e) => {
		if (e.key === 'Enter' && !continueBtn.disabled) {
			e.preventDefault();
			form.requestSubmit();
		}
	});

	// Handle form submission
	form.addEventListener('submit', async (e) => {
		e.preventDefault();

		// Get selected options
		const selectedOptions = Array.from(checkboxes)
			.filter(cb => cb.checked)
			.map(cb => cb.value);

		// Save selections to sessionStorage
		sessionStorage.setItem('selectedGameOptions', JSON.stringify(selectedOptions));

		console.log('Submitting with options:', selectedOptions);

		// Send POST request with JSON body
		try {
			const response = await fetch('/lts', {
				method: 'POST',
				headers: {
					'Content-Type': 'application/json'
				},
				body: JSON.stringify({ options: selectedOptions })
			});

			console.log('Response status:', response.status);

			if (response.ok) {
				const html = await response.text();

				// Replace entire page to update progress indicator
				document.open();
				document.write(html);
				document.close();

				// Update URL and title after document rewrite
				window.history.pushState({}, '', '/lts');
				document.title = 'Leagues & Teams - EsportsCalendar';
			} else {
				console.error('Request failed:', response.statusText);
				alert('Request failed: ' + response.statusText);
			}
		} catch (error) {
			console.error('Error:', error);
			alert('Error: ' + error.message);
		}
	});
})();
