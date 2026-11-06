// League/team names and image URLs come from the PandaScore API, so every
// node below is built with DOM APIs (textContent, attributes) rather than
// interpolated into innerHTML.
function createItemIcon(src, alt, className) {
	const DEFAULT_ITEM_LOGO = '/static/images/default-logo.png';
	const img = document.createElement('img');
	img.src = src || DEFAULT_ITEM_LOGO;
	img.alt = alt;
	img.className = className;
	img.addEventListener(
		'error',
		() => {
			img.src = DEFAULT_ITEM_LOGO;
		},
		{ once: true },
	);
	return img;
}

// One checkbox row in a league/team search dropdown.
function createOptionItem(index, item, label, checked, onChange) {
	const li = document.createElement('li');
	li.dataset.index = String(index);

	const labelEl = document.createElement('label');
	labelEl.className = 'label cursor-pointer justify-start gap-2 p-2';

	const checkbox = document.createElement('input');
	checkbox.type = 'checkbox';
	checkbox.className = 'checkbox checkbox-sm checkbox-primary';
	checkbox.checked = checked;
	checkbox.addEventListener('change', onChange);

	const iconWrap = document.createElement('div');
	iconWrap.className = 'item-icon-container';
	iconWrap.appendChild(createItemIcon(item.image, item.name, 'item-icon'));

	const text = document.createElement('span');
	text.className = 'text-sm';
	text.textContent = label;

	labelEl.append(checkbox, iconWrap, text);
	li.appendChild(labelEl);
	return li;
}

// One removable pill in a card's "selected leagues/teams" row.
function createSelectedBadge(variant, item, label, onRemove) {
	const badge = document.createElement('div');
	badge.className = `badge ${variant} rounded-md inline-flex items-center gap-1.5`;

	const text = document.createElement('span');
	text.textContent = label;

	const removeBtn = document.createElement('button');
	removeBtn.type = 'button';
	removeBtn.className = 'cursor-pointer leading-none opacity-70 hover:opacity-100';
	removeBtn.setAttribute('aria-label', `Remove ${item.name}`);
	removeBtn.textContent = '✕';
	removeBtn.addEventListener('click', onRemove);

	badge.append(createItemIcon(item.image, item.name, 'item-icon-badge'), text, removeBtn);
	return badge;
}

const LEAGUE_OPTIONS_TIMEOUT_MS = 10000;

function teamLabel(team) {
	return team.acronym ? team.acronym + ' - ' + team.name : team.name;
}

// Game selection - League and Team management.
// storagePrefix namespaces the saved filters in sessionStorage; /fixtures and
// /calendar share the same prefix so their filters stay in sync.
// Every saved change dispatches `game-selection-change` on document, with
// detail.seeded true when it's the tier-1 auto-select rather than the user.
// Returns a promise that settles once this game's selections are final: true if
// they can be trusted, false if the options failed to load and nothing was saved,
// which leaves the game without its tier-1 seed.
function initGameSelection(gameId, storagePrefix) {
	if (!gameId || gameId === 'null' || gameId === 'undefined') {
		console.error('Invalid gameId:', gameId);
		return Promise.resolve(false);
	}

	const searchInput = document.getElementById('search-' + gameId);
	const dropdownMenu = document.getElementById('dropdown-menu-' + gameId);
	const loadingElement = document.getElementById('loading-' + gameId);
	const leagueList = document.getElementById('league-list-' + gameId);
	const noResults = document.getElementById('no-results-' + gameId);
	const selectedLeaguesContainer = document.getElementById('selected-leagues-' + gameId);
	const selectedTeamsContainer = document.getElementById('selected-teams-' + gameId);

	if (!searchInput || !dropdownMenu || !loadingElement) {
		console.error('Required elements not found for gameId:', gameId);
		return Promise.resolve(false);
	}

	let allLeagues = [];
	let selectedLeagues = new Set();
	let currentFilteredLeagues = [];
	let maxTier = 2; // Default to tier A (tier 2)

	// Restore saved selections from sessionStorage
	const savedKey = storagePrefix + gameId;
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
	// The timeout matters: pages hold changes back until this settles, so a hung
	// request must end in the catch below rather than leave them waiting.
	const apiUrl = '/api/league-options/' + gameId;
	const ready = fetch(apiUrl, { signal: AbortSignal.timeout(LEAGUE_OPTIONS_TIMEOUT_MS) })
		.then((response) => response.json())
		.then((data) => {
			loadingElement.classList.add('hidden');

			if (data.error) {
				noResults.textContent = data.message;
				noResults.classList.remove('hidden');
				return hasSavedSelections;
			}
			if (data.leagues && data.leagues.length > 0) {
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
						saveSelections({ seeded: true });
					}
				}

				renderLeagues(allLeagues);
				updateCombinedDisplay();
				leagueList.classList.remove('hidden');
			} else {
				noResults.textContent = 'No leagues available for this game';
				noResults.classList.remove('hidden');
			}
			return true;
		})
		.catch((error) => {
			loadingElement.classList.add('hidden');
			noResults.textContent = 'Failed to load leagues. Please try again.';
			noResults.classList.remove('hidden');
			console.error('Error fetching leagues:', error);
			return hasSavedSelections;
		});

	// Render leagues list
	let highlightedLeagueIndex = -1;
	function renderLeagues(leagues) {
		leagueList.replaceChildren(
			...leagues.map((league, index) =>
				createOptionItem(index, league, league.name, selectedLeagues.has(league.id), () =>
					toggleLeague(league),
				),
			),
		);
		highlightedLeagueIndex = -1;
	}

	// Save selections to sessionStorage
	function saveSelections(opts) {
		const data = {
			leagues: Array.from(selectedLeagues),
			teams: Array.from(selectedTeams),
			maxTier: maxTier,
		};
		sessionStorage.setItem(savedKey, JSON.stringify(data));
		document.dispatchEvent(
			new CustomEvent('game-selection-change', {
				detail: { gameId, seeded: !!(opts && opts.seeded) },
			}),
		);
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
		teamList.replaceChildren(
			...teams.map((team, index) =>
				createOptionItem(index, team, teamLabel(team), selectedTeams.has(team.id), () => toggleTeam(team)),
			),
		);
		highlightedTeamIndex = -1;
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
		if (selectedLeaguesContainer) {
			selectedLeaguesContainer.replaceChildren(
				...allLeagues
					.filter((l) => selectedLeagues.has(l.id))
					.map((league) => {
						const badge = createSelectedBadge('badge-primary', league, league.name, () =>
							removeLeague(league.id),
						);
						badge.dataset.leagueId = String(league.id);
						return badge;
					}),
			);
		}
		if (selectedTeamsContainer) {
			selectedTeamsContainer.replaceChildren(
				...allTeams
					.filter((t) => selectedTeams.has(t.id))
					.map((team) => {
						const badge = createSelectedBadge('badge-secondary', team, teamLabel(team), () =>
							removeTeam(team.id),
						);
						badge.dataset.teamId = String(team.id);
						return badge;
					}),
			);
		}
	}

	function removeLeague(id) {
		selectedLeagues.delete(id);
		saveSelections();
		updateCombinedDisplay();
		renderLeagues(filterLeagues(searchInput.value));
	}

	function removeTeam(id) {
		selectedTeams.delete(id);
		saveSelections();
		updateCombinedDisplay();
		renderTeams(filterTeams(searchTeamsInput.value));
	}

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

	return ready;
}
