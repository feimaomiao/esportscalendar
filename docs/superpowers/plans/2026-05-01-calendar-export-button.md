# Calendar Page Export Button Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Surface the existing `POST /export` ICS-subscription action directly on `/calendar` so users can export their current month-view filter state without detouring through `/lts` → `/preview`.

**Architecture:** Extract the modal + POST logic that currently lives inline in `static/js/preview.js` into a new shared helper `static/js/export-calendar.js` exposing `window.exportCalendarPayload(payload, button)`. Add an Export button to `components/calendar-page.templ`'s `hud-control-bar-actions` row (sibling of the existing Tune button) and wire it in `static/js/calendar.js` to build a `{selections, hideScores}` payload from the page's existing `collectSelections()` helper, then delegate to `exportCalendarPayload`. Refactor `preview.js` to delegate to the same helper so behavior stays in lockstep across both pages until `/preview` is eventually retired.

**Tech Stack:** Go 1.26 + Gin, templ 0.3 (HTML templates → generated `_templ.go`), vanilla ES module JavaScript (no bundler), Tailwind v4 + DaisyUI v5 for styling.

---

## Repository conventions (read before starting)

- **No automated tests exist** for the middleware HTTP layer or the JS modules (`*_test.go` not present in `middleware/`; no JS test runner in `package.json`). This plan therefore verifies behavior with `go build ./...`, `npx eslint .`, and explicit manual browser checks rather than fabricating a TDD scaffold.
- **Do not run `git commit`.** Per the user's standing preference for this branch (`frontend-rewrite`), every task ends with a verification step instead of a commit. The user will commit manually when satisfied.
- Templ regeneration: every edit to `*.templ` requires `templ generate` to refresh the matching `*_templ.go` file. The generated file is checked in and consumed by `go build`.
- IIFE module pattern: every JS file under `static/js/` is wrapped in a self-aborting IIFE with a `window.__<name>Abort` controller so re-execution after htmx swaps cleans up old listeners. Preserve that idiom in any new file.
- Global helpers from `app.js`: `window.showToast(message, kind)`, `window.setButtonLoading(button, loading)`, `window.convertMatchTimesIn(root)`, `window.executeScriptsIn(root)`. Defer-loaded; safely use optional-chaining (`window.showToast?.(...)`).

## File Structure

| File                                | Action         | Responsibility                                                                                                                                                                                                                  |
| ----------------------------------- | -------------- | ------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| `static/js/export-calendar.js`      | **Create**     | Shared helper exposing `window.exportCalendarPayload(payload, button)` that POSTs to `/export`, handles errors, and renders the link modal. Pure module — no page-specific DOM lookups.                                         |
| `static/js/preview.js`              | **Modify**     | Replace inline export logic (lines 4–116, 170–180) with a call to the shared helper. Keep "Back to Selection" button logic and the `convertMatchTimesIn` boot.                                                                  |
| `components/preview-page.templ`     | **Modify**     | Add `<script src="/static/js/export-calendar.js"></script>` before the existing `preview.js` script tag.                                                                                                                        |
| `components/preview-page_templ.go`  | **Regenerate** | Auto-produced by `templ generate`; do not hand-edit.                                                                                                                                                                            |
| `components/calendar-page.templ`    | **Modify**     | Add an Export button to `hud-control-bar-actions` (after the Tune button) and a `<script src="/static/js/export-calendar.js">` tag before the existing `calendar.js` script.                                                    |
| `components/calendar-page_templ.go` | **Regenerate** | Auto-produced by `templ generate`; do not hand-edit.                                                                                                                                                                            |
| `static/js/calendar.js`             | **Modify**     | Add a click handler for the new `#calendar-export-btn` that builds `{selections, hideScores}` from existing `collectSelections()` + the `hideScoresEl` checkbox, validates non-empty, and calls `window.exportCalendarPayload`. |

**Out of scope (do not touch in this plan):** removing `/lts` and `/preview` routes; renaming `/schedule`; rewriting the `/` index page; adding the Export button to `/schedule` (a follow-up will reuse the same helper). If you find yourself editing those files for any reason other than the four listed above, stop and re-read the spec.

---

## Task 1: Create the shared export helper module

**Files:**

- Create: `static/js/export-calendar.js`

