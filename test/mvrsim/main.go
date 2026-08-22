// mvrsim — a fake MVR device for developing the annotate overlay off-device.
//
// Serves the repo (index.html + mvr_annotate.json + assets) and, on top of it,
// the two SSE feeds the real device exposes (MVR_homeweb_sse.md):
//
//	GET  /ai/events     inference stream (latest-only, generated here)
//	GET  /study/events  study/recording timeline (replayed on connect)
//	GET  /ai/latest, /study/latest   poll fallbacks
//
// index.html is served with a small window.MvrOverlay shim injected, so the page
// believes it runs inside the device: injectTimelineEvent() posts back here and
// is logged + mirrored into /study/events as an "ext" event.
//
// Drive the device from the control page at /mvr/ (or curl the endpoints).
//
//	go run ./test/mvrsim            # serves the repo root at :3335
package main

import (
	"bytes"
	"encoding/json"
	"flag"
	"fmt"
	"html"
	"io"
	"log"
	"math"
	"math/rand"
	"net/http"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"sync"
	"time"
)

type event struct {
	id   int
	data []byte
}

// hub is one SSE feed: fan-out to subscribers plus a replay buffer.
type hub struct {
	mu      sync.Mutex
	nextID  int
	buf     []event // last `keep` events, oldest first
	keep    int
	subs    map[chan event]bool
	latest  []byte
	retryMs int
}

func newHub(keep int) *hub {
	return &hub{keep: keep, subs: map[chan event]bool{}, retryMs: 3000}
}

func (h *hub) publish(v any) {
	data, err := json.Marshal(v)
	if err != nil {
		log.Println("marshal:", err)
		return
	}
	h.mu.Lock()
	h.nextID++
	ev := event{id: h.nextID, data: data}
	h.latest = data
	if h.keep > 0 {
		h.buf = append(h.buf, ev)
		if len(h.buf) > h.keep {
			h.buf = h.buf[len(h.buf)-h.keep:]
		}
	}
	for ch := range h.subs {
		select {
		case ch <- ev:
		default: // slow client: drop, like the device's bounded queue
		}
	}
	h.mu.Unlock()
}

// subscribe registers a client and returns the backlog it should get first:
// everything after lastID from the replay buffer, or — for a latest-only feed —
// just the most recent event.
func (h *hub) subscribe(lastID int) (chan event, []event) {
	ch := make(chan event, 64)
	h.mu.Lock()
	defer h.mu.Unlock()
	h.subs[ch] = true
	var back []event
	if h.keep > 0 {
		for _, e := range h.buf {
			if e.id > lastID {
				back = append(back, e)
			}
		}
	} else if h.latest != nil {
		back = []event{{id: h.nextID, data: h.latest}}
	}
	return ch, back
}

func (h *hub) unsubscribe(ch chan event) {
	h.mu.Lock()
	delete(h.subs, ch)
	h.mu.Unlock()
}

func (h *hub) serveSSE(w http.ResponseWriter, r *http.Request) {
	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "no streaming", http.StatusInternalServerError)
		return
	}
	lastID, _ := strconv.Atoi(r.Header.Get("Last-Event-ID"))
	ch, back := h.subscribe(lastID)
	defer h.unsubscribe(ch)

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	fmt.Fprintf(w, "retry: %d\n\n", h.retryMs)
	for _, e := range back {
		fmt.Fprintf(w, "id: %d\ndata: %s\n\n", e.id, e.data)
	}
	flusher.Flush()

	ping := time.NewTicker(15 * time.Second)
	defer ping.Stop()
	for {
		select {
		case <-r.Context().Done():
			return
		case e := <-ch:
			fmt.Fprintf(w, "id: %d\ndata: %s\n\n", e.id, e.data)
			flusher.Flush()
		case <-ping.C:
			fmt.Fprint(w, ": ping\n\n")
			flusher.Flush()
		}
	}
}

func (h *hub) serveLatest(w http.ResponseWriter, r *http.Request) {
	h.mu.Lock()
	data := h.latest
	h.mu.Unlock()
	if data == nil {
		data = []byte("{}")
	}
	w.Header().Set("Content-Type", "application/json")
	w.Write(data)
}

// --- device state ----------------------------------------------------------

