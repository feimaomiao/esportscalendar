// Index page form submission logic
(function() {
	const continueBtn = document.getElementById('continue-btn');
	const checkboxes = document.querySelectorAll('.game-checkbox');
	const form = document.getElementById('game-form');

	function updateButtonState() {
		const anyChecked = Array.from(checkboxes).some(cb => cb.checked);
		continueBtn.disabled = !anyChecked;
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

		console.log('Submitting with options:', selectedOptions);

		// Send POST request with JSON body
		try {
			const response = await fetch('/lts', {
				method: 'POST',
				headers: {
					'Content-Type': 'application/json',
					'HX-Request': 'true'
				},
				body: JSON.stringify({ options: selectedOptions })
			});

			console.log('Response status:', response.status);

			if (response.ok) {
				const html = await response.text();
				const container = document.getElementById('select');
				container.innerHTML = html;

				// Execute scripts in the newly inserted HTML
				const scripts = container.querySelectorAll('script');
				scripts.forEach(oldScript => {
					const newScript = document.createElement('script');
					newScript.textContent = oldScript.textContent;
					oldScript.parentNode.replaceChild(newScript, oldScript);
				});

				window.history.pushState({}, '', '/lts');
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
