// Rehydrate a page whose state lives only in sessionStorage. The server
// renders a small "restoring session" shell (components/rehydrate.templ) that
// embeds a <div id="hud-rehydrate-config"> with the parameters needed to
// reconstruct the original POST. We read the saved payload, POST it back to
// the same URL with HX-Request: true so the server returns the inner partial,
// and swap that partial into #page-content. If anything goes wrong (no saved
// payload, malformed JSON, non-2xx response, network error), redirect to /.
(function () {
	function run() {
		const cfg = document.getElementById('hud-rehydrate-config');
		if (!cfg) return;

		const sessionKey = cfg.getAttribute('data-session-key');
		const targetURL = cfg.getAttribute('data-target-url');
		const bodyMode = cfg.getAttribute('data-body-mode');
		const finalTitle = cfg.getAttribute('data-final-title');

		const fail = (reason) => {
			if (reason && window.console) console.warn('rehydrate:', reason);
			window.location.replace('/');
		};

		let saved;
		try {
			saved = sessionStorage.getItem(sessionKey);
		} catch {
			return fail('sessionStorage unavailable');
		}
		if (!saved) return fail('no saved payload');

		let body, contentType;
		if (bodyMode === 'json') {
			// Validate it's actually parseable JSON.
			try {
				JSON.parse(saved);
			} catch {
				return fail('saved JSON is malformed');
			}
			body = saved;
			contentType = 'application/json';
		} else if (bodyMode === 'form-options') {
			let ids;
			try {
				ids = JSON.parse(saved);
			} catch {
				return fail('saved options malformed');
			}
			if (!Array.isArray(ids) || ids.length === 0) {
				return fail('no options selected');
			}
			body = ids.map((id) => 'options=' + encodeURIComponent(id)).join('&');
			contentType = 'application/x-www-form-urlencoded';
		} else {
			return fail('unknown bodyMode: ' + bodyMode);
		}

		fetch(targetURL, {
			method: 'POST',
			credentials: 'same-origin',
			headers: {
				'Content-Type': contentType,
				'HX-Request': 'true',
				'HX-Target': 'page-content',
			},
			body: body,
		})
			.then((res) => {
				if (!res.ok) throw new Error('status=' + res.status);
				return res.text();
			})
			.then((html) => {
				const target = document.getElementById('page-content');
				if (!target) throw new Error('no #page-content');
				target.innerHTML = html;
				target.classList.add('fade-in');
				if (typeof window.executeScriptsIn === 'function') {
					window.executeScriptsIn(target);
				}
				if (typeof htmx !== 'undefined') {
					htmx.process(target);
				}
				if (finalTitle) document.title = finalTitle;
			})
			.catch((err) => fail(err && err.message));
	}

	// Wait for DOMContentLoaded so app.js has installed window.executeScriptsIn
	// and htmx config has been set.
	if (document.readyState === 'loading') {
		document.addEventListener('DOMContentLoaded', run, { once: true });
	} else {
		run();
	}
})();