type device struct {
	mu       sync.Mutex
	recState string // STOPPED | RUNNING | PAUSED
	study    string // "" when no active study
	recFile  string
	recSeq   int
}

var (
	dev      = &device{recState: "STOPPED", study: "CASE0001"}
	studyHub = newHub(300) // replayed on connect, like the device
	aiHub    = newHub(0)   // latest-only
)

// studyEvent publishes one timeline line: {ts, ev, ...extra}.
// timeline, if -timeline was given: every study event is appended to it as one
// JSON object per line, the same NDJSON the device writes as timeline.ndjson.
// Opened once in main; writes are serialized by timelineMu because studyEvent
// runs from both request handlers and the aiLoop goroutine.
var (
	timelineFile *os.File
	timelineMu   sync.Mutex
)

// timelineKeyOrder puts the keys that matter first on a timeline line; anything
// else follows in alphabetical order. json.Marshal of a map always sorts keys,
// which buries `marker` behind `ev`/`ip` and makes a scanned line hard to read.
var timelineKeyOrder = []string{"ts", "ev", "marker", "level", "status",
	"modifier", "modifier2", "modifier3"}

func marshalOrdered(m map[string]any) ([]byte, error) {
	rest := make([]string, 0, len(m))
	for k := range m {
		if !slices.Contains(timelineKeyOrder, k) {
			rest = append(rest, k)
		}
	}
	slices.Sort(rest)
	var b bytes.Buffer
	b.WriteByte('{')
	for _, k := range append(slices.Clone(timelineKeyOrder), rest...) {
		v, ok := m[k]
		if !ok {
			continue
		}
		val, err := json.Marshal(v)
		if err != nil {
			return nil, err
		}
		if b.Len() > 1 {
			b.WriteByte(',')
		}
		key, _ := json.Marshal(k)
		b.Write(key)
		b.WriteByte(':')
		b.Write(val)
	}
	b.WriteByte('}')
	return b.Bytes(), nil
}

func studyEvent(name string, extra map[string]any) {
	m := map[string]any{"ts": time.Now().UnixMilli(), "ev": name}
	for k, v := range extra {
		m[k] = v
	}
	studyHub.publish(m)
	log.Printf("study: %s %v", name, extra)
	if timelineFile == nil {
		return
	}
	line, err := marshalOrdered(m)
	if err != nil {
		return
	}
	timelineMu.Lock()
	defer timelineMu.Unlock()
	timelineFile.Write(append(line, '\n')) // O_APPEND + unbuffered: tail -f works
}

// setRec applies one recording/snapshot action and emits its timeline event.
func (d *device) setRec(action string) string {
	d.mu.Lock()
	defer d.mu.Unlock()
	switch action {
	case "start":
		if d.recState != "STOPPED" {
			return "already recording"
		}
		d.recSeq++
		d.recFile = fmt.Sprintf("VIDEO%04d.mp4", d.recSeq)
		d.recState = "RUNNING"
		studyEvent("rec_start", map[string]any{"file_name": d.recFile})
	case "pause":
		if d.recState != "RUNNING" {
			return "not running"
		}
		d.recState = "PAUSED"
		studyEvent("rec_pause", map[string]any{"file_name": d.recFile})
	case "resume":
		if d.recState != "PAUSED" {
			return "not paused"
		}
		d.recState = "RUNNING"
		studyEvent("rec_resume", map[string]any{"file_name": d.recFile})
	case "stop":
		if d.recState == "STOPPED" {
			return "not recording"
		}
		d.recState = "STOPPED"
		studyEvent("rec_stop", map[string]any{"file_name": d.recFile})
	case "error":
		d.recState = "STOPPED"
		studyEvent("rec_error", map[string]any{"file_name": d.recFile})
	case "snapshot":
		studyEvent("snapshot", map[string]any{"file_name": fmt.Sprintf("PHOTO%04d.jpg", time.Now().Unix()%10000)})
	default:
		return "unknown action"
	}
	return "ok"
}

// --- fake inference stream -------------------------------------------------

