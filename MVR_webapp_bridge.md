# MVR web-app JavaScript bridge (`window.MvrOverlay`)

When an external web app is opened inside the MVR device (either in the normal
side-by-side pane or as the transparent **"Run over active Study"** overlay),
MVR injects a JavaScript bridge object into the page as `window.MvrOverlay`.

This lets a web page:

- **detect that it is running inside an MVR device** (rather than in an ordinary
  desktop/mobile browser), and
- read basic, non-sensitive **device identity** (device ID, hardware model, app
  and firmware version), and
- check whether a **paid feature** is activated on the device, and
- read the current active **study path**, and
- read the current **recording status**, and
- (overlay mode only) opt a touch gesture out of native click-through.

The bridge is attached to every external web app configured on the device, over
both `LOCAL` and `HTTP(S)` schemes. On any other browser the object simply does
not exist.

> Availability note: the bridge is a native
> [`@JavascriptInterface`](https://developer.android.com/reference/android/webkit/JavascriptInterface)
> object. All methods are synchronous and return primitive values (strings /
> numbers / booleans / null). Only methods annotated for JavaScript are
> callable; nothing else on the object is accessible.

---

## Detecting the MVR device

Feature-detect the bridge before using it. Its presence is itself the signal
that the page runs inside MVR:

```js
const onMvr = !!(window.MvrOverlay && window.MvrOverlay.getDeviceId);

if (onMvr) {
    // running inside an MVR device
} else {
    // running on an external PC / plain browser
}
```

---

## Device identity

### `window.MvrOverlay.getDeviceInfo() → string (JSON)`

Returns all device identity fields as a JSON string. Parse it with
`JSON.parse`. The `firmware` key is omitted when the firmware version is not
available on the device.

```js
const info = JSON.parse(window.MvrOverlay.getDeviceInfo());
// {
//   "deviceId":   "A1B2C3D4",   // device serial / ID (hex string)
//   "model":      "MVR-PRO",    // hardware model name (upper case)
//   "appVersion": 260704,       // app version code, integer (see note below)
//   "firmware":   "1.2.3"       // firmware version string (key omitted if unknown)
// }
```

The `deviceId` is the same value shown on the device **INFORMATION** screen
(field **ID**) and printed on the unit's label, so it is safe to display or log
in the web app.

### Individual getters

The same values are also available one at a time:

| Method | Returns | Notes |
| --- | --- | --- |
| `getDeviceId()` | `string` | Device serial / ID, hex string. Same as INFORMATION-screen **ID**. |
| `getModelName()` | `string` | Hardware model name, upper case. Same as INFORMATION-screen **HW**. |
| `getAppVersion()` | `number` | App version code, integer. See note below. |
| `getFirmwareVersion()` | `string \| null` | Firmware version, or `null` if unavailable. |

```js
const id    = window.MvrOverlay.getDeviceId();
const model = window.MvrOverlay.getModelName();
const app   = window.MvrOverlay.getAppVersion();       // e.g. 260704
const fw    = window.MvrOverlay.getFirmwareVersion();  // may be null
```

> **App version format.** The value is derived from the build date. It is the
> lower 6 digits of a `yyyyMMdd` build stamp (i.e. `yyMMdd`) — for a build made
> on 2026-07-04 the value is `260704`. Larger numbers indicate newer builds, so
> a numeric comparison is a reliable "newer than" check within the same century.

---

## Paid features

### `window.MvrOverlay.hasFeature(id) → boolean`

Returns `true` when the paid feature `id` is activated on this device. Feature
IDs are stable strings:

| Feature ID | Feature |
| --- | --- |
| `mvr_pro_4k` | 4K recording |
| `mvr_pro_pacs` | DICOM / PACS |
| `mvr_435_activation` | MVR unlock (the `435` in the name is historical) |
| `mvx_441_activation` | MVX441 |
| `mvr_aiscope` | AI Scope |

```js
if (window.MvrOverlay?.hasFeature?.('mvr_aiscope')) {
    // AI-Scope UI
}
```

Notes:

- Only the on/off state is exposed. The activation keys themselves are **not**
  readable from a web app.
- An unknown feature ID returns `false`.

---

## Active study

### `window.MvrOverlay.getCurrentStudyPath() -> string | null`

Returns the current active study folder path, or `null` when there is no active
study.

```js
const studyPath = window.MvrOverlay?.getCurrentStudyPath?.() ?? null;

if (studyPath) {
    // e.g. "CASE0001" or another MVR study folder path
}
```

Notes:

- The value is the study path used by MVR for the active study folder.
- This method is read-only. It does not create, start, finish, or change a
  study.

---

## Recording status

### `window.MvrOverlay.isRecordingActive() -> boolean`

Returns `true` when the main video recorder is currently active. A paused
recording is still considered active because recording has been started and the
current recording file/session has not been stopped yet.

```js
const recordingActive = window.MvrOverlay?.isRecordingActive?.() === true;
```

### `window.MvrOverlay.getRecordingState() -> string`

Returns the exact main recording state:

- `"STOPPED"` — no main recording is active.
- `"RUNNING"` — main recording is actively writing video.
- `"PAUSED"` — main recording is started but currently paused.

```js
const state = window.MvrOverlay?.getRecordingState?.() ?? "STOPPED";

if (state === "RUNNING" || state === "PAUSED") {
    // recording is active
}
```

Notes:

- These methods are read-only. They do not start, stop, pause, or resume
  recording.
- The status is the current in-device state at the moment the method is called;
  call it again when you need a fresh value.

---

## Study timeline: inject a custom event

During an active study MVR keeps a **timeline** of events (study start/finish,
snapshots, recordings, signal changes, …) as a `timeline.ndjson` file inside the
study folder. A web app can add its own **external** events to that timeline.

### `window.MvrOverlay.injectTimelineEvent(json) → boolean`

Records one custom event into the current study's timeline. The argument is a
**JSON object string**; its fields are stored alongside the event. The record is
written as an `"ext"` event with `"ip":"localhost"` and an `"ts"` timestamp
(epoch milliseconds) added by MVR:

```js
const ok = window.MvrOverlay?.injectTimelineEvent(
    JSON.stringify({ marker: "incision", note: "step 1" })
);
// -> resulting timeline line:
// {"ts":1751731200000,"ev":"ext","ip":"localhost","marker":"incision","note":"step 1"}
```

Returns:

- `true` — the event was accepted for recording.
- `false` — there is **no active study** (or it is already finished), or the
  argument is not a valid JSON object.

Notes:

- The study folder is created lazily. Injecting an event **triggers folder
  creation** (like a capture/recording does): the first event on its own is
  enough to materialize the study folder and its `timeline.ndjson` on disk.
- The reserved keys `ts`, `ev` and `ip` are always set by MVR and cannot be
  overridden by the supplied JSON.
- The call is best-effort and returns immediately; the actual disk write happens
  asynchronously.

---

## Active-study overlay: touch pass-through

Only relevant when the web app runs as the **"Run over active Study"** overlay
(transparent layer on top of the live camera preview, with native controls
still tappable underneath).

### `window.MvrOverlay.reportInteractive() → void`

By default a touch on the transparent overlay passes **through** to the native
control underneath. Call `reportInteractive()` from a page gesture handler to
claim that gesture for the web page instead, so the tap is **not** forwarded to
native controls.

```js
element.addEventListener('pointerdown', () => {
    // this tap is for the web UI, not for the native control behind it
    window.MvrOverlay?.reportInteractive();
});
```

Notes:

- The overlay's own chrome (FAB / toolbar) always blocks pass-through; you only
  need `reportInteractive()` for your page's own interactive content.
- The overlay and AOI (area-of-interest) editing are mutually exclusive, since
  AOI's drag handles can't receive touch through the overlay.
- Calling `reportInteractive()` outside overlay mode is harmless (no effect).

---

## Compatibility

- Always feature-detect (`window.MvrOverlay?.method`) — the object is absent in
  ordinary browsers and on MVR firmware predating this bridge.
- All fields are read-only; the bridge exposes no control over the device.