**Context:** This module is purely additive — nothing imports or loads it yet, so it cannot break anything. It is a verbatim extraction of the modal + POST behavior currently inlined in `static/js/preview.js:23–114`. Later tasks point both `preview.js` and `calendar.js` at it.

- [ ] **Step 1: Write the helper file**

Create `static/js/export-calendar.js` with this exact content:

```js
// Shared helper for the Export Calendar action. Used by both /preview and
// /calendar. POSTs the supplied {selections, hideScores} payload to /export,
// auto-copies the returned subscription URL to the clipboard, and shows a
// modal so the user can re-copy or close.
//
// Public API: window.exportCalendarPayload(payload, button)
//   payload — object with shape { selections: {...}, hideScores: boolean }
//             matching the format /api/calendar and /api/schedule already use.
//   button  — the trigger element. Used with window.setButtonLoading so the
//             button shows a spinner while the request is in flight.
//
// Failure modes (each surfaces a toast and returns without throwing):
//   * Non-2xx response: shows the server's error message when present.
//   * Network error: shows the JS error message.
//   * Clipboard rejected: modal still opens, status line tells the user
//     to copy manually.
(function init() {
  if (typeof window.exportCalendarPayload === "function") return;

  function showLinkModal(url, autoCopied) {
    const dialog = document.createElement("dialog");
    dialog.className = "modal";
    dialog.innerHTML = `
			<div class="modal-box max-w-lg">
				<h3 class="font-bold text-lg mb-2">Calendar Link Created</h3>
				<p class="text-sm text-base-content/70 mb-4">Add this URL to your calendar app (Google Calendar → "From URL", Apple Calendar → "New Calendar Subscription", etc.) — it stays in sync as new matches are scheduled.</p>
				<input type="text" readonly class="input input-bordered w-full font-mono text-xs" id="calendar-url-input">
				<p class="text-xs mt-2 h-4" id="calendar-url-status" aria-live="polite"></p>
				<div class="flex justify-end gap-2 mt-4">
					<form method="dialog" class="contents">
						<button type="submit" class="btn">Done</button>
					</form>
					<button type="button" class="btn btn-primary" id="calendar-url-copy">Copy</button>
				</div>
			</div>
			<form method="dialog" class="modal-backdrop">
				<button type="submit" aria-label="Close">close</button>
			</form>
		`;
    const input = dialog.querySelector("#calendar-url-input");
    const copyBtn = dialog.querySelector("#calendar-url-copy");
    const status = dialog.querySelector("#calendar-url-status");

    input.value = url;
    input.addEventListener("focus", () => input.select());
    input.addEventListener("click", () => input.select());

    function setStatus(text, kind) {
      status.textContent = text;
      status.className = "text-xs mt-2 h-4 " + (kind === "error" ? "text-error" : "text-success");
    }

    copyBtn.addEventListener("click", async () => {
      try {
        await navigator.clipboard.writeText(url);
        setStatus("Link copied to clipboard.", "success");
      } catch {
        input.focus();
        input.select();
        setStatus("Press ⌘C / Ctrl+C to copy.", "error");
      }
    });

    if (autoCopied) {
      setStatus("Link copied to clipboard.", "success");
    }

    dialog.addEventListener("close", () => dialog.remove());
    document.body.appendChild(dialog);
    dialog.showModal();
  }

  async function exportCalendarPayload(payload, button) {
    if (!payload || typeof payload !== "object") {
      window.showToast?.("No selections to export.", "warning");
      return;
    }

    if (button) window.setButtonLoading?.(button, true);

    try {
      const response = await fetch("/export", {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify(payload),
      });

      if (!response.ok) {
        let message = "Unknown error";
        try {
          const errorData = await response.json();
          if (errorData && errorData.error) message = errorData.error;
        } catch {}
        window.showToast?.("Failed to export calendar: " + message, "error");
        return;
      }

      const data = await response.json();
      let autoCopied = false;
      try {
        await navigator.clipboard.writeText(data.url);
        autoCopied = true;
      } catch {}
      showLinkModal(data.url, autoCopied);
    } catch (err) {
      window.showToast?.("Error exporting calendar: " + err.message, "error");
    } finally {
      if (button) window.setButtonLoading?.(button, false);
    }
  }

  window.exportCalendarPayload = exportCalendarPayload;
})();
```

