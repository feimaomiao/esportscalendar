// Check if any selections are made and enable/disable submit button
function checkAndUpdateSubmitButton() {
	const gameCards = document.querySelectorAll('[data-game-id]');
	let hasSelections = false;

	gameCards.forEach((card) => {
		// Selected leagues and selected teams now live in two separate
		// containers per card; scan all `.selected-items-container` under
		// the card so either type's badges count.
		if (card.querySelectorAll('.selected-items-container .badge').length > 0) {
			hasSelections = true;
		}
	});

	const submitBtn = document.getElementById('submit-selection-btn');
	if (submitBtn) {
		submitBtn.disabled = !hasSelections;
	}
}

// Game selection - League and Team management.
// storagePrefix lets callers (e.g. /fixtures) namespace their saved filters
// instead of sharing the wizard's `lts-selections-` keys.
function initGameSelection(gameId, storagePrefix) {
	if (!gameId || gameId === 'null' || gameId === 'undefined') {
		console.error('Invalid gameId:', gameId);
		return;
	}

	const prefix = storagePrefix || 'lts-selections-';

	const searchInput = document.getElementById('search-' + gameId);
	const dropdownMenu = document.getElementById('dropdown-menu-' + gameId);
	const loadingElement = document.getElementById('loading-' + gameId);
	const leagueList = document.getElementById('league-list-' + gameId);
	const noResults = document.getElementById('no-results-' + gameId);
	const selectedLeaguesContainer = document.getElementById('selected-leagues-' + gameId);
	const selectedTeamsContainer = document.getElementById('selected-teams-' + gameId);

	if (!searchInput || !dropdownMenu || !loadingElement) {
		console.error('Required elements not found for gameId:', gameId);
		return;
	}

	let allLeagues = [];
	let selectedLeagues = new Set();
	let currentFilteredLeagues = [];
	let maxTier = 2; // Default to tier A (tier 2)

	// Restore saved selections from sessionStorage
	const savedKey = prefix + gameId;
	const savedData = sessionStorage.getItem(savedKey);
	let hasSavedSelections = false;
	if (savedData) {
		try {
			const parsed = JSON.parse(savedData);
			if (parsed.leagues && Array.isArray(parsed.leagues)) {
				parsed.leagues.forEach((id) => selectedLeagues.add(id));
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
	fetch(apiUrl)
		.then((response) => response.json())
		.then((data) => {
			loadingElement.classList.add('hidden');

			if (data.error) {
				noResults.textContent = data.message;
				noResults.classList.remove('hidden');
			} else if (data.leagues && data.leagues.length > 0) {
				allLeagues = data.leagues;
				currentFilteredLeagues = data.leagues;

				// Auto-select tier 1 leagues only if no saved selections
				if (!hasSavedSelections) {
					allLeagues.forEach((league) => {
						if (league.is_tier1) {
							selectedLeagues.add(league.id);
						}
					});
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
		.catch((error) => {
			loadingElement.classList.add('hidden');
			noResults.textContent = 'Failed to load leagues. Please try again.';
			noResults.classList.remove('hidden');
			console.error('Error fetching leagues:', error);
		});

	// Render leagues list
	let highlightedLeagueIndex = -1;
	function renderLeagues(leagues) {
		const html = leagues
			.map(
				(league, index) => `
			<li data-index="${index}">
				<label class="label cursor-pointer justify-start gap-2 p-2">
					<input type="checkbox" class="checkbox checkbox-sm checkbox-primary" ${selectedLeagues.has(league.id) ? 'checked' : ''}>
					<div class="item-icon-container">
						<img src="${league.image || '/static/images/default-logo.png'}" alt="${league.name}" class="item-icon" onerror="this.src='/static/images/default-logo.png'">
					</div>
					<span class="text-sm">${league.name}</span>
				</label>
			</li>
		`,
			)
			.join('');
		leagueList.innerHTML = html;
		highlightedLeagueIndex = -1;
		leagueList.querySelectorAll('input[type="checkbox"]').forEach((cb, i) => {
			cb.addEventListener('change', () => toggleLeague(leagues[i]));
		});
	}

	// Save selections to sessionStorage
	function saveSelections() {
		const data = {
			leagues: Array.from(selectedLeagues),
			teams: Array.from(selectedTeams),
			maxTier: maxTier,
		};
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
		return allLeagues.filter((league) => league.name.toLowerCase().includes(lowerQuery));
	}

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

	function highlightLeagueItem(index) {
		const items = leagueList.querySelectorAll('li');
		items.forEach((item, i) => {
			const label = item.querySelector('label');
			if (!label) return;
			label.classList.toggle('dropdown-item-highlight', i === index);
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

	searchInput.addEventListener('focus', () => {
		dropdownMenu.style.display = 'block';
	});

	searchInput.addEventListener('blur', () => {
		setTimeout(() => {
			dropdownMenu.style.display = 'none';
		}, 200);
	});

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

	let allTeams = [];
	let selectedTeams = new Set();
	let currentFilteredTeams = [];

	if (savedData) {
		try {
			const parsed = JSON.parse(savedData);
			if (parsed.teams && Array.isArray(parsed.teams)) {
				parsed.teams.forEach((id) => selectedTeams.add(id));
			}
		} catch (e) {
			console.error('Failed to parse saved team selections:', e);
		}
	}

	const teamApiUrl = '/api/team-options/' + gameId;
	fetch(teamApiUrl)
		.then((response) => response.json())
		.then((data) => {
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
		.catch((error) => {
			loadingTeamsElement.classList.add('hidden');
			noTeamsResults.textContent = 'Failed to load teams. Please try again.';
			noTeamsResults.classList.remove('hidden');
			console.error('Error fetching teams:', error);
		});

	let highlightedTeamIndex = -1;
	function renderTeams(teams) {
		const html = teams
			.map(
				(team, index) => `
			<li data-index="${index}">
				<label class="label cursor-pointer justify-start gap-2 p-2">
					<input type="checkbox" class="checkbox checkbox-sm checkbox-primary" ${selectedTeams.has(team.id) ? 'checked' : ''}>
					<div class="item-icon-container">
						<img src="${team.image || '/static/images/default-logo.png'}" alt="${team.name}" class="item-icon" onerror="this.src='/static/images/default-logo.png'">
					</div>
					<span class="text-sm">${team.acronym ? team.acronym + ' - ' + team.name : team.name}</span>
				</label>
			</li>
		`,
			)
			.join('');
		teamList.innerHTML = html;
		highlightedTeamIndex = -1;
		teamList.querySelectorAll('input[type="checkbox"]').forEach((cb, i) => {
			cb.addEventListener('change', () => toggleTeam(teams[i]));
		});
	}

	function toggleTeam(team) {
		if (selectedTeams.has(team.id)) {
			selectedTeams.delete(team.id);
		} else {
			selectedTeams.add(team.id);
		}
		saveSelections();
		updateCombinedDisplay();
	}

	function updateCombinedDisplay() {
		const leagueHTML = allLeagues
			.filter((l) => selectedLeagues.has(l.id))
			.map(
				(league) => `
			<div class="badge badge-primary rounded-md inline-flex items-center gap-1.5" data-league-id="${league.id}">
				<img src="${league.image || '/static/images/default-logo.png'}" alt="${league.name}" class="item-icon-badge" onerror="this.src='/static/images/default-logo.png'">
				<span>${league.name}</span>
				<button type="button" class="cursor-pointer leading-none opacity-70 hover:opacity-100" onclick="removeLeague(${league.id}, '${gameId}')" aria-label="Remove ${league.name}">✕</button>
			</div>
		`,
			)
			.join('');

		const teamHTML = allTeams
			.filter((t) => selectedTeams.has(t.id))
			.map(
				(team) => `
			<div class="badge badge-secondary rounded-md inline-flex items-center gap-1.5" data-team-id="${team.id}">
				<img src="${team.image || '/static/images/default-logo.png'}" alt="${team.name}" class="item-icon-badge" onerror="this.src='/static/images/default-logo.png'">
				<span>${team.acronym ? team.acronym + ' - ' + team.name : team.name}</span>
				<button type="button" class="cursor-pointer leading-none opacity-70 hover:opacity-100" onclick="removeTeam(${team.id}, '${gameId}')" aria-label="Remove ${team.name}">✕</button>
			</div>
		`,
			)
			.join('');

		if (selectedLeaguesContainer) selectedLeaguesContainer.innerHTML = leagueHTML;
		if (selectedTeamsContainer) selectedTeamsContainer.innerHTML = teamHTML;

		checkAndUpdateSubmitButton();
	}

	// Per-card handlers — registered in a global registry keyed by gameId so
	// pages with multiple game cards (e.g. /fixtures) don't clobber each other's
	// closures via a single window.removeLeague.
	window.__gameSelectionHandlers = window.__gameSelectionHandlers || {};
	window.__gameSelectionHandlers[gameId] = {
		removeLeague(id) {
			const league = allLeagues.find((l) => l.id === id);
			if (!league) return false;
			selectedLeagues.delete(id);
			saveSelections();
			updateCombinedDisplay();
			renderLeagues(filterLeagues(searchInput.value));
			return true;
		},
		removeTeam(id) {
			const team = allTeams.find((t) => t.id === id);
			if (!team) return false;
			selectedTeams.delete(id);
			saveSelections();
			updateCombinedDisplay();
			renderTeams(filterTeams(searchTeamsInput.value));
			return true;
		},
	};

	// Global dispatchers — preferred call form passes the owning gameId; falls
	// back to a sweep across registered cards for legacy callers that omit it.
	window.removeLeague = function (id, ownerGameId) {
		const handlers = window.__gameSelectionHandlers || {};
		if (ownerGameId && handlers[ownerGameId]) {
			handlers[ownerGameId].removeLeague(id);
			return;
		}
		Object.values(handlers).some((h) => h.removeLeague(id));
	};
	window.removeTeam = function (id, ownerGameId) {
		const handlers = window.__gameSelectionHandlers || {};
		if (ownerGameId && handlers[ownerGameId]) {
			handlers[ownerGameId].removeTeam(id);
			return;
		}
		Object.values(handlers).some((h) => h.removeTeam(id));
	};

	function filterTeams(query) {
		const lowerQuery = query.toLowerCase();
		return allTeams.filter(
			(team) =>
				team.name.toLowerCase().includes(lowerQuery) ||
				(team.acronym && team.acronym.toLowerCase().includes(lowerQuery)),
		);
	}

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

	function highlightTeamItem(index) {
		const items = teamList.querySelectorAll('li');
		items.forEach((item, i) => {
			const label = item.querySelector('label');
			if (!label) return;
			label.classList.toggle('dropdown-item-highlight', i === index);
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

	searchTeamsInput.addEventListener('focus', () => {
		dropdownTeamsMenu.style.display = 'block';
	});

	searchTeamsInput.addEventListener('blur', () => {
		setTimeout(() => {
			dropdownTeamsMenu.style.display = 'none';
		}, 200);
	});

	document.addEventListener('click', (e) => {
		if (!searchTeamsInput.contains(e.target) && !dropdownTeamsMenu.contains(e.target)) {
			dropdownTeamsMenu.style.display = 'none';
		}
	});

	function setupTierSlider() {
		const tierSlider = document.getElementById('tier-slider-' + gameId);
		const tierValue = document.getElementById('tier-value-' + gameId);

		function getTierLabel(tier) {
			// 0 = OFF (no tier auto-include — only selected leagues/teams show).
			// 1–5 = auto-include tier S through D. Range form makes the additive
			// nature obvious as the slider drags right (more tiers join).
			const rangeMap = { 0: 'OFF', 1: 'S', 2: 'S–A', 3: 'S–B', 4: 'S–C', 5: 'S–D' };
			return rangeMap[tier] ?? tier;
		}

		if (tierSlider && tierValue) {
			tierSlider.value = maxTier;
			tierValue.textContent = getTierLabel(maxTier);

			tierSlider.addEventListener('input', (e) => {
				maxTier = parseInt(e.target.value);
				tierValue.textContent = getTierLabel(maxTier);
				saveSelections();
			});
		}
	}

	function setupDeselectAllButton() {
		// Each clear button now scopes to its own type — clearing leagues
		// keeps team picks intact and vice versa. Early-return when the set
		// is already empty so the click is a no-op (no needless re-renders).
		const deselectLeaguesBtn = document.getElementById('deselect-leagues-' + gameId);
		if (deselectLeaguesBtn) {
			deselectLeaguesBtn.addEventListener('click', () => {
				if (selectedLeagues.size === 0) return;
				selectedLeagues.clear();
				saveSelections();
				updateCombinedDisplay();
				if (allLeagues.length > 0) {
					renderLeagues(searchInput.value ? filterLeagues(searchInput.value) : allLeagues);
				}
			});
		}

		const deselectTeamsBtn = document.getElementById('deselect-teams-' + gameId);
		if (deselectTeamsBtn) {
			deselectTeamsBtn.addEventListener('click', () => {
				if (selectedTeams.size === 0) return;
				selectedTeams.clear();
				saveSelections();
				updateCombinedDisplay();
				if (allTeams.length > 0) {
					renderTeams(searchTeamsInput.value ? filterTeams(searchTeamsInput.value) : allTeams);
				}
			});
		}
	}

	setTimeout(() => {
		setupDeselectAllButton();
		setupTierSlider();
	}, 100);
}
