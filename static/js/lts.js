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

		// Handle keyboard events
		document.addEventListener('keydown', (e) => {
			const activeElement = document.activeElement;
			const isSearchInput = activeElement && (
				activeElement.id && (
					activeElement.id.startsWith('search-') ||
					activeElement.id.startsWith('search-teams-')
				)
			);

			// Enter key to submit (only when NOT focused on search boxes)
			if (e.key === 'Enter' && !submitBtn.disabled && !isSearchInput) {
				e.preventDefault();
				submitBtn.click();
			}
		});

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
					if (badge.classList.contains('badge-primary') && badge.hasAttribute('data-league-id')) {
						const leagueId = parseInt(badge.getAttribute('data-league-id'));
						leagues.push(leagueId);
					} else if (badge.classList.contains('badge-secondary') && badge.hasAttribute('data-team-id')) {
						const teamId = parseInt(badge.getAttribute('data-team-id'));
						teams.push(teamId);
					}
				});

				// Get tier value from sessionStorage
				const savedKey = 'lts-selections-' + gameId;
				const savedData = sessionStorage.getItem(savedKey);
				let maxTier = 1; // Default to tier 1
				if (savedData) {
					try {
						const parsed = JSON.parse(savedData);
						if (parsed.maxTier !== undefined) {
							maxTier = parsed.maxTier;
						}
					} catch (e) {
						console.error('Failed to parse saved tier:', e);
					}
				}

				if (leagues.length > 0 || teams.length > 0) {
					// Sort leagues and teams numerically
					leagues.sort((a, b) => a - b);
					teams.sort((a, b) => a - b);

					selections[gameId] = {
						leagues: leagues,
						teams: teams,
						maxTier: maxTier
					};
				}
			});

			// Sort game IDs and create a sorted selections object
			const sortedSelections = {};
			Object.keys(selections).sort((a, b) => parseInt(a) - parseInt(b)).forEach(key => {
				sortedSelections[key] = selections[key];
			});

			console.log('Collected selections:', sortedSelections);

			// Check if any selections were made
			if (Object.keys(sortedSelections).length === 0) {
				alert('Please select at least one league or team before submitting.');
				return;
			}

			// Save selections to sessionStorage before navigating
			sessionStorage.setItem('preview-selections', JSON.stringify(sortedSelections));

			// Send POST request to /preview
			try {
				const response = await fetch('/preview', {
					method: 'POST',
					headers: {
						'Content-Type': 'application/json'
					},
					body: JSON.stringify(sortedSelections)
				});

				console.log('Response status:', response.status);

				if (response.ok) {
					const html = await response.text();

					// Store current theme before replacing document
					const currentTheme = localStorage.getItem('theme') || 'dark';
					console.log('Current theme before navigation:', currentTheme);

					// Replace entire page content
					document.open();
					document.write(html);
					document.close();

					// The BaseLayout's inline script will handle theme initialization
					// We don't need to do anything here

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
