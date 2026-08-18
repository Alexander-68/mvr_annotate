# MVR HomeWeb API (live study, AI, and device data)

The **HomeWeb** server hosts the self-hosted web apps of the device. Each one has
a slot and lives in its own sub-folder: `homeweb_<slot>.zip` in the iMave folder,
served at `http://<device>:3335/<slot>/` (slots 1–99 ship with the firmware,
100+ are uploaded by the user). In addition to serving static files it exposes
**Server-Sent Events (SSE)** endpoints that push live device events to the
browser as they happen — no polling required.

Reference your assets **relatively** (`href="styles.css"`), so the page works
under its sub-folder. A bundle that references them from the root
(`/assets/x.js`, the Vite/React default) still loads — the server falls back to
the `Referer` to find the app — but relative paths avoid relying on that.

Two independent SSE feeds exist:

| Feed | SSE endpoint | Poll fallback | Payload |
|------|--------------|---------------|---------|
| **Study / recording** | `GET /study/events` | `GET /study/latest` | one timeline event per message |
| **Vision-AI** | `GET /ai/events` | `GET /ai/latest` | one inference result per message |

Both are **unidirectional** (device → browser). The browser cannot push back
over these; to *write* an event into the study timeline from the page, use
`window.MvrOverlay.injectTimelineEvent(...)` (see
[`MVR_webapp_bridge.md`](MVR_webapp_bridge.md)). Device identity and paid-feature
activation are not served here either — they are read from the same bridge
(`getDeviceInfo()`, `hasFeature(id)`), which is not reachable from an ordinary
browser on the network.

> This document covers the **HomeWeb browser app** only. The transparent
> "Run over active Study" **overlay** WebView uses the separate, pull-only
> `window.MvrOverlay` bridge and does **not** receive these SSE feeds.

---

## Study / recording feed — `/study/events`

This is the feed to use for notifications about **snapshot**, **recording
start/stop**, and **recording pause/resume**, plus study lifecycle and signal
changes. It mirrors, line for line, exactly what the device records to the
study's `timeline.ndjson` — so the browser sees the same authoritative event
stream the device persists to disk.

### Event schema

Each SSE message's `data` is a single JSON object. Every object has:

- `ts` — event time, epoch milliseconds (number)
- `ev` — event name (string)

plus event-specific fields:

| `ev` | Meaning | Extra fields |
|------|---------|--------------|
| `study_start`  | A study session began | `folder_name`, `device_id` |
| `study_finish` | The study session ended | — |
| `rec_start`    | Recording started | `file_name` |
| `rec_pause`    | Recording paused | `file_name` |
| `rec_resume`   | Recording resumed | `file_name` |
| `rec_stop`     | Recording stopped | `file_name` |
| `rec_error`    | Recorder error | `file_name` |
| `snapshot`     | A snapshot/photo was saved | `file_name` |
| `signal_good`  | A camera input got signal | `input_name`, `signal_resolution` |
| `signal_lost`  | A camera input lost signal | `input_name` |
| `signal_change`| Input mode / view layout changed | `mode`, `inputs` (array) |
| `ext`          | External event injected via API or `injectTimelineEvent` | `ip`, plus caller fields |

> The `snapshot`, `rec_*`, `signal_*` set may grow — treat an unknown `ev` as a
> pass-through and switch only on the names you care about.

### Example

```js
const es = new EventSource('/study/events');

es.addEventListener('message', (e) => {
    const ev = JSON.parse(e.data);
    switch (ev.ev) {
        case 'rec_start':  onRecordingStarted(ev.file_name); break;
        case 'rec_stop':   onRecordingStopped(ev.file_name); break;
        case 'rec_pause':  onRecordingPaused(); break;
        case 'rec_resume': onRecordingResumed(); break;
        case 'snapshot':   onSnapshot(ev.file_name); break;
        // study_start / study_finish / signal_* / ext ...
    }
});
```

### Rebuilding current state on connect

When a browser connects **mid-study**, the server immediately replays the most
recent buffered events (up to a few hundred) so the page can reconstruct the
current recording/signal state — you do not have to wait for the next live
event. Fold over the replayed events on load to compute "am I recording right
now?" rather than assuming a clean start.

If you only need a single most-recent value (e.g. a lightweight poll from a page
that can't hold an SSE connection open), `GET /study/latest` returns the last
event object as JSON (or `{}` if none yet).

---

## Vision-AI feed — `/ai/events`

Real-time vision-AI inference results, one JSON object per message, forwarded
verbatim from the on-device native inference process. This is a **latest-only**
feed: a freshly connected browser receives the most recent result immediately,
and there is no history replay. `GET /ai/latest` is the poll fallback.

```js
const es = new EventSource('/ai/events');
es.onmessage = (e) => render(JSON.parse(e.data));
```

---

## Reconnection & delivery semantics (both feeds)

- **Auto-reconnect.** `EventSource` reconnects automatically if the connection
  drops; the server advertises a 3 s retry hint.
- **No duplicates across reconnect.** Every frame carries an SSE `id:`. On
  reconnect the browser automatically sends the last id back via the
  `Last-Event-ID` header, and the server resumes from the next event — you will
  not re-receive events you already processed. (This resume works within the
  server's replay window; after a very long disconnect, older buffered events
  may have aged out.)
- **Keep-alive.** A `: ping` comment line is sent every 15 s so idle connections
  and dead sockets are detected promptly. `EventSource` ignores comment lines;
  you never see them as messages.
- **Back-pressure.** Each client has a bounded per-connection queue; if a browser
  falls badly behind, the oldest queued frames are dropped rather than stalling
  the device. A slow page can therefore miss intermediate events — reconcile
  against state, don't assume every single frame is delivered under overload.
- **Overload.** The AI feed is latest-only by design, so drops there just mean
  "you got a newer frame instead."

---

## Availability / lifecycle notes

- The HomeWeb server (and therefore these endpoints) runs **only** when a
  `LOCAL`-type external web app is configured/enabled on the device. When it is
  off, the endpoints are unreachable.
- The server is **not** tied to a single study — it stays up across studies while
  the home app is enabled. One SSE connection therefore spans multiple studies:
  it delivers `study_finish` for one study and later `study_start` for the next
  over the same stream. **Reset your per-study UI state on `study_start`** (and
  keep in mind the replay buffer on connect may include the tail of a just-ended
  study). A connection actually closing means the home app was disabled or the
  device restarted — treat that as "reconnect", which `EventSource` does for you.
- Both feeds share the same origin/port as the served web app (`:3335`) and live
  at the server root, shared by every hosted app, so
  `new EventSource('/study/events')` just works from any hosted page. A path
  relative to the app's own sub-folder (`new EventSource('study/events')`, which
  resolves to `/<slot>/study/events`) is served as well.

---

## Implementation pointers (device side)

- `SseHub` (`app/src/mvr/.../api/SseHub.kt`) — generic in-process fan-out with a
  replay buffer and monotonic event ids; one instance per feed.
- `HomeWebServer` (`app/src/mvr/.../api/HomeWeb.kt`) — routes the endpoints,
  `SseStream` writes the SSE wire format (`id:` / `data:` / heartbeat).
- `AiVisionUdpBridge` (`app/src/mvr/.../api/AiVisionUdpBridge.kt`) — UDP → hub
  adapter for the AI feed.
- `StudyTimeline` (`app/src/main/.../study/StudyTimeline.kt`) — the single source
  of truth for study events; every line it writes is mirrored to the study hub
  via `App.homeWebEventSink`, which `HomeWebServer` installs while it is running.
