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

				// Auto-select tier 1 leagues
				allLeagues.forEach(league => {
					if (league.is_tier1) {
						selectedLeagues.add(league.id);
					}
				});

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

	// Render leagues list
	function renderLeagues(leagues) {
		leagueList.innerHTML = '';
		leagues.forEach(league => {
			const li = document.createElement('li');
			const label = document.createElement('label');
			label.className = 'label cursor-pointer justify-start gap-2 p-2';

			const checkbox = document.createElement('input');
			checkbox.type = 'checkbox';
			checkbox.className = 'checkbox checkbox-sm checkbox-primary';
			checkbox.checked = selectedLeagues.has(league.id);
			checkbox.addEventListener('change', () => toggleLeague(league));

			// Add league image with white background
			const imgContainer = document.createElement('div');
			imgContainer.className = 'item-icon-container';
			const img = document.createElement('img');
			img.src = league.image || '/static/images/default-logo.png';
			img.alt = league.name;
			img.className = 'item-icon';
			img.onerror = function() {
				this.src = '/static/images/default-logo.png';
			};
			imgContainer.appendChild(img);

			const span = document.createElement('span');
			span.textContent = league.name;
			span.className = 'text-sm';

			label.appendChild(checkbox);
			label.appendChild(imgContainer);
			label.appendChild(span);
			li.appendChild(label);
			leagueList.appendChild(li);
		});
	}

	// Toggle league selection
	function toggleLeague(league) {
		if (selectedLeagues.has(league.id)) {
			selectedLeagues.delete(league.id);
		} else {
			selectedLeagues.add(league.id);
		}
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
	let currentFilteredLeagues = [];
	searchInput.addEventListener('input', (e) => {
		const filtered = filterLeagues(e.target.value);
		currentFilteredLeagues = filtered;
		if (filtered.length > 0) {
			renderLeagues(filtered);
			leagueList.classList.remove('hidden');
			noResults.classList.add('hidden');
		} else {
			leagueList.classList.add('hidden');
			noResults.classList.remove('hidden');
		}
	});

	// Handle Enter key to select first result
	searchInput.addEventListener('keydown', (e) => {
		if (e.key === 'Enter' && currentFilteredLeagues.length > 0) {
			e.preventDefault();
			const firstLeague = currentFilteredLeagues[0];
			if (!selectedLeagues.has(firstLeague.id)) {
				selectedLeagues.add(firstLeague.id);
				updateCombinedDisplay();
				renderLeagues(currentFilteredLeagues);
			}
			searchInput.value = '';
			currentFilteredLeagues = allLeagues;
			renderLeagues(allLeagues);
		}
	});

	// Show dropdown on focus
	searchInput.addEventListener('focus', () => {
		dropdownMenu.style.display = 'block';
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

	// Render teams list
	function renderTeams(teams) {
		teamList.innerHTML = '';
		teams.forEach(team => {
			const li = document.createElement('li');
			const label = document.createElement('label');
			label.className = 'label cursor-pointer justify-start gap-2 p-2';

			const checkbox = document.createElement('input');
			checkbox.type = 'checkbox';
			checkbox.className = 'checkbox checkbox-sm checkbox-primary';
			checkbox.checked = selectedTeams.has(team.id);
			checkbox.addEventListener('change', () => toggleTeam(team));

			// Add team image with white background
			const imgContainer = document.createElement('div');
			imgContainer.className = 'item-icon-container';
			const img = document.createElement('img');
			img.src = team.image || '/static/images/default-logo.png';
			img.alt = team.name;
			img.className = 'item-icon';
			img.onerror = function() {
				this.src = '/static/images/default-logo.png';
			};
			imgContainer.appendChild(img);

			const span = document.createElement('span');
			span.textContent = team.acronym ? team.acronym + ' - ' + team.name : team.name;
			span.className = 'text-sm';

			label.appendChild(checkbox);
			label.appendChild(imgContainer);
			label.appendChild(span);
			li.appendChild(label);
			teamList.appendChild(li);
		});
	}

	// Toggle team selection
	function toggleTeam(team) {
		if (selectedTeams.has(team.id)) {
			selectedTeams.delete(team.id);
		} else {
			selectedTeams.add(team.id);
		}
		updateCombinedDisplay();
	}

	// Update combined display with both leagues and teams
	function updateCombinedDisplay() {
		selectedCombinedContainer.innerHTML = '';

		// Add selected leagues
		allLeagues.filter(l => selectedLeagues.has(l.id)).forEach(league => {
			const badge = document.createElement('div');
			badge.className = 'badge badge-primary badge-lg gap-2 rounded-md py-3';

			// Add league image with white background
			const imgContainer = document.createElement('div');
			imgContainer.className = 'item-icon-container';
			const img = document.createElement('img');
			img.src = league.image || '/static/images/default-logo.png';
			img.alt = league.name;
			img.className = 'item-icon-badge';
			img.onerror = function() {
				this.src = '/static/images/default-logo.png';
			};
			imgContainer.appendChild(img);
			badge.appendChild(imgContainer);

			const span = document.createElement('span');
			span.textContent = league.name;
			span.className = 'text-sm';
			badge.appendChild(span);

			const removeBtn = document.createElement('button');
			removeBtn.className = 'btn btn-ghost btn-xs btn-circle ml-1';
			removeBtn.innerHTML = '✕';
			removeBtn.addEventListener('click', () => {
				selectedLeagues.delete(league.id);
				updateCombinedDisplay();
				const filtered = filterLeagues(searchInput.value);
				renderLeagues(filtered);
			});

			badge.appendChild(removeBtn);
			selectedCombinedContainer.appendChild(badge);
		});

		// Add selected teams
		allTeams.filter(t => selectedTeams.has(t.id)).forEach(team => {
			const badge = document.createElement('div');
			badge.className = 'badge badge-secondary badge-lg gap-2 rounded-md py-3';

			// Add team image with white background
			const imgContainer = document.createElement('div');
			imgContainer.className = 'item-icon-container';
			const img = document.createElement('img');
			img.src = team.image || '/static/images/default-logo.png';
			img.alt = team.name;
			img.className = 'item-icon-badge';
			img.onerror = function() {
				this.src = '/static/images/default-logo.png';
			};
			imgContainer.appendChild(img);
			badge.appendChild(imgContainer);

			const span = document.createElement('span');
			span.textContent = team.acronym ? team.acronym + ' - ' + team.name : team.name;
			span.className = 'text-sm';
			badge.appendChild(span);

			const removeBtn = document.createElement('button');
			removeBtn.className = 'btn btn-ghost btn-xs btn-circle ml-1';
			removeBtn.innerHTML = '✕';
			removeBtn.addEventListener('click', () => {
				selectedTeams.delete(team.id);
				updateCombinedDisplay();
				const filtered = filterTeams(searchTeamsInput.value);
				renderTeams(filtered);
			});

			badge.appendChild(removeBtn);
			selectedCombinedContainer.appendChild(badge);
		});
	}

	// Filter teams based on search input
	function filterTeams(query) {
		const lowerQuery = query.toLowerCase();
		return allTeams.filter(team =>
			team.name.toLowerCase().includes(lowerQuery) ||
			(team.acronym && team.acronym.toLowerCase().includes(lowerQuery))
		);
	}

	// Search input event for teams
	let currentFilteredTeams = [];
	searchTeamsInput.addEventListener('input', (e) => {
		const filtered = filterTeams(e.target.value);
		currentFilteredTeams = filtered;
		if (filtered.length > 0) {
			renderTeams(filtered);
			teamList.classList.remove('hidden');
			noTeamsResults.classList.add('hidden');
		} else {
			teamList.classList.add('hidden');
			noTeamsResults.classList.remove('hidden');
		}
	});

	// Handle Enter key to select first result for teams
	searchTeamsInput.addEventListener('keydown', (e) => {
		if (e.key === 'Enter' && currentFilteredTeams.length > 0) {
			e.preventDefault();
			const firstTeam = currentFilteredTeams[0];
			if (!selectedTeams.has(firstTeam.id)) {
				selectedTeams.add(firstTeam.id);
				updateCombinedDisplay();
				renderTeams(currentFilteredTeams);
			}
			searchTeamsInput.value = '';
			currentFilteredTeams = allTeams;
			renderTeams(allTeams);
		}
	});

	// Show dropdown on focus for teams
	searchTeamsInput.addEventListener('focus', () => {
		dropdownTeamsMenu.style.display = 'block';
	});

	// Hide dropdown when clicking outside for teams
	document.addEventListener('click', (e) => {
		if (!searchTeamsInput.contains(e.target) && !dropdownTeamsMenu.contains(e.target)) {
			dropdownTeamsMenu.style.display = 'none';
		}
	});
}
