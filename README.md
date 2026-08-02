# mvr_annotate

A small, dependency-free web overlay for MVR/MTR devices that lets you **annotate a
Study in real time** — tapping the on-screen buttons injects events into the active
Study's timeline while it records. It runs as a transparent overlay on top of the
live camera preview; see [`timeline.md`](timeline.md) for what the timeline unlocks.

## Using the overlay

Buttons live in **clusters** — a stack of buttons that moves as one unit.

- **Annotate** — tap a button to mark it (white text → enlarged yellow). Tapping
  injects a timeline event; a short tap marks a moment, a **long-press** (hold
  still) starts/ends a span. Tap a selected button again to finish it. Every
  select dings, every deselect dongs.
- **Move / resize** — drag anywhere on a cluster to move the whole group; pinch
  (two fingers) or mouse-wheel to zoom. Shove a cluster off a screen edge to
  **dock** it flush along that edge; pull it back off to undock. Twist two fingers
  to flip a free-floating cluster between a column and a row.
- **Submenus** — long-pressing a button that has modifiers pops a **submenu** of
  them (e.g. `Polyp` → `Cold Snare, EMR, …`), while your finger is still down.
  Some hosts open on a plain tap too (`openOnTap`). Submenus can nest.
- **Minimize** — tap a cluster's small id label (on its top edge) to collapse it
  to a single button; tap that button to restore. Minimize/restore is **not** a
  timeline event. Top-level clusters start minimized until you move them.

Positions, sizes, orientation, and minimized state are remembered locally per
browser; the annotations themselves go to the device timeline.

## Menu content — edit before use

[`mvr_annotate.json`](mvr_annotate.json) holds the **annotation markers** shown as
buttons (the clusters, their labels, and the submenus). The values committed here
are only an **example set** (colonoscopy segments and actions) — **edit
`mvr_annotate.json` to match your own procedure before packaging.** The rest of the
app (`index.html`) is generic and needs no changes. This file is the single source
of truth: if it fails to load, the page shows an error banner and builds no menus.

Shape (abbreviated):

```jsonc
{
  "clusters": [
    // exclusive: true => single-select; false => independent multi-select.
    { "id": "Segments", "exclusive": true,  "buttons": ["Illeum", "R.Colon", …] },
    { "id": "Actions",  "exclusive": false, "buttons": ["Withdrawal", "Injection", …] }
  ],
  // Each key is a host button's LABEL; long-pressing it pops a submenu of the
  // modifiers. "openOnTap": true also opens it on a plain tap. Bare array form
  // is equivalent to openOnTap:false.
  "submenus": {
    "Injection": ["Lift", "Hemostasis", …],
    "Status":    { "openOnTap": true, "modifiers": ["Normal", "Inaccessible", …] }
  }
}
```

### Recording-inactive warning

Optional top-level `recordingWarning` shows a dismissible banner when a user taps
an annotation while the device reports recording is stopped (requires MVR FW
260709+):

```json
"recordingWarning": {
  "text": "Recording is not active. Start recording on the MVR before adding timeline annotations.",
  "backgroundColor": "#f59e0b"
}
```

Omit the block, or leave either value empty, to disable it. The banner sits
centred near the bottom of the screen; **Dismiss** hides it for the current
browser session. Per cluster, add `"recordingWarning": false`
to opt that cluster (and its submenus) out of the warning — handy for
data-entry clusters used before recording starts. Defaults to `true`.

### aiScope vision-AI indicators (optional)

If the device runs on-board vision inference, add an `aiScope` block to display
live per-class scores over the camera feed:

```jsonc
"aiScope": {
  "id": "ai_classes",
  "movingAverage": 12,                 // packets to smooth the live % over
  "graph": { "id": "ai_graph", "windowSeconds": 30, "downsample": 3, … },
  "classes": [
    { "cls": 0, "label": "Blur", "color": "#ff69b4", "threshold": 50 },
    { "cls": 1, "label": "PooP", "color": "#8a5a2b", "threshold": 60 }
  ]
}
```

Each class square shows three lines: its label, an **above-threshold
accumulator** (0..100% — how much of the study its score has spent past the
class `threshold`), and the live smoothed %. While a video **records**, the
accumulator line switches to a **per-video** count (small red `REC` badge) that
resets each recording; pausing or stopping injects a summary marker
(`{"marker":"aiScope","event":"video_summary","classes":[…],"model":…}`) and
reverts the line to the per-study count. The optional `graph` plots each class's
score over time as a draggable strip, shading the spans where the device was
recording. Indicators and graph appear only while inference data flows.

## Packaging

Build the distribution zip from the three shipped files (nothing else — no
`node_modules`, no test files):

```bash
zip mvr_annotate.zip index.html fv.png mvr_annotate.json
```

`mvr_annotate.zip` must contain exactly:

- `index.html` — the app (interaction/layout logic)
- `fv.png` — the app icon
- `mvr_annotate.json` — **your** annotation markers

## Installing on an MVR/MTR device

1. Copy `mvr_annotate.zip` to a USB flash drive.
2. On the device, go to **Advanced Settings → Connections → External apps**.
3. Tap the **local_web** application to edit it.
4. Tap **Upload web** and choose `mvr_annotate.zip` from the dropdown.
5. Enable the action **Run over active Study**.
6. Set the app details (suggested):
   - **Name:** `Annotate`
   - **Description:** `Colonoscopy`
7. Tap **Update**.

Once installed, **mvr_annotate** runs as a transparent overlay over the active
Study, so you can annotate it (inject events into the Study timeline) in real
time while it records.