// aiLoop emits one packet per frame with the single artifact class (cls 0) the
// tiled3x3 model outputs: the estimated proportion of the frame covered by
// artefacts, drifting on a slow sine so the indicator crosses every icon band.
func aiLoop(fps int, model string) {
	t := time.NewTicker(time.Second / time.Duration(fps))
	defer t.Stop()
	frm := 0
	for range t.C {
		frm++
		phase := float64(frm) / float64(fps) / 12.0 * 2 * math.Pi
		v := 0.5 + 0.45*math.Sin(phase) + 0.05*(rand.Float64()-0.5)
		v = math.Max(0, math.Min(1, v))
		aiHub.publish(map[string]any{
			"ts_us": time.Now().UnixMicro(),
			"cam":   100,
			"frm":   frm,
			"src":   []int{1280, 720},
			"aoi":   []int{0, 0, 0, 0},
			"mdl":   model,
			"det": []map[string]any{
				{"cls": 0, "scr": round6(v)},
			},
		})
	}
}

func round6(f float64) float64 { return math.Round(f*1e6) / 1e6 }

// --- MvrOverlay bridge shim ------------------------------------------------

// Injected into every served .html so the page detects a "device". Recording
// state is seeded synchronously from /mvr/state (the page may call
// isRecordingActive() before any SSE arrives) and then kept fresh off
// /study/events.
const shim = `<script>
(function () {
  var state = { rec: "STOPPED", study: "CASE0001" };
  try {
    var x = new XMLHttpRequest();
    x.open("GET", "/mvr/state", false); x.send();
    state = JSON.parse(x.responseText);
  } catch (e) { console.warn("[mvrsim] state seed failed", e); }
  try {
    var es = new EventSource("/study/events");
    es.onmessage = function (e) {
      var ev = JSON.parse(e.data);
      if (ev.ev === "rec_start" || ev.ev === "rec_resume") state.rec = "RUNNING";
      else if (ev.ev === "rec_pause") state.rec = "PAUSED";
      else if (ev.ev === "rec_stop" || ev.ev === "rec_error") state.rec = "STOPPED";
      else if (ev.ev === "study_start") state.study = ev.folder_name;
      else if (ev.ev === "study_finish") state.study = null;
    };
  } catch (e) {}
  var FEATURES = { mvr_aiscope: true, mvr_pro_4k: true, mvr_pro_pacs: true,
                   mvr_435_activation: true, mvx_441_activation: true };
  window.MvrOverlay = {
    getDeviceInfo: function () { return JSON.stringify({
      deviceId: "S1MU1AT0", model: "MVR-SIM", appVersion: 260704, firmware: "0.0.0-sim" }); },
    getDeviceId: function () { return "S1MU1AT0"; },
    getModelName: function () { return "MVR-SIM"; },
    getAppVersion: function () { return 260704; },
    getFirmwareVersion: function () { return "0.0.0-sim"; },
    hasFeature: function (id) { return FEATURES[id] === true; },
    getCurrentStudyPath: function () { return state.study || null; },
    isRecordingActive: function () { return state.rec !== "STOPPED"; },
    getRecordingState: function () { return state.rec; },
    reportInteractive: function () {},
    injectTimelineEvent: function (json) {
      try { JSON.parse(json); } catch (e) { return false; }
      if (!state.study) return false;
      fetch("/mvr/inject", { method: "POST", body: json });
      return true;
    }
  };
  console.log("[mvrsim] MvrOverlay bridge installed");
})();
</script>
`

// --- control panel ---------------------------------------------------------

