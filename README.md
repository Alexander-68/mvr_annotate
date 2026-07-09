# mvr_annotate

A small web overlay for MVR/MTR devices that lets you **annotate a Study in real
time** — tapping the on-screen buttons injects events into the active Study's
timeline while it records.

## Menu content — edit before use

[`mvr_annotate.json`](mvr_annotate.json) holds the **annotation markers** shown as
buttons (the clusters, their labels, and the long-press submenus). The values
committed here are only an **example set** (colonoscopy segments and actions) —
**edit `mvr_annotate.json` to match your own procedure before packaging.** The
rest of the app (`index.html`) is generic and needs no changes.

Optional `recordingWarning` config shows a dismissible warning when a user taps
an annotation while the MVR bridge reports that recording is stopped:

```json
"recordingWarning": {
  "text": "Recording is not active. Start recording on the MVR before adding timeline annotations.",
  "backgroundColor": "#f59e0b"
}
```

Omit this block, or leave either value empty, to disable the warning. The warning
can be dragged and resized with pinch-zoom or the mouse wheel; its position and
scale are remembered locally. Tapping **Dismiss** hides it for the current
browser session.

## Packaging

Build the distribution zip from the three shipped files (nothing else — no
`node_modules`, no test files):

```bash
zip mvr_annotate.zip index.html favicon.svg mvr_annotate.json
```

`mvr_annotate.zip` must contain exactly:

- `index.html` — the app (interaction/layout logic)
- `favicon.svg` — the app icon
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
