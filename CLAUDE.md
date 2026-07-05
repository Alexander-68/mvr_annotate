# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## What this is

A tiny, dependency-free static web project — no package manager or test framework, and the only "build" is zipping the files for distribution (see below). Everything lives in a single self-contained HTML file (inline `<style>` and `<script>`, no external assets).

- `index.html` — the actual page: a fullscreen transparent `<canvas>` overlay carrying **button clusters** on top. Uses the Pointer Events API (`pointerdown`/`pointermove`/`pointerup`/`pointercancel`) to unify mouse, touch, and pen input in one code path, with a movement threshold (`DRAG_THRESHOLD` in the script) to distinguish a tap from a drag. See the button-cluster section below.
- `favicon.svg` — the page icon, referenced by `index.html`.

## Running / viewing

Just open the `index.html` file with a web browser.

## Build step

Package the shipped files into `mvr_classify.zip`:

```bash
zip mvr_classify.zip index.html favicon.svg
```

## Button clusters (the core UI idea)

The interactive UI is organized into **clusters**. A cluster is a vertically stacked group of buttons that behaves as one unit:

- **Grouped drag** — dragging anywhere on a cluster (a button or the gap between them) moves the *whole cluster*, never an individual button. Position is held as explicit `left`/`top` on the cluster element. The whole (scaled) box is clamped inside the canvas, and clusters **cannot overlap** — a drag slides along whichever axis stays clear of the other cluster (`hitsOtherCluster`).
- **Orientation flip (vertical ⇄ horizontal)** — a cluster is a vertical stack by default. Dragging it against the **top or bottom** canvas edge and *forcing it over* (over-dragging past the edge by `EDGE_FORCE` px) flips the whole cluster to a **horizontal row** (`.cluster.horizontal`: same buttons, the shared `-2px` edge and rounded corners just move to the X axis). It stays horizontal while docked against that edge and reverts to the vertical stack as soon as it's dragged back away. Detection lives in the drag branch of `pointermove`: it compares the *desired* (unclamped) top against the top/bottom bounds, and `setOrientation()` toggles the class (which reflows synchronously, so the box is re-measured immediately after to re-clamp). Left/right edges are unaffected — they never flip.
- **Grouped zoom** — pinch (two pointers) or mouse wheel scales the whole cluster via a `transform: scale`, so button size and text size grow/shrink together. Zoom is anchored to the pinch midpoint / cursor so the cluster zooms toward the focal point. Scale is clamped to `[MIN_SCALE, MAX_SCALE]` **and** capped so the box can never grow past the canvas edges; a zoom step that would overlap the other cluster is rejected (`applyZoom`).
- **Single selection per cluster** — tapping a button plays the ding and marks it selected (white text → enlarged yellow text, yellow outline). Selecting another button in the same cluster clears the previous one. **Clusters are independent**: each tracks its own position, scale, and selection.
- **Tap vs. drag** — a pointer that moves less than `DRAG_THRESHOLD` counts as a tap (select); more counts as a drag (reposition). A two-finger gesture is always a pinch, never a select.
- **Uniform button size** — every button in a cluster has the **same box size**: there's no fixed width (buttons hug the text), so the cluster is as wide as its widest label and the flex column's default `align-items: stretch` makes every button match that width. `font-size`/`font-weight` are constant (they drive layout), so selecting a button never changes its box or reflows the stack. The label lives in an inner `<span class="label">`; the idle label sits at `transform: scale(0.8)` and the selected one at `scale(1)`, so the selected text enlarges *within its existing button area* rather than growing the button. "Bold" emphasis is faked with an extra text-shadow (not a weight change) to keep layout fixed, and `overflow: hidden` guards against spill.
- **Tight vertical stacking** — `gap: 0` and a `-2px` top margin on every button after the first overlap adjacent borders into a single shared line: one button's bottom outline *is* the next button's top outline. Internal corners are square; only the cluster's outer corners are rounded. The selected button gets `z-index: 1` so its full yellow outline paints above the shared edges.
- **Appearance** — buttons have a transparent background with white text (yellow when selected), a `2px` border, and a text-shadow for legibility over the live camera preview. Horizontal padding is kept tight (`10px`) so there's little blank space beside the labels. Clusters are **not** named or tagged — just buttons.

Clusters are built in JS by `createCluster(labels, left, top)` from a plain array of label strings, so **button names, counts, and the number of clusters are all configurable** — edit the arrays / `createCluster` calls near the bottom of the script. Current clusters:

- Cluster 1: `Illeum, R.Colon, Tv.Colon, L.Colon, S.Colon, Rectum`
- Cluster 2: `Injection, Withdrawal, Biopsy`

## Important: notifying the native Android host

This page is loaded as a transparent overlay on top of native Android UI. **Any interactive element MUST call:**

```js
window.MvrOverlay?.reportInteractive();
```

**on the action that begins the interaction** (e.g. at the top of the `pointerdown` handler, and on `wheel` for zoom) — see the calls in the cluster handlers in `index.html`. This tells the native Android app that the gesture landed on a web-UI element, so it doesn't let the touch propagate through to the native UI underneath. Without this call, taps on web elements would fall through to Android as if the overlay wasn't there. This rule is unconditional: whenever you add a new interactive element or gesture, wire up `reportInteractive()` on its interaction-start event.

## Key implementation details in index.html

- Canvas is sized to `window.innerWidth/innerHeight * devicePixelRatio` and uses `ctx.setTransform(dpr, 0, 0, dpr, 0, 0)` so drawing coordinates stay in CSS pixels while the backing store matches the display's real resolution.
- `html, body { overflow: hidden }` plus `#stage`/`canvas` using `position: fixed/absolute; inset: 0` keeps the canvas pinned edge-to-edge with no scrollbar-induced gaps.
- Each cluster's position is stored as explicit `left`/`top` pixel values with `transform-origin: top left` so the origin stays pinned while `transform: scale(...)` zooms it. Dragging updates `left`/`top` directly (screen-pixel deltas map 1:1 regardless of scale); `clampOrigin()` keeps the cluster's top-left corner reachable, and the `resize` handler re-clamps after a viewport resize.
- Per-cluster gesture state lives in a `state` object with a `pointers` map (pointerId → position) and a `mode` of `null` (pending) / `'drag'` / `'pinch'`. A second pointer switches to pinch and cancels any pending tap; lifting back to one pointer rebases the drag so the cluster doesn't jump.
- `touch-action: none` on the cluster and its buttons prevents the browser from hijacking touch gestures as scroll/zoom during drag/pinch; the `wheel` handler is registered `{ passive: false }` and calls `preventDefault()` so wheel zoom doesn't scroll the page.