// The device controls, as a self-contained floating panel. It is injected into
// every served page (so the overlay can be driven from its own window) and is
// also the whole body of the standalone /mvr/ page. Everything is scoped under
// #mvrsim-panel so it cannot touch the overlay's own styles; z-index sits above
// the app's highest layer (9999). Collapsed state is remembered per browser.
const panel = `<div id="mvrsim-panel" data-open="1">
<style>
#mvrsim-panel { position: fixed; left: 8px; bottom: 8px; z-index: 100000;
  font: 12px/1.3 system-ui, sans-serif; color: #eee; background: rgba(20,20,24,0.92);
  border: 1px solid #555; border-radius: 8px; padding: 6px; max-width: 260px;
  touch-action: manipulation; }
#mvrsim-panel button { font: inherit; color: #eee; background: #33343c; border: 1px solid #666;
  border-radius: 5px; padding: 3px 7px; margin: 1px; cursor: pointer; }
#mvrsim-panel button:hover { background: #45464f; }
#mvrsim-panel .mvrsim-head { display: flex; justify-content: space-between; align-items: center;
  gap: 8px; cursor: pointer; }
#mvrsim-panel .mvrsim-title { font-weight: 600; letter-spacing: .04em; }
#mvrsim-panel .mvrsim-state { margin: 4px 0; color: #9fd; font-family: ui-monospace, monospace; }
#mvrsim-panel[data-open="0"] .mvrsim-body { display: none; }
</style>
<div class="mvrsim-head"><span class="mvrsim-title">mvrsim</span><span class="mvrsim-toggle">&#9660;</span></div>
<div class="mvrsim-body">
  <div class="mvrsim-state">&hellip;</div>
  <div><button data-a="rec/start">rec</button><button data-a="rec/pause">pause</button>
  <button data-a="rec/resume">resume</button><button data-a="rec/stop">stop</button>
  <button data-a="rec/error">error</button></div>
  <div><button data-a="snapshot">snapshot</button>
  <button data-a="study/start">study start</button>
  <button data-a="study/finish">study finish</button></div>
</div>
</div>
<script>
(function () {
  var root = document.getElementById('mvrsim-panel');
  var out = root.querySelector('.mvrsim-state');
  root.setAttribute('data-open', localStorage.getItem('mvrsim-panel-open') === '0' ? '0' : '1');
  root.querySelector('.mvrsim-head').addEventListener('click', function () {
    var open = root.getAttribute('data-open') === '1' ? '0' : '1';
    root.setAttribute('data-open', open);
    localStorage.setItem('mvrsim-panel-open', open);
    root.querySelector('.mvrsim-toggle').innerHTML = open === '1' ? '&#9660;' : '&#9650;';
  });
  async function refresh() {
    try {
      var s = await (await fetch('/mvr/state')).json();
      out.textContent = s.rec + '  ' + (s.study || 'no study') + (s.file ? '  ' + s.file : '');
    } catch (e) { out.textContent = 'offline'; }
  }
  root.addEventListener('click', async function (e) {
    var a = e.target.getAttribute && e.target.getAttribute('data-a');
    if (!a) return;
    // Claim the tap so it never reaches the overlay's clusters underneath.
    e.stopPropagation();
    var r = await fetch('/mvr/' + a, { method: 'POST' });
    var msg = await r.text();
    if (msg !== 'ok') console.log('[mvrsim]', a, msg);
    refresh();
  });
  // pointerdown is what the overlay listens on — keep panel taps out of it.
  root.addEventListener('pointerdown', function (e) { e.stopPropagation(); });
  refresh(); setInterval(refresh, 1000);
})();
</script>
`

const controlPage = `<!doctype html><meta charset=utf-8><title>mvrsim control</title>
<style>body{font:15px system-ui;margin:2rem;background:#111;color:#eee}
a{color:#9cf}</style>
<h1>mvrsim</h1><p><a href="/">open the overlay app</a> (the same panel is injected there)</p>
` + panel

// --- static files with shim injection --------------------------------------

type shimFS struct {
	root  string
	fs    http.Handler
	bg    string // page backdrop; the overlay itself is transparent
	panel bool   // inject the floating device-control panel
}

func (s shimFS) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	p := r.URL.Path
	if p == "/" {
		p = "/index.html"
	}
	if strings.HasSuffix(p, ".html") && !strings.Contains(p, "..") {
		b, err := os.ReadFile(filepath.Join(s.root, filepath.FromSlash(strings.TrimPrefix(p, "/"))))
		if err == nil {
			b = bytes.Replace(b, []byte("<head>"), []byte("<head>\n"+shim), 1)
			if s.bg != "" {
				// Stand in for the device's live camera preview: the overlay is a
				// transparent layer, so without a backdrop it sits on browser white.
				// Goes last in <head> so it wins over the page's own transparent rule.
				css := "<style>html,body{background:" + html.EscapeString(s.bg) + "}</style>\n</head>"
				b = bytes.Replace(b, []byte("</head>"), []byte(css), 1)
			}
			if s.panel {
				b = bytes.Replace(b, []byte("</body>"), []byte(panel+"</body>"), 1)
			}
			w.Header().Set("Content-Type", "text/html; charset=utf-8")
			w.Header().Set("Cache-Control", "no-store")
			w.Write(b)
			return
		}
	}
	s.fs.ServeHTTP(w, r)
}