- [ ] **Step 2: Verify the file lints clean**

Run: `cd /Users/cheuk/projects/ecal/esportscalendar && npx eslint static/js/export-calendar.js`
Expected: exit code 0, no warnings or errors.

- [ ] **Step 3: Verify nothing else broke**

Run: `cd /Users/cheuk/projects/ecal/esportscalendar && go build ./...`
Expected: exit code 0, no output (the new JS file isn't referenced by Go yet, so this is just a sanity check that Task 1 didn't accidentally touch anything else).

- [ ] **Step 4: Skip commit per user preference**

The user has asked not to commit during this branch's work. Move on to Task 2.

---

## Task 2: Refactor `preview.js` to use the shared helper, and load the helper from `preview-page.templ`

**Files:**

- Modify: `static/js/preview.js`
- Modify: `components/preview-page.templ:152-153` (script tags region near the end of the templ block)

**Context:** Task 1 added the helper but nothing loads it. This task makes `preview.js` delegate to the helper, proving the helper works end-to-end before Task 3 introduces a second consumer. After this task, `/preview` Export must continue to behave exactly as before.

- [ ] **Step 1: Replace the export logic in `preview.js`**

Open `/Users/cheuk/projects/ecal/esportscalendar/static/js/preview.js` and replace the entire file contents with this:

```js
// Bootstraps the "Preview" page. Converts UTC times to local, and wires up
// the Export Calendar button (delegating to window.exportCalendarPayload from
// export-calendar.js). Re-runs when the inner partial is swapped.
(function init() {
  const exportBtn = document.getElementById("export-calendar-btn");
  if (!exportBtn) return;

  // Each IIFE invocation owns its event listeners. Aborting any prior
  // controller cleans up listeners attached on a previous swap.
  if (window.__previewAbort) window.__previewAbort.abort();
  const controller = new AbortController();
  window.__previewAbort = controller;
  const { signal } = controller;

  // app.js is deferred; on cold load convertMatchTimesIn isn't defined yet.
  // htmx-driven swaps land here with readyState already 'complete'.
  const localize = () => window.convertMatchTimesIn?.(document);
  if (document.readyState === "loading") {
    document.addEventListener("DOMContentLoaded", localize, { once: true, signal });
  } else {
    localize();
  }

  function exportCalendar() {
    const previewSelections = sessionStorage.getItem("preview-selections");
    if (!previewSelections) {
      window.showToast?.("No selections found. Please go back and make your selections again.", "warning");
      return;
    }

    let payload;
    try {
      payload = JSON.parse(previewSelections);
    } catch {
      window.showToast?.("Selections are corrupted. Please go back and make your selections again.", "warning");
      return;
    }

    window.exportCalendarPayload?.(payload, exportBtn);
  }

  exportBtn.addEventListener("click", exportCalendar, { signal });

  // "Back to Selection" — re-POST stored game IDs to /lts so the leagues
  // & teams page is rebuilt server-side with the user's selections intact.
  const backBtn = document.getElementById("back-to-selection-btn");
  if (backBtn) {
    backBtn.addEventListener(
      "click",
      async () => {
        let stored = [];
        try {
          const raw = sessionStorage.getItem("selectedGameOptions");
          if (raw) stored = JSON.parse(raw);
        } catch {}

        if (!Array.isArray(stored) || stored.length === 0) {
          history.pushState({}, "", "/");
          location.assign("/");
          return;
        }

        backBtn.disabled = true;
        try {
          const response = await fetch("/lts", {
            method: "POST",
            headers: {
              "Content-Type": "application/json",
              "HX-Request": "true",
              "HX-Target": "page-content",
            },
            body: JSON.stringify({ options: stored }),
          });
          if (!response.ok) {
            window.showToast?.(`Navigation failed (${response.status}).`, "error");
            return;
          }
          const html = await response.text();
          const target = document.getElementById("page-content");
          target.innerHTML = html;
          target.classList.add("fade-in");
          window.executeScriptsIn?.(target);
          if (typeof htmx !== "undefined") htmx.process(target);
          history.pushState({}, "", "/lts");
          document.title = "Leagues & Teams - EsportsCalendar";
        } catch (err) {
          window.showToast?.("Network error: " + err.message, "error");
        } finally {
          backBtn.disabled = false;
        }
      },
      { signal },
    );
  }

  document.addEventListener(
    "keydown",
    (e) => {
      if (e.key !== "Enter") return;
      const tag = ((document.activeElement && document.activeElement.tagName) || "").toLowerCase();
      if (tag === "input" || tag === "textarea") return;
      e.preventDefault();
      exportCalendar();
    },
    { signal },
  );
})();
```

