// Check if any selections are made and enable/disable submit button
function checkAndUpdateSubmitButton() {
	const gameCards = document.querySelectorAll('[data-game-id]');
	let hasSelections = false;

	gameCards.forEach(card => {
		const gameId = card.getAttribute('data-game-id');
		const selectedContainer = document.getElementById('selected-combined-' + gameId);
		if (selectedContainer && selectedContainer.querySelectorAll('.badge').length > 0) {
			hasSelections = true;
		}
	});

	const submitBtn = document.getElementById('submit-selection-btn');
	if (submitBtn) {
		submitBtn.disabled = !hasSelections;
	}
}

// Game selection - League and Team management
function initGameSelection(gameId) {
	console.log('initGameSelection called with gameId:', gameId, 'type:', typeof gameId);

	// Validate gameId
	if (!gameId || gameId === 'null' || gameId === 'undefined') {
		console.error('Invalid gameId:', gameId);
		return;
	}

	const searchInput = document.getElementById('search-' + gameId);
	const dropdownMenu = document.getElementById('dropdown-menu-' + gameId);
	const loadingElement = document.getElementById('loading-' + gameId);
	const leagueList = document.getElementById('league-list-' + gameId);
	const noResults = document.getElementById('no-results-' + gameId);
	const selectedCombinedContainer = document.getElementById('selected-combined-' + gameId);

	console.log('Elements found:', {
		searchInput: !!searchInput,
		dropdownMenu: !!dropdownMenu,
		loadingElement: !!loadingElement,
		leagueList: !!leagueList,
		noResults: !!noResults,
		selectedCombinedContainer: !!selectedCombinedContainer
	});

	if (!searchInput || !dropdownMenu || !loadingElement) {
		console.error('Required elements not found for gameId:', gameId);
		return;
	}

	let allLeagues = [];
	let selectedLeagues = new Set();
	let currentFilteredLeagues = [];
	let maxTier = 2; // Default to tier A (tier 2)

	// Restore saved selections from sessionStorage
	const savedKey = 'lts-selections-' + gameId;
	const savedData = sessionStorage.getItem(savedKey);
	let hasSavedSelections = false;
	if (savedData) {
		try {
			const parsed = JSON.parse(savedData);
			console.log('Restoring saved selections for game', gameId, ':', parsed);
			if (parsed.leagues && Array.isArray(parsed.leagues)) {
				parsed.leagues.forEach(id => selectedLeagues.add(id));
				hasSavedSelections = true;
			}
			if (parsed.maxTier !== undefined) {
				maxTier = parsed.maxTier;
			}
		} catch (e) {
			console.error('Failed to parse saved selections:', e);
		}
	}

	// Fetch leagues from API
	const apiUrl = '/api/league-options/' + gameId;
	console.log('Fetching leagues from:', apiUrl);
	fetch(apiUrl)
		.then(response => response.json())
		.then(data => {
			loadingElement.classList.add('hidden');

			if (data.error) {
				noResults.textContent = data.message;
				noResults.classList.remove('hidden');
			} else if (data.leagues && data.leagues.length > 0) {
				allLeagues = data.leagues;
				currentFilteredLeagues = data.leagues;

				// Auto-select tier 1 leagues only if no saved selections
				if (!hasSavedSelections) {
					allLeagues.forEach(league => {
						if (league.is_tier1) {
							selectedLeagues.add(league.id);
						}
					});
					// Save the auto-selected tier 1 leagues
					if (selectedLeagues.size > 0) {
						saveSelections();
					}
				}

				renderLeagues(allLeagues);
				updateCombinedDisplay();
				leagueList.classList.remove('hidden');
			} else {
				noResults.textContent = 'No leagues available for this game';
				noResults.classList.remove('hidden');
			}
		})
		.catch(error => {
			loadingElement.classList.add('hidden');
			noResults.textContent = 'Failed to load leagues. Please try again.';
			noResults.classList.remove('hidden');
			console.error('Error fetching leagues:', error);
		});

	// Render leagues list - optimized
	let highlightedLeagueIndex = -1;
	function renderLeagues(leagues) {
		const html = leagues.map((league, index) => `
			<li data-index="${index}">
				<label class="label cursor-pointer justify-start gap-2 p-2">
					<input type="checkbox" class="checkbox checkbox-sm checkbox-primary" ${selectedLeagues.has(league.id) ? 'checked' : ''} onchange="toggleLeague(${league.id})">
					<div class="item-icon-container">
						<img src="${league.image || '/static/images/default-logo.png'}" alt="${league.name}" class="item-icon" onerror="this.src='/static/images/default-logo.png'">
					</div>
					<span class="text-sm">${league.name}</span>
				</label>
			</li>
		`).join('');
		leagueList.innerHTML = html;
		highlightedLeagueIndex = -1;
		// Re-bind toggle functions
		leagueList.querySelectorAll('input[type="checkbox"]').forEach((cb, i) => {
			cb.addEventListener('change', () => toggleLeague(leagues[i]));
		});
	}

	// Save selections to sessionStorage
	function saveSelections() {
		const data = {
			leagues: Array.from(selectedLeagues),
			teams: Array.from(selectedTeams),
			maxTier: maxTier
		};
		console.log('Saving selections for game', gameId, ':', data);
		sessionStorage.setItem(savedKey, JSON.stringify(data));
	}

	// Toggle league selection
	function toggleLeague(league) {
		if (selectedLeagues.has(league.id)) {
			selectedLeagues.delete(league.id);
		} else {
			selectedLeagues.add(league.id);
		}
		saveSelections();
		updateCombinedDisplay();
	}

	// Filter leagues based on search input
	function filterLeagues(query) {
		const lowerQuery = query.toLowerCase();
		return allLeagues.filter(league =>
			league.name.toLowerCase().includes(lowerQuery)
		);
	}

	// Search input event
	searchInput.addEventListener('input', (e) => {
		const filtered = filterLeagues(e.target.value);
		currentFilteredLeagues = filtered;
		highlightedLeagueIndex = -1;
		if (filtered.length > 0) {
			renderLeagues(filtered);
			leagueList.classList.remove('hidden');
			noResults.classList.add('hidden');
		} else {
			leagueList.classList.add('hidden');
			noResults.classList.remove('hidden');
		}
	});

	// Handle arrow keys and Enter for keyboard navigation
	function highlightLeagueItem(index) {
		const items = leagueList.querySelectorAll('li');
		items.forEach((item, i) => {
			if (i === index) {
				item.querySelector('label').style.backgroundColor = 'oklch(var(--b3))';
			} else {
				item.querySelector('label').style.backgroundColor = '';
			}
		});
	}

	searchInput.addEventListener('keydown', (e) => {
		if (e.key === 'Escape') {
			e.preventDefault();
			dropdownMenu.style.display = 'none';
			searchInput.blur();
		} else if (e.key === 'ArrowDown') {
			e.preventDefault();
			const items = leagueList.querySelectorAll('li');
			if (items.length > 0) {
				highlightedLeagueIndex = highlightedLeagueIndex + 1;
				if (highlightedLeagueIndex >= items.length) {
					highlightedLeagueIndex = 0;
				}
				highlightLeagueItem(highlightedLeagueIndex);
				items[highlightedLeagueIndex].scrollIntoView({ block: 'nearest' });
			}
		} else if (e.key === 'ArrowUp') {
			e.preventDefault();
			const items = leagueList.querySelectorAll('li');
			if (items.length > 0) {
				highlightedLeagueIndex = highlightedLeagueIndex - 1;
				if (highlightedLeagueIndex < 0) {
					highlightedLeagueIndex = items.length - 1;
				}
				highlightLeagueItem(highlightedLeagueIndex);
				items[highlightedLeagueIndex].scrollIntoView({ block: 'nearest' });
			}
		} else if (e.key === 'Enter') {
			e.preventDefault();
			if (highlightedLeagueIndex >= 0 && highlightedLeagueIndex < currentFilteredLeagues.length) {
				const league = currentFilteredLeagues[highlightedLeagueIndex];
				toggleLeague(league);
				renderLeagues(currentFilteredLeagues);
				highlightLeagueItem(highlightedLeagueIndex);
			}
		}
	});

	// Show dropdown on focus
	searchInput.addEventListener('focus', () => {
		dropdownMenu.style.display = 'block';
	});

	// Hide dropdown on blur (when losing focus)
	searchInput.addEventListener('blur', () => {
		// Use setTimeout to allow click events on dropdown items to register first
		setTimeout(() => {
			dropdownMenu.style.display = 'none';
		}, 200);
	});

	// Hide dropdown when clicking outside
	document.addEventListener('click', (e) => {
		if (!searchInput.contains(e.target) && !dropdownMenu.contains(e.target)) {
			dropdownMenu.style.display = 'none';
		}
	});

	// TEAMS SECTION
	const searchTeamsInput = document.getElementById('search-teams-' + gameId);
	const dropdownTeamsMenu = document.getElementById('dropdown-teams-menu-' + gameId);
	const loadingTeamsElement = document.getElementById('loading-teams-' + gameId);
	const teamList = document.getElementById('team-list-' + gameId);
	const noTeamsResults = document.getElementById('no-teams-results-' + gameId);

	console.log('Team elements found:', {
		searchTeamsInput: !!searchTeamsInput,
		dropdownTeamsMenu: !!dropdownTeamsMenu,
		loadingTeamsElement: !!loadingTeamsElement,
		teamList: !!teamList,
		noTeamsResults: !!noTeamsResults
	});

	let allTeams = [];
	let selectedTeams = new Set();
	let currentFilteredTeams = [];

	// Restore saved team selections from sessionStorage
	if (savedData) {
		try {
			const parsed = JSON.parse(savedData);
			if (parsed.teams && Array.isArray(parsed.teams)) {
				parsed.teams.forEach(id => selectedTeams.add(id));
			}
		} catch (e) {
			console.error('Failed to parse saved team selections:', e);
		}
	}

	// Fetch teams from API
	const teamApiUrl = '/api/team-options/' + gameId;
	console.log('Fetching teams from:', teamApiUrl);
	fetch(teamApiUrl)
		.then(response => response.json())
		.then(data => {
			loadingTeamsElement.classList.add('hidden');

			if (data.error) {
				noTeamsResults.textContent = data.message;
				noTeamsResults.classList.remove('hidden');
			} else if (data.teams && data.teams.length > 0) {
				allTeams = data.teams;
				currentFilteredTeams = data.teams;
				renderTeams(allTeams);
				updateCombinedDisplay();
				teamList.classList.remove('hidden');
			} else {
				noTeamsResults.textContent = 'No teams available for this game';
				noTeamsResults.classList.remove('hidden');
			}
		})
		.catch(error => {
			loadingTeamsElement.classList.add('hidden');
			noTeamsResults.textContent = 'Failed to load teams. Please try again.';
			noTeamsResults.classList.remove('hidden');
			console.error('Error fetching teams:', error);
		});

	// Render teams list - optimized
	let highlightedTeamIndex = -1;
	function renderTeams(teams) {
		const html = teams.map((team, index) => `
			<li data-index="${index}">
				<label class="label cursor-pointer justify-start gap-2 p-2">
					<input type="checkbox" class="checkbox checkbox-sm checkbox-primary" ${selectedTeams.has(team.id) ? 'checked' : ''}>
					<div class="item-icon-container">
						<img src="${team.image || '/static/images/default-logo.png'}" alt="${team.name}" class="item-icon" onerror="this.src='/static/images/default-logo.png'">
					</div>
					<span class="text-sm">${team.acronym ? team.acronym + ' - ' + team.name : team.name}</span>
				</label>
			</li>
		`).join('');
		teamList.innerHTML = html;
		highlightedTeamIndex = -1;
		// Re-bind toggle functions
		teamList.querySelectorAll('input[type="checkbox"]').forEach((cb, i) => {
			cb.addEventListener('change', () => toggleTeam(teams[i]));
		});
	}

	// Toggle team selection
	function toggleTeam(team) {
		if (selectedTeams.has(team.id)) {
			selectedTeams.delete(team.id);
		} else {
			selectedTeams.add(team.id);
		}
		saveSelections();
		updateCombinedDisplay();
	}

	// Update combined display - optimized with template literals
	function updateCombinedDisplay() {
		const leagueHTML = allLeagues.filter(l => selectedLeagues.has(l.id)).map(league => `
			<div class="badge badge-primary badge-lg gap-2 rounded-md py-3" data-league-id="${league.id}">
				<div class="item-icon-container">
					<img src="${league.image || '/static/images/default-logo.png'}" alt="${league.name}" class="item-icon-badge" onerror="this.src='/static/images/default-logo.png'">
				</div>
				<span class="text-sm">${league.name}</span>
				<button class="btn btn-ghost btn-xs btn-circle ml-1" onclick="removeLeague(${league.id})">✕</button>
			</div>
		`).join('');

		const teamHTML = allTeams.filter(t => selectedTeams.has(t.id)).map(team => `
			<div class="badge badge-secondary badge-lg gap-2 rounded-md py-3" data-team-id="${team.id}">
				<div class="item-icon-container">
					<img src="${team.image || '/static/images/default-logo.png'}" alt="${team.name}" class="item-icon-badge" onerror="this.src='/static/images/default-logo.png'">
				</div>
				<span class="text-sm">${team.acronym ? team.acronym + ' - ' + team.name : team.name}</span>
				<button class="btn btn-ghost btn-xs btn-circle ml-1" onclick="removeTeam(${team.id})">✕</button>
			</div>
		`).join('');

		selectedCombinedContainer.innerHTML = leagueHTML + teamHTML;

		// Update submit button state
		checkAndUpdateSubmitButton();
	}

	// Helper functions for removal
	window.removeLeague = function(id) {
		const league = allLeagues.find(l => l.id === id);
		if (league) {
			selectedLeagues.delete(id);
			saveSelections();
			updateCombinedDisplay();
			renderLeagues(filterLeagues(searchInput.value));
		}
	};

	window.removeTeam = function(id) {
		const team = allTeams.find(t => t.id === id);
		if (team) {
			selectedTeams.delete(id);
			saveSelections();
			updateCombinedDisplay();
			renderTeams(filterTeams(searchTeamsInput.value));
		}
	};

	// Filter teams based on search input
	function filterTeams(query) {
		const lowerQuery = query.toLowerCase();
		return allTeams.filter(team =>
			team.name.toLowerCase().includes(lowerQuery) ||
			(team.acronym && team.acronym.toLowerCase().includes(lowerQuery))
		);
	}

	// Search input event for teams
	searchTeamsInput.addEventListener('input', (e) => {
		const filtered = filterTeams(e.target.value);
		currentFilteredTeams = filtered;
		highlightedTeamIndex = -1;
		if (filtered.length > 0) {
			renderTeams(filtered);
			teamList.classList.remove('hidden');
			noTeamsResults.classList.add('hidden');
		} else {
			teamList.classList.add('hidden');
			noTeamsResults.classList.remove('hidden');
		}
	});

	// Handle arrow keys and Enter for keyboard navigation in teams
	function highlightTeamItem(index) {
		const items = teamList.querySelectorAll('li');
		items.forEach((item, i) => {
			if (i === index) {
				item.querySelector('label').style.backgroundColor = 'oklch(var(--b3))';
			} else {
				item.querySelector('label').style.backgroundColor = '';
			}
		});
	}

	searchTeamsInput.addEventListener('keydown', (e) => {
		if (e.key === 'Escape') {
			e.preventDefault();
			dropdownTeamsMenu.style.display = 'none';
			searchTeamsInput.blur();
		} else if (e.key === 'ArrowDown') {
			e.preventDefault();
			const items = teamList.querySelectorAll('li');
			if (items.length > 0) {
				highlightedTeamIndex = highlightedTeamIndex + 1;
				if (highlightedTeamIndex >= items.length) {
					highlightedTeamIndex = 0;
				}
				highlightTeamItem(highlightedTeamIndex);
				items[highlightedTeamIndex].scrollIntoView({ block: 'nearest' });
			}
		} else if (e.key === 'ArrowUp') {
			e.preventDefault();
			const items = teamList.querySelectorAll('li');
			if (items.length > 0) {
				highlightedTeamIndex = highlightedTeamIndex - 1;
				if (highlightedTeamIndex < 0) {
					highlightedTeamIndex = items.length - 1;
				}
				highlightTeamItem(highlightedTeamIndex);
				items[highlightedTeamIndex].scrollIntoView({ block: 'nearest' });
			}
		} else if (e.key === 'Enter') {
			e.preventDefault();
			if (highlightedTeamIndex >= 0 && highlightedTeamIndex < currentFilteredTeams.length) {
				const team = currentFilteredTeams[highlightedTeamIndex];
				toggleTeam(team);
				renderTeams(currentFilteredTeams);
				highlightTeamItem(highlightedTeamIndex);
			}
		}
	});

	// Show dropdown on focus for teams
	searchTeamsInput.addEventListener('focus', () => {
		dropdownTeamsMenu.style.display = 'block';
	});

	// Hide dropdown on blur for teams (when losing focus)
	searchTeamsInput.addEventListener('blur', () => {
		// Use setTimeout to allow click events on dropdown items to register first
		setTimeout(() => {
			dropdownTeamsMenu.style.display = 'none';
		}, 200);
	});

	// Hide dropdown when clicking outside for teams
	document.addEventListener('click', (e) => {
		if (!searchTeamsInput.contains(e.target) && !dropdownTeamsMenu.contains(e.target)) {
			dropdownTeamsMenu.style.display = 'none';
		}
	});

	// Setup tier slider
	function setupTierSlider() {
		const tierSlider = document.getElementById('tier-slider-' + gameId);
		const tierValue = document.getElementById('tier-value-' + gameId);

		// Function to convert tier number to letter
		function getTierLabel(tier) {
			const tierMap = { 1: 'S', 2: 'A', 3: 'B', 4: 'C', 5: 'D', 6: 'All' };
			return tierMap[tier] || tier;
		}

		if (tierSlider && tierValue) {
			// Restore saved tier value
			tierSlider.value = maxTier;
			tierValue.textContent = getTierLabel(maxTier);

			tierSlider.addEventListener('input', (e) => {
				maxTier = parseInt(e.target.value);
				tierValue.textContent = getTierLabel(maxTier);
				saveSelections();
			});
		}
	}

	// Deselect all button - needs to reference both leagues and teams
	function setupDeselectAllButton() {
		const deselectAllBtn = document.getElementById('deselect-all-' + gameId);
		if (deselectAllBtn) {
			deselectAllBtn.addEventListener('click', () => {
				console.log('Deselect all clicked for game:', gameId);
				console.log('Before clear - Leagues:', selectedLeagues.size, 'Teams:', selectedTeams.size);

				selectedLeagues.clear();
				selectedTeams.clear();

				console.log('After clear - Leagues:', selectedLeagues.size, 'Teams:', selectedTeams.size);

				saveSelections();
				updateCombinedDisplay();

				// Re-render both lists to update checkboxes
				if (allLeagues.length > 0) {
					renderLeagues(searchInput.value ? filterLeagues(searchInput.value) : allLeagues);
				}
				if (allTeams.length > 0) {
					renderTeams(searchTeamsInput.value ? filterTeams(searchTeamsInput.value) : allTeams);
				}
			});
		}
	}

	// Call setup after a brief delay to ensure both lists are loaded
	setTimeout(() => {
		setupDeselectAllButton();
		setupTierSlider();
	}, 100);
}