func main() {
	addr := flag.String("addr", ":3335", "listen address")
	root := flag.String("root", "../..", "folder to serve (repo root)")
	fps := flag.Int("fps", 10, "fake inference packets per second (0 = off)")
	bg := flag.String("bg", "#000", "backdrop behind the transparent overlay (CSS color, \"\" = none)")
	showPanel := flag.Bool("panel", true, "inject the floating device-control panel into served pages")
	model := flag.String("model", "pd_mobinenetv3l_blur_poor_prep_trained_opset18_fp", "reported model name")
	timeline := flag.String("timeline", "timeline.ndjson", "append every study event to this NDJSON file (\"\" = off)")
	flag.Parse()

	abs, err := filepath.Abs(*root)
	if err != nil {
		log.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(abs, "index.html")); err != nil {
		log.Fatalf("no index.html under %s (pass -root)", abs)
	}

	if *timeline != "" {
		f, err := os.OpenFile(*timeline, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
		if err != nil {
			log.Fatalf("cannot open timeline file: %v", err)
		}
		defer f.Close()
		timelineFile = f
		abs, _ := filepath.Abs(*timeline)
		log.Printf("timeline: appending to %s", abs)
	}

	mux := http.NewServeMux()
	mux.HandleFunc("/ai/events", aiHub.serveSSE)
	mux.HandleFunc("/ai/latest", aiHub.serveLatest)
	mux.HandleFunc("/study/events", studyHub.serveSSE)
	mux.HandleFunc("/study/latest", studyHub.serveLatest)
	mux.HandleFunc("/mvr/", serveControl)
	mux.Handle("/", shimFS{root: abs, fs: http.FileServer(http.Dir(abs)), bg: *bg, panel: *showPanel})

	// Open a study so injectTimelineEvent() is accepted from the first tap.
	studyEvent("study_start", map[string]any{"folder_name": dev.study, "device_id": "S1MU1AT0"})
	if *fps > 0 {
		go aiLoop(*fps, *model)
	}

	log.Printf("mvrsim serving %s on http://localhost%s  (control page: /mvr/)", abs, *addr)
	log.Fatal(http.ListenAndServe(*addr, mux))
}

func serveControl(w http.ResponseWriter, r *http.Request) {
	switch action := strings.TrimPrefix(r.URL.Path, "/mvr/"); action {
	case "", "index.html":
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		io.WriteString(w, controlPage)
	case "state":
		dev.mu.Lock()
		st := map[string]any{"rec": dev.recState, "study": dev.study, "file": dev.recFile}
		dev.mu.Unlock()
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(st)
	case "inject":
		body, _ := io.ReadAll(io.LimitReader(r.Body, 1<<20))
		var m map[string]any
		if json.Unmarshal(body, &m) != nil {
			http.Error(w, "bad json", http.StatusBadRequest)
			return
		}
		delete(m, "ts") // reserved keys are set by the device, never the caller
		delete(m, "ev")
		m["ip"] = "localhost"
		studyEvent("ext", m)
		io.WriteString(w, "ok")
	case "study/start":
		dev.mu.Lock()
		dev.study = fmt.Sprintf("CASE%04d", time.Now().Unix()%10000)
		dev.recState, dev.recFile = "STOPPED", ""
		folder := dev.study
		dev.mu.Unlock()
		studyEvent("study_start", map[string]any{"folder_name": folder, "device_id": "S1MU1AT0"})
		io.WriteString(w, "ok")
	case "study/finish":
		dev.mu.Lock()
		dev.study, dev.recState = "", "STOPPED"
		dev.mu.Unlock()
		studyEvent("study_finish", nil)
		io.WriteString(w, "ok")
	case "snapshot":
		io.WriteString(w, dev.setRec("snapshot"))
	default:
		if strings.HasPrefix(action, "rec/") {
			io.WriteString(w, dev.setRec(strings.TrimPrefix(action, "rec/")))
			return
		}
		http.NotFound(w, r)
	}
}