This is a strict refactor: the `showLinkModal` definition and the inline POST/clipboard logic are removed in favor of a call to `window.exportCalendarPayload(payload, exportBtn)`. The `Back to Selection` button, time-localization, and Enter-key shortcut are unchanged.

- [ ] **Step 2: Add the script tag to `preview-page.templ`**

Open `/Users/cheuk/projects/ecal/esportscalendar/components/preview-page.templ`. Find the trailing `<script src="/static/js/preview.js"></script>` (around line 19 of the `PreviewInner` templ block):

```go
templ PreviewInner(matches []dbtypes.GetFutureMatchesBySelectionsRow, showingPast bool, hideScores bool) {
	@ProgressIndicator(3)
	<div class="container mx-auto p-4">
		<div class="max-w-5xl mx-auto">
			@PreviewPageContent(matches, showingPast, hideScores)
		</div>
	</div>
	<script src="/static/js/preview.js"></script>
}
```

Replace the trailing script line so a helper script loads first:

```go
templ PreviewInner(matches []dbtypes.GetFutureMatchesBySelectionsRow, showingPast bool, hideScores bool) {
	@ProgressIndicator(3)
	<div class="container mx-auto p-4">
		<div class="max-w-5xl mx-auto">
			@PreviewPageContent(matches, showingPast, hideScores)
		</div>
	</div>
	<script src="/static/js/export-calendar.js"></script>
	<script src="/static/js/preview.js"></script>
}
```

Order matters: `export-calendar.js` must come before `preview.js` so `window.exportCalendarPayload` is defined when the preview IIFE runs.

- [ ] **Step 3: Regenerate the templ output**

Run: `cd /Users/cheuk/projects/ecal/esportscalendar && templ generate`
Expected: exit code 0, no errors. The file `components/preview-page_templ.go` will be updated to include the new script tag.

- [ ] **Step 4: Verify the build**

Run: `cd /Users/cheuk/projects/ecal/esportscalendar && go build ./...`
Expected: exit code 0, no output.

- [ ] **Step 5: Verify the JS lints clean**

Run: `cd /Users/cheuk/projects/ecal/esportscalendar && npx eslint static/js/preview.js static/js/export-calendar.js`
Expected: exit code 0, no warnings or errors.

- [ ] **Step 6: Manual smoke test the existing /preview Export flow**

Start the dev server: `cd /Users/cheuk/projects/ecal/esportscalendar && go run main.go` (or use `make dev` if running templ in watch mode).

In a browser:

1. Visit `/`, pick at least one game, click Continue.
2. On `/lts`, pick at least one league for one game, click Submit.
3. On `/preview`, click "Export Calendar".
4. Verify: a modal appears with a calendar URL, the URL is copied to the clipboard, and clicking Copy re-copies with a "Link copied" status line.
5. Click Done; verify the modal closes cleanly.

If anything regresses, the helper extraction is wrong — re-check Step 1 of this task and the script-tag order in Step 2.

- [ ] **Step 7: Skip commit per user preference**

Move on to Task 3.

---

## Task 3: Add the Export button and helper script tag to `calendar-page.templ`

**Files:**

- Modify: `components/calendar-page.templ:51-69` (the `hud-control-bar-actions` div) and `components/calendar-page.templ:152-153` (script-tag region)

**Context:** The calendar page's existing control bar already holds the Spoiler checkbox and the Tune button. Add a third action — Export — styled identically to the Tune button so the row stays visually balanced. Also load the new helper script before `calendar.js` so Task 4 can call into it.

- [ ] **Step 1: Add the Export button next to the Tune button**

Open `/Users/cheuk/projects/ecal/esportscalendar/components/calendar-page.templ`. Find this block (currently lines 51–69):

