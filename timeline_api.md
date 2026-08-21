## Study timeline

During an active study MVR records a **timeline** of events into a file
`timeline.ndjson` in the study folder (one per enabled storage, next to the
`study_info.yaml` file). The file is written in
[NDJSON](https://github.com/ndjson/ndjson-spec) format: one JSON object per line.

The file is created when the study folder is created — lazily, when the first
file is saved into the study **or** when the first timeline event is injected via
`PUT /api/study/event` / the web-app bridge — and every buffered event from the
start of the study is flushed to it then. If a finished study is later re-opened
to append new recordings, a new `study_start` … `study_finish` block is appended
after the existing content.

Every record has an `"ts"` field (event time, epoch milliseconds) and an `"ev"`
field (event name). The first record is always `study_start`, the last is
`study_finish`.

| `ev` | Extra fields | Meaning |
| --- | --- | --- |
| `study_start` | `folder_name`, `device_id` | Study started (first record). `folder_name` is the study folder path; `device_id` is the device serial number. |
| `study_finish` | – | Study finished (last record). |
| `signal_good` | `input_name`, `signal_resolution` | Input signal is present/stable (also emitted once right after `study_start` when the camera opens). |
| `signal_lost` | `input_name` | Input signal was lost. |
| `signal_change` | `mode` (`single`/`split`), `inputs` (array of input names) | User changed input or swapped camera views. |
| `snapshot` | `file_name` | Snapshot image was saved. |
| `rec_start` | `file_name` | Video recording started. |
| `rec_pause` / `rec_resume` | `file_name` | Recording paused / resumed. |
| `rec_stop` | `file_name` | Recording stopped. |
| `rec_error` | `file_name` | Recording error. |
| `error` | `message` | Other error during the study. |
| `ext` | `ip`, plus custom fields | External event injected via `PUT /api/study/event` or the web-app bridge (`ip` is the client IP, or `localhost` for the bridge). |

`file_name` is the file name only, without the study-folder path.

Example:
```
{"ts":1751731200000,"ev":"study_start","folder_name":"20260705_083129_6d244bd0","device_id":"6d244bd0"}
{"ts":1751731200500,"ev":"signal_good","input_name":"IN1 DP","signal_resolution":"1920x1080"}
{"ts":1751731205000,"ev":"rec_start","file_name":"V0001.mp4"}
{"ts":1751731260000,"ev":"snapshot","file_name":"I0002.jpg"}
{"ts":1751731300000,"ev":"ext","ip":"192.168.1.40","marker":"incision"}
{"ts":1751731400000,"ev":"rec_stop","file_name":"V0001.mp4"}
{"ts":1751731405000,"ev":"study_finish"}
```

External events can be added over the API with `PUT /api/study/event` (see above),
or from a web app running on the device via the
`window.MvrOverlay.injectTimelineEvent()` bridge method.


---

## `PUT /api/study/event`

Inject a custom event into the current study's **timeline** (see *Study timeline*
below). The request body must be a JSON object; its fields are recorded alongside
the event.

The event is stored as an `"ext"` record: MVR adds `"ev":"ext"`, `"ts"` (epoch
milliseconds) and `"ip"` (the requesting client's IP address). The reserved keys
`ts`, `ev` and `ip` cannot be overridden by the request body.

The study folder is created lazily. Injecting an event **triggers folder
creation** (like a capture/recording does): the first event on its own is enough
to materialize the study folder and its `timeline.ndjson` on disk.

Example request body:
```json
{ "marker": "incision", "note": "step 1" }
```
resulting timeline line:
```json
{"ts":1751731200000,"ev":"ext","ip":"192.168.1.40","marker":"incision","note":"step 1"}
```

**Errors:** `400` if there is no active study (or it is already finished), or if
the body is not a valid JSON object.


---
