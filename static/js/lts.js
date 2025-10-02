// LTS page - Submit selection for preview
(function() {
	console.log('lts.js loaded');

	function setupSubmitButton() {
		const submitBtn = document.getElementById('submit-selection-btn');
		console.log('Looking for submit button...', submitBtn);

		if (!submitBtn) {
			console.error('Submit button not found');
			// Try again after a short delay
			setTimeout(setupSubmitButton, 100);
			return;
		}

		console.log('Submit button found, attaching event listener');

		submitBtn.addEventListener('click', async (e) => {
			console.log('Submit button clicked', e);
			e.preventDefault();

			// Collect all selections from all game cards
			const gameCards = document.querySelectorAll('[data-game-id]');
			const selections = {};

			gameCards.forEach(card => {
				const gameId = card.getAttribute('data-game-id');
				const selectedContainer = card.querySelector(`#selected-combined-${gameId}`);

				if (!selectedContainer) {
					console.error('Selected container not found for game:', gameId);
					return;
				}

				// Get all badges (leagues and teams)
				const badges = selectedContainer.querySelectorAll('.badge');
				const leagues = [];
				const teams = [];

				badges.forEach(badge => {
					// Check if it's a league (badge-primary) or team (badge-secondary)
					if (badge.classList.contains('badge-primary')) {
						const text = badge.textContent.replace('✕', '').trim();
						leagues.push(text);
					} else if (badge.classList.contains('badge-secondary')) {
						const text = badge.textContent.replace('✕', '').trim();
						teams.push(text);
					}
				});

				if (leagues.length > 0 || teams.length > 0) {
					selections[gameId] = {
						leagues: leagues,
						teams: teams
					};
				}
			});

			console.log('Collected selections:', selections);

			// Check if any selections were made
			if (Object.keys(selections).length === 0) {
				alert('Please select at least one league or team before submitting.');
				return;
			}

			// Send POST request to /preview
			try {
				const response = await fetch('/preview', {
					method: 'POST',
					headers: {
						'Content-Type': 'application/json'
					},
					body: JSON.stringify(selections)
				});

				console.log('Response status:', response.status);

				if (response.ok) {
					const html = await response.text();
					// Replace entire page content
					document.open();
					document.write(html);
					document.close();
					window.history.pushState({}, '', '/preview');
				} else {
					console.error('Request failed:', response.statusText);
					alert('Failed to submit selections: ' + response.statusText);
				}
			} catch (error) {
				console.error('Error:', error);
				alert('Error: ' + error.message);
			}
		}, false);
	}

	// Call setup immediately and also wait for DOM ready
	setupSubmitButton();
})();