```go
<div class="hud-control-bar-actions">
	<label class="hud-mono text-xs uppercase tracking-widest flex items-center gap-2 cursor-pointer">
		<input
			type="checkbox"
			id="calendar-hide-scores"
			class="checkbox checkbox-primary checkbox-sm"
			if hideScores {
				checked
			}
		/>
		<span class="hidden sm:inline">Spoiler</span>
	</label>
	<button type="button" id="calendar-tune-open" class="hud-tune-btn" aria-haspopup="dialog" aria-controls="calendar-drawer">
		<svg xmlns="http://www.w3.org/2000/svg" class="w-4 h-4" fill="none" viewBox="0 0 24 24" stroke-width="1.75" stroke="currentColor" aria-hidden="true">
			<path stroke-linecap="round" stroke-linejoin="round" d="M10.5 6h9m-9 6h9m-9 6h9M3.75 6h.008v.008H3.75V6zm.375 0a.375.375 0 11-.75 0 .375.375 0 01.75 0zM3.75 12h.008v.008H3.75V12zm.375 0a.375.375 0 11-.75 0 .375.375 0 01.75 0zM3.75 18h.008v.008H3.75V18zm.375 0a.375.375 0 11-.75 0 .375.375 0 01.75 0z"></path>
		</svg>
		<span class="uppercase tracking-widest">Tune</span>
	</button>
</div>
```

Replace it with this version, which adds an Export button after the Tune button:

```go
<div class="hud-control-bar-actions">
	<label class="hud-mono text-xs uppercase tracking-widest flex items-center gap-2 cursor-pointer">
		<input
			type="checkbox"
			id="calendar-hide-scores"
			class="checkbox checkbox-primary checkbox-sm"
			if hideScores {
				checked
			}
		/>
		<span class="hidden sm:inline">Spoiler</span>
	</label>
	<button type="button" id="calendar-tune-open" class="hud-tune-btn" aria-haspopup="dialog" aria-controls="calendar-drawer">
		<svg xmlns="http://www.w3.org/2000/svg" class="w-4 h-4" fill="none" viewBox="0 0 24 24" stroke-width="1.75" stroke="currentColor" aria-hidden="true">
			<path stroke-linecap="round" stroke-linejoin="round" d="M10.5 6h9m-9 6h9m-9 6h9M3.75 6h.008v.008H3.75V6zm.375 0a.375.375 0 11-.75 0 .375.375 0 01.75 0zM3.75 12h.008v.008H3.75V12zm.375 0a.375.375 0 11-.75 0 .375.375 0 01.75 0zM3.75 18h.008v.008H3.75V18zm.375 0a.375.375 0 11-.75 0 .375.375 0 01.75 0z"></path>
		</svg>
		<span class="uppercase tracking-widest">Tune</span>
	</button>
	<button type="button" id="calendar-export-btn" class="hud-tune-btn" aria-label="Export calendar subscription">
		<svg xmlns="http://www.w3.org/2000/svg" class="w-4 h-4" fill="none" viewBox="0 0 24 24" stroke-width="1.75" stroke="currentColor" aria-hidden="true">
			<path stroke-linecap="round" stroke-linejoin="round" d="M3 16.5v2.25A2.25 2.25 0 005.25 21h13.5A2.25 2.25 0 0021 18.75V16.5M16.5 12L12 16.5m0 0L7.5 12m4.5 4.5V3"></path>
		</svg>
		<span class="uppercase tracking-widest">Export</span>
	</button>
</div>
```

The new button reuses the existing `hud-tune-btn` class so it inherits the dense, monogram-style treatment (see `static/css/app.css:1353`). The download arrow icon matches the one already used on `/preview`'s Export button. The `aria-label` is set explicitly because the visible "Export" text collapses to icon-only at viewport widths below 640px (per the existing `.hud-tune-btn` media query).

- [ ] **Step 2: Add the helper script tag before `calendar.js`**

Still in `components/calendar-page.templ`, find the trailing script tags (currently around lines 152–153):

```go
<script src="/static/js/game-selection.js"></script>
<script src="/static/js/calendar.js"></script>
```

Replace with:

```go
<script src="/static/js/game-selection.js"></script>
<script src="/static/js/export-calendar.js"></script>
<script src="/static/js/calendar.js"></script>
```

