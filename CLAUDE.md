# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## What this is

A tiny, dependency-free static web project — no package manager or test framework, and the only "build" is zipping the files for distribution (see below). Everything lives in a single self-contained HTML file (inline `<style>` and `<script>`, no external assets).

- `index.html` — the actual page: a fullscreen transparent `<canvas>` overlay carrying **button clusters** on top. Uses the Pointer Events API (`pointerdown`/`pointermove`/`pointerup`/`pointercancel`) to unify mouse, touch, and pen input in one code path, with a movement threshold (`DRAG_THRESHOLD` in the script) to distinguish a tap from a drag. See the button-cluster section below. It holds **only the interaction/layout logic** — the menu *content* lives in `mvr_annotate.json` (below).
- `mvr_annotate.json` — **the menu content**, split out from the code so labels/clusters/submenus can be edited without touching the logic. `index.html` `fetch()`es it once at startup (`init()`) and builds the UI from it (`buildFromConfig`); see [Menu content](#menu-content-mvr_annotatejson). This file is the **single source of truth** — there is no embedded fallback: if it can't be fetched/parsed the page **fails loud** with an on-screen banner (`showConfigError`) and builds no menus.
- `favicon.svg` — the page icon, referenced by `index.html`.

## Running / viewing

Just open the `index.html` file with a web browser.

## Build step

Package the shipped files into `mvr_annotate.zip`:

```bash
zip mvr_annotate.zip index.html favicon.svg mvr_annotate.json
```

> **Loading note:** because the page `fetch()`es `mvr_annotate.json` at startup, opening `index.html` directly via `file://` may be blocked by CORS in some browsers/WebViews (the fail-loud banner appears). Serve the folder over HTTP (e.g. `python3 -m http.server`) when viewing locally; on the MVR device it is served over the `LOCAL`/`HTTP(S)` scheme so `fetch` works.

## Button clusters (the core UI idea)

The interactive UI is organized into **clusters**. A cluster is a vertically stacked group of buttons that behaves as one unit:

- **Grouped drag** — dragging anywhere on a cluster (a button or the gap between them) moves the *whole cluster*, never an individual button. Position is held as explicit `left`/`top` on the cluster element. The whole (scaled) box is clamped inside the canvas, and clusters **cannot overlap** — a drag slides along whichever axis stays clear of the other cluster (`hitsOtherCluster`).
- **Orientation (vertical ⇄ horizontal) — a persistent, sticky property** — a cluster is a vertical stack (column) by default; a **horizontal row** is `.cluster.horizontal` (same buttons, the shared `-2px` edge and rounded corners move to the X axis). Orientation lives in `state.horizontal`, is **persisted** (`saveLayout`), and is **sticky** — it changes only two ways, and otherwise stays put:
  1. **Two-finger twist** — rotating the two pointers past `TWIST_FLIP_RAD` (45°) flips orientation (`twistDuringPinch`). This is how you make a **free-floating horizontal** menu, or turn one back. See the two-finger gesture note under Grouped zoom.
  2. **Docking to an edge** — docking lays the cluster *along* its edge, so orientation follows the edge: **top/bottom → row**, **left/right → column**. Undocking is sticky (the orientation the edge gave it stays; pull-off does **not** revert it — twist to change).

  `setOrientation()` toggles the class, which reflows synchronously, so callers re-measure the box immediately after to re-clamp.
- **Docking to any of the 4 edges (`state.dock` ∈ `top|bottom|left|right|null`)** — while single-finger dragging, shoving one of the cluster's edges off-screen past `EDGE_FORCE` px snaps it flush to that edge and orients it along the edge (see above). The detection (`if (state.dock === null)` branch of the drag handler) takes the **largest** of the four edge overshoots so a corner picks one edge, `setOrientation()`s to match, then snaps to the slot and re-bases the drag onto it. While docked, the cluster **slides along its edge** to follow the finger and is pinned on the perpendicular axis (`dockSlot`); it **undocks** (keeping orientation) once the finger pulls it off the edge by more than `EDGE_FORCE`. **Only top/bottom docks stack** against same-edge siblings (`dockAnchor` — rest on the floor below a top row / hang from the ceiling above a bottom row); left/right docks just sit flush and rely on the general no-overlap slide. `reflowIntoViewport` re-pins each dock edge after a viewport resize.
- **Grouped zoom + two-finger manipulation** — a two-finger gesture is a **combined manipulation**: **pinch** (spread) zooms, **twist** (rotate) flips orientation, and **sliding both fingers** translates the cluster — all at once, in one gesture (`beginPinch` / `twistDuringPinch` / `applyZoom`, driven off the fingers' distance, angle, and midpoint). Because it's free-form manipulation, starting a two-finger gesture **undocks** the cluster (orientation stays, sticky). Zoom is anchored to the midpoint (or the cursor, for mouse wheel) so the cluster zooms toward the focal point; scale is clamped to `[MIN_SCALE, MAX_SCALE]` **and** capped so the box can't grow past the canvas edges; a zoom/translate step that would overlap the other cluster is rejected. A twist-flip re-clamps and re-bases the pinch (`rebasePinch`) so zoom continues smoothly across the orientation change. Two-finger drag is thus the way to **move a menu freely while keeping its orientation** (e.g. reposition a free-floating horizontal menu).
- **Selection (per-cluster exclusivity is configurable)** — tapping a button marks it selected (white text → enlarged yellow text, yellow outline). A cluster created with `exclusive` (the default) allows only one selected button at a time — selecting another clears the previous one; a cluster created with `exclusive = false` lets every button toggle independently, so multiple can be selected at once. **Clusters are independent**: each tracks its own position, scale, and selection. The `exclusive` flag is the 5th arg to `createCluster`; `state.selected` holds the last-selected button (only meaningful for exclusive clusters), and the pointer-up deselect check keys off the button's own `.selected` class so non-exclusive toggling works.
- **Select/deselect + long-press, and the timeline `status`** — pressing a button toggles its selection, and whether the press was a short tap or a **long-press** (held still, no drag, for `LONG_PRESS_MS`) tags the injected timeline event. All four combinations sound a **ding on select** and a lower **"dong" on deselect** (`playDong`, an octave below `playDing`). The event is always `{"marker": <label>}` plus an optional `status`:
  - short tap, not selected → **select**, no `status` (bare `{marker}`)
  - short tap, already selected → **deselect**, `status: "finish"`
  - long-press, not selected → **select**, `status: "on"`
  - long-press, already selected → **deselect**, `status: "off"`

  A `setTimeout` started on `pointerdown` fires at `LONG_PRESS_MS`; a drag (crossing `DRAG_THRESHOLD`) or a second finger cancels it first. **A long-press resolves the moment that timer fires — mid-hold, while the finger is still down** (`fireLongPress`), not on release: you get immediate feedback and never have to guess how long to hold. Resolving mid-hold **locks the parent cluster's drag** (`state.dragLocked`) so the same gesture can't also reposition it, and marks `state.holdActionDone` so pointer-up is a no-op. To reposition instead, start dragging **before** `LONG_PRESS_MS` — the early drag cancels the timer. **Short taps** (both bare-select and `finish`-deselect) still resolve on **pointer-up**. `injectEvent()` builds the JSON. This mid-hold behavior is uniform across every button, including [submenu hosts](#pop-up-submenus-host--modifiers).
- **Pop-up submenus (host -> modifiers, nested)** — any button whose label is a key in config `submenus` can own a **transient submenu cluster**. This is not limited to top-level clusters: when a submenu opens, its buttons are checked against the same `submenus` map, so a submenu button can become a sub-submenu host without changing the config shape. Each submenu is a normal cluster built by `createCluster` (so it drags/pinches/flips and calls `reportInteractive()`), created with `transient = true` — it is **not built at page load**, its **selection** is discarded on close, and its **position/zoom/orientation persist** under a stable path-derived `id`. First-level submenu buttons inject `{"marker":<host>,"modifier":<label>,status?}`; deeper buttons keep the root marker and append `modifier2`, `modifier3`, etc. Example: `Injection -> Hemostasis -> Hemoclip` injects `{"marker":"Injection","modifier":"Hemostasis","modifier2":"Hemoclip"}`. **Spawn placement**: on open it restores its saved spot if one exists and is still clear; otherwise `placeSubmenuInOpenSpace()` finds open space that doesn't overlap any other cluster — preferring beside the host (right, left, below, above), then a grid scan of the canvas, and only sitting beside the host on top of another cluster if nothing is clear. The chosen spot is saved on open, and every drag/zoom re-saves it. Lifecycle: the submenu opens **mid-hold, while the finger is still down** — `fireLongPress(state)` calls `openSubmenu(host)` the moment the hold crosses the threshold. `closeSubmenu(host)` recursively removes descendant submenus, removes their host registrations, removes the element, and splices it out of `clusters`. A long-hold on a submenu host **opens** it whether the host is currently unselected (selects it, reports `on`) or already selected with no submenu open (keeps it selected, re-reports `on`); it **closes** (deselect + report `off`) when the submenu is already open. Short taps still resolve on **pointer-up**, keyed on `submenuHosts.get(btn)`: a short tap selects an unselected host bare (no submenu) or finishes + closes a selected one (`finish`). **Opt-in `openOnTap`** still works at every level. To prevent endless recursive popups, a submenu button is not registered as a host when its label is already in that open path.
- **Tap vs. drag** — a pointer that moves less than `DRAG_THRESHOLD` counts as a tap (toggles selection); more counts as a drag (reposition). A two-finger gesture is always a pinch, never a select.
- **Uniform button size** — every button in a cluster has the **same box size**: there's no fixed width (buttons hug the text), so the cluster is as wide as its widest label and the flex column's default `align-items: stretch` makes every button match that width. `font-size`/`font-weight` are constant (they drive layout), so selecting a button never changes its box or reflows the stack. The label lives in an inner `<span class="label">`; the idle label sits at `transform: scale(0.8)` and the selected one at `scale(1)`, so the selected text enlarges *within its existing button area* rather than growing the button. "Bold" emphasis is faked with an extra text-shadow (not a weight change) to keep layout fixed, and `overflow: hidden` guards against spill.
- **Tight vertical stacking** — `gap: 0` and a `-2px` top margin on every button after the first overlap adjacent borders into a single shared line: one button's bottom outline *is* the next button's top outline. Internal corners are square; only the cluster's outer corners are rounded. The selected button gets `z-index: 1` so its full yellow outline paints above the shared edges.
- **Appearance + minimize** — buttons have a transparent background with white text (yellow when selected), a `2px` border, and a text-shadow for legibility over the live camera preview. Horizontal padding is kept tight (`10px`) so there's little blank space beside the labels. Non-transient button clusters imprint their `id` as a small label on the upper outline of the first menu element; tapping that label minimizes the cluster into one same-style id button, and tapping that id button restores the full cluster. Transient submenus imprint a passive parent-label title on the upper outline (e.g. `Injection`, `Hemostasis`) but have no minimize control and never show the internal path-derived layout id. Minimize/restore is UI-only: it calls `reportInteractive()` through the normal pointer path, but it never calls `injectTimelineEvent()`. Minimized/full state is saved with the cluster layout, and top-level button clusters default to minimized when no saved state exists.

Clusters are built in JS by `createCluster(id, labels, left, top, exclusive)` from a plain array of label strings — but **you don't edit those calls to change menus**; the button names, counts, cluster count, and submenus all come from `mvr_annotate.json` (see [Menu content](#menu-content-mvr_annotatejson)). `buildFromConfig()` reads the config and issues the `createCluster` calls for you.

## Menu content (`mvr_annotate.json`)

All menu *content* — the clusters, their buttons, and the long-press submenus — lives in `mvr_annotate.json`, kept separate from the interaction logic in `index.html`. **To add/rename/reorder menus, edit this file only** (then re-zip; no code change needed). `index.html` loads it once at startup via `fetch()` (`init()`) and builds the UI with `buildFromConfig()`. It is the single source of truth — a load/parse failure fails loud (see the file list above), not a silent fallback.

Shape:

```jsonc
{
  "clusters": [
    // exclusive omitted/true => single-select; false => independent multi-select.
    // optional "left"/"top" override the default spawn spot (else 40,100 for the
    // first cluster; later clusters auto-place toward the right edge, clear of it).
    // "recordingWarning": false opts the cluster (and its submenus) out of the
    // not-recording warning; omitted/true keeps it (the default).
    { "id": "segments", "exclusive": true,  "buttons": ["Illeum", "R.Colon", ...] },
    { "id": "actions",  "exclusive": false, "buttons": ["Withdrawal", "Injection", ...] }
  ],
  // Each key is a host button's LABEL. The host can live in a top-level cluster
  // or inside another submenu; long-pressing it pops an exclusive submenu of the
  // listed modifiers. A submenu's persistence key/id is derived from its open
  // path, so renaming a host or ancestor resets that saved submenu position.
  //
  // A submenu value is EITHER the modifier array directly, OR an object
  // { "modifiers": [...], "openOnTap": true }. `openOnTap` (default false) makes
  // the submenu pop on a plain SHORT TAP of the host too, not just a long-press
  // — the tap does the normal bare-select AND opens the submenu; the next short
  // tap deselects (`finish`) and closes it. Long-press behaviour is unchanged.
  // The bare-array form is equivalent to openOnTap:false.
  "submenus": {
    "Injection": ["Lift", "Hemostasis", ...],                       // array form => openOnTap:false
    "Biopsy":    { "openOnTap": true, "modifiers": ["Forceps", ...] },
    // If "Hemostasis" appears inside another submenu, this same entry makes it
    // a nested host there too.
    "Hemostasis": ["Hemoclip", "Thermal", ...]
  }
}
```

Notes:
- `id` is the cluster's stable **layout-persistence key** (`saveLayout`/`restoreLayout`) — changing it orphans that cluster's saved position/zoom/orientation.
- A cluster's `id` must be unique; submenu ids are path-derived (`<host-slug>-modifier`, `<root-slug>-<modifier-slug>-modifier`, ...), and must not collide with a cluster `id`.

Current content:

- Cluster `segments` (exclusive): `Illeum, R.Colon, Tv.Colon, L.Colon, S.Colon, Rectum`
- Cluster `actions` (non-exclusive selection): `Status, Withdrawal, Injection, Hemostasis, Biopsy, Polyp`
- Injection submenu (transient, exclusive; long-press Injection): `Lift, Hemostasis, Botox, Steroid, Tattoo, Contrast`
- Hemostasis submenu (transient, exclusive; long-press Hemostasis): `Hemoclip, Thermal, APC, Injection, Band, Topical, Surgical`
- Biopsy submenu (transient, exclusive; long-press Biopsy): `Forceps, FNA/FNB, Brush, Snare, Suction`
- Polyp submenu (transient, exclusive; long-press Polyp): `Forceps, Cold Snare, Hot Snare, EMR, ESD, EFTR, APC/Ablation, Part.Resected, Fully Resected, Surgical`
- Status submenu (transient, exclusive; tap or long-press Status): `Normal, Inaccessible, Not Explored, Ulcer.Colitis, Infect.Colitis, Ischem.Colitis, Crohn's, Diverticulosis, Diverticulitis, Hemorrhoids, Cancer/Tumor`

## Important: notifying the native Android host

This page is loaded as a transparent overlay on top of native Android UI. **Any interactive element MUST call:**

```js
window.MvrOverlay?.reportInteractive();
```

**on the action that begins the interaction** (e.g. at the top of the `pointerdown` handler, and on `wheel` for zoom) — see the calls in the cluster handlers in `index.html`. This tells the native Android app that the gesture landed on a web-UI element, so it doesn't let the touch propagate through to the native UI underneath. Without this call, taps on web elements would fall through to Android as if the overlay wasn't there. This rule is unconditional: whenever you add a new interactive element or gesture, wire up `reportInteractive()` on its interaction-start event.

**Timeline-event payload is open-ended.** `window.MvrOverlay?.injectTimelineEvent(<json-string>)` (built by `injectEvent()`) accepts an arbitrary JSON object — the bridge stores/forwards it as-is and does **not** limit the number or names of fields. The current `{marker, modifier?, modifier2?, modifier3?, status?}` shape is a *web-side convention*, not a bridge constraint. Deeper submenu structures are safe to emit whenever the web UI wants them — no native change needed to carry extra fields.

## Key implementation details in index.html

- Canvas is sized to `window.innerWidth/innerHeight * devicePixelRatio` and uses `ctx.setTransform(dpr, 0, 0, dpr, 0, 0)` so drawing coordinates stay in CSS pixels while the backing store matches the display's real resolution.
- `html, body { overflow: hidden }` plus `#stage`/`canvas` using `position: fixed/absolute; inset: 0` keeps the canvas pinned edge-to-edge with no scrollbar-induced gaps.
- Each cluster's position is stored as explicit `left`/`top` pixel values with `transform-origin: top left` so the origin stays pinned while `transform: scale(...)` zooms it. Dragging updates `left`/`top` directly (screen-pixel deltas map 1:1 regardless of scale); `clampOrigin()` keeps the cluster's top-left corner reachable, and the `resize` handler re-clamps after a viewport resize.
- Per-cluster gesture state lives in a `state` object with a `pointers` map (pointerId → position) and a `mode` of `null` (pending) / `'drag'` / `'pinch'`. A second pointer switches to pinch and cancels any pending tap; lifting back to one pointer rebases the drag so the cluster doesn't jump.
- `touch-action: none` on the cluster and its buttons prevents the browser from hijacking touch gestures as scroll/zoom during drag/pinch; the `wheel` handler is registered `{ passive: false }` and calls `preventDefault()` so wheel zoom doesn't scroll the page.
