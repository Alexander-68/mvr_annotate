# Annotations Release Notes

## 2026-07-09-2
1. Add recording inactive warning. Activated when user annotates without start video recording. Requires MVR FW 260709+.
2. Add aiScope live history graph: plots each class's moving-averaged score over time in a long button-styled strip. Drag the right edge to make it longer (shows more history); pinch or mouse-wheel to zoom the timeline; configurable dashed threshold line (default 50%). Position and size persist; collected AI data does not. Configured under "aiScope": { "graph": {...} } in mvr_annotate.json.
3. Overlay recording events on the aiScope graph: shaded bands mark when the MVR was recording, edge lines mark start/stop, and dashed ticks mark snapshots (fed from the device's study event stream). Colors are configurable under "aiScope": { "graph": {...} } as recordingFillColor / recordingEdgeColor / snapshotColor; omit a color to hide that element. The graph also now stays on screen after the AI stream stops so the trace can be reviewed.

## 2026-07-09
1. Submenues can be nested.
2. Submenues can be opened immediately on tap, not just on the long tap; configurable with "openOnTap": true. See example: Actions-Status-Disease.
3. Menu now contains a title.
4. Top-level menu cluster can be minimized into a button by tapping the title; minimization/expansion is not a reportable event for the Study timeline.
5. Example of the mvr_annotate.json configuration file shows almost the complete setup for colonoscopy annotations; overcomplicated, though.