Order matters: `export-calendar.js` must load before `calendar.js` so `window.exportCalendarPayload` is defined when Task 4's click handler fires.

- [ ] **Step 3: Regenerate the templ output**

Run: `cd /Users/cheuk/projects/ecal/esportscalendar && templ generate`
Expected: exit code 0, no errors. `components/calendar-page_templ.go` is updated.

- [ ] **Step 4: Verify the build**

Run: `cd /Users/cheuk/projects/ecal/esportscalendar && go build ./...`
Expected: exit code 0, no output.

- [ ] **Step 5: Manual visual check**

Start the dev server (`go run main.go` or `make dev`) and visit `/calendar`.

Verify:

1. The Export button appears immediately to the right of the Tune button in the control bar.
2. Both buttons have identical height and styling (filled primary background).
3. At a narrow viewport (< 640px), the "Export" label collapses just like "Tune" does, leaving only the download icon.
4. Clicking the Export button does **nothing yet** (no console error). Task 4 wires up the handler.

If clicking the button produces a JavaScript console error like `exportCalendarPayload is not a function`, the script-tag order in Step 2 is wrong — re-check.

- [ ] **Step 6: Skip commit per user preference**

Move on to Task 4.

---

## Task 4: Wire the calendar Export button to call `exportCalendarPayload`

**Files:**

- Modify: `static/js/calendar.js` (add a click handler near the bottom of the IIFE, alongside the other action wirings)

**Context:** The calendar page already has a `collectSelections()` helper that returns the same `{gameId: {leagues, teams, maxTier}}` shape `/api/calendar` accepts. The same shape is what `/export` expects under the `selections` key. The Export button click handler builds `{selections, hideScores}` from current page state, refuses to fire on an empty selection (matching the pre-existing empty-state check in `refresh()`), and delegates to the shared helper.

- [ ] **Step 1: Read the existing calendar.js so you understand `collectSelections` and `hideScoresEl`**

Open `/Users/cheuk/projects/ecal/esportscalendar/static/js/calendar.js`. Confirm:

- `hideScoresEl` is captured at line 18: `const hideScoresEl = document.getElementById('calendar-hide-scores');`
- `collectSelections()` is defined at line 240 and returns an object whose values are `{leagues, teams, maxTier}` — same format the export endpoint expects.
- `Object.keys(selections).length === 0` is the empty-state predicate the existing `refresh()` uses (line 287).

If any of these have moved, adjust the inserted code to match the actual identifiers — but don't refactor anything beyond adding the export wiring.

- [ ] **Step 2: Add the export button wiring inside the IIFE**

Open `/Users/cheuk/projects/ecal/esportscalendar/static/js/calendar.js`. Find the `todayBtn?.addEventListener(...)` block (currently around lines 350–362) — it ends with the closing `);` of the listener registration, just before the `function openDayModal(dateKey) {` declaration around line 364.

Insert the following block immediately after `todayBtn?.addEventListener(...)` and before `function openDayModal(...)`:

```js
const exportBtn = document.getElementById("calendar-export-btn");
exportBtn?.addEventListener(
  "click",
  () => {
    const selections = collectSelections();
    if (Object.keys(selections).length === 0) {
      window.showToast?.("Pick at least one league or team before exporting.", "warning");
      return;
    }
    const hideScores = !!(hideScoresEl && hideScoresEl.checked);
    window.exportCalendarPayload?.({ selections, hideScores }, exportBtn);
  },
  { signal },
);
```

Why this placement: the listener uses `signal` (the IIFE's AbortController) so it auto-detaches on htmx swap, matching every other `addEventListener` in this file. Putting it next to the other top-level button wirings (`prevBtn`, `nextBtn`, `todayBtn`) keeps the action-handler section contiguous.

- [ ] **Step 3: Verify the JS lints clean**

Run: `cd /Users/cheuk/projects/ecal/esportscalendar && npx eslint static/js/calendar.js`
Expected: exit code 0, no warnings or errors.

- [ ] **Step 4: Manual end-to-end test the new flow**

Start the dev server and visit `/calendar`.

**Happy path:**

1. The page loads in default state (all games on, tier-1 leagues auto-selected by `initGameSelection`).
2. Click Export.
3. Verify: a modal appears with a calendar URL, the URL is copied to the clipboard, and the status line says "Link copied to clipboard."
4. Click Copy; the status line re-confirms.
5. Click Done; the modal closes.
6. Open the URL in a new tab — it should download an `.ics` file (proof the round-trip succeeded).

**Filter-applied path:**

1. Click Tune. In the drawer, toggle one game off, and tweak another game's leagues/teams.
2. Close the drawer; click Export.
3. Verify: a modal appears with a _different_ URL than before (because the payload's hash changed). Open it — the `.ics` should reflect the new filter set.

**Empty-state path:**

1. Click Tune. Toggle every game off, OR remove every league/team from every game.
2. Close the drawer; click Export.
3. Verify: a toast appears with "Pick at least one league or team before exporting." and **no** request is made to `/export` (check the Network tab).

**Hide-scores path:**

1. Toggle the Spoiler checkbox; click Export.
2. Open the returned `.ics` URL in a new tab; confirm match summaries omit scores (e.g., `TEAM A vs TEAM B` instead of `TEAM A 2-1 TEAM B`).

If any of these regress, double-check Step 2 (the click handler's payload shape) and re-run `npx eslint`.

- [ ] **Step 5: Smoke test that /preview Export still works**

After Task 4, both `/preview` and `/calendar` use the shared helper. Re-run the manual flow from Task 2 Step 6 (visit `/`, pick games, continue, pick leagues, submit, export) to confirm `/preview` Export still produces the same modal and a working URL.

- [ ] **Step 6: Skip commit per user preference**

Move on to Task 5.

---

## Task 5: Final verification sweep

**Files:** none modified — this task only runs verification commands.

**Context:** Catch anything the per-task spot-checks missed. If everything is green, the feature is ready to hand back to the user.

- [ ] **Step 1: Lint every JS file the plan touched (and the new one)**

Run: `cd /Users/cheuk/projects/ecal/esportscalendar && npx eslint static/js/export-calendar.js static/js/preview.js static/js/calendar.js`
Expected: exit code 0, no warnings.

- [ ] **Step 2: Build the Go binary**

Run: `cd /Users/cheuk/projects/ecal/esportscalendar && go build ./...`
Expected: exit code 0, no output.

- [ ] **Step 3: Confirm there are no stray un-regenerated templ files**

Run: `cd /Users/cheuk/projects/ecal/esportscalendar && templ generate && git status --short components/`
Expected: every modified `*.templ` has a corresponding `*_templ.go` modification listed. If `git status` shows a `*.templ` change with no matching `*_templ.go` change, run `templ generate` again and re-check.

- [ ] **Step 4: Diff review**

Run both, in sequence:

```sh
cd /Users/cheuk/projects/ecal/esportscalendar
git diff --stat static/js/ components/calendar-page.templ components/preview-page.templ components/calendar-page_templ.go components/preview-page_templ.go
git status --short static/js/ components/
```

Expected output from `git diff --stat` (tracked-file changes only — small numbers will vary):

```
 components/calendar-page.templ      | small change
 components/calendar-page_templ.go   | small change
 components/preview-page.templ       | small change
 components/preview-page_templ.go    | small change
 static/js/calendar.js               | small change
 static/js/preview.js                | larger change — full rewrite
```

Expected output from `git status --short` (must include the new untracked file):

```
?? static/js/export-calendar.js
```

If unexpected files appear in either listing, investigate before handing off.

- [ ] **Step 5: Do not commit; report completion**

The user will commit manually. Summarize the change for them: "Calendar page now has an Export button next to Tune. Both /calendar and /preview use a new shared helper at `static/js/export-calendar.js`. /preview behavior is unchanged."

---

## Open questions / things to flag back to the user if encountered

- **If `templ generate` fails** with a "templ binary not found" error, the engineer should note this and ask the user — `templ` is normally available via the project's tooling (`make dev` runs `templ generate -watch`) and a missing binary is an environment problem, not a plan problem.
- **If `npx eslint .` reports unrelated lint failures** in files this plan does not touch, do not fix them — report them to the user. Other in-flight branches may be responsible.
- **If `/export` returns a 4xx for a non-empty selection** (e.g., "too many leagues"), that is the server's `validateSelections` (see `middleware/utils.go:111`) doing its job. The toast surfaces the message — no plan change needed.
