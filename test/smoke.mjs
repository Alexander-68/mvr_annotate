// Dev-only smoke test (not shipped). Serves the project over HTTP, loads the
// page in headless Chromium, and checks the config-driven UI actually builds and
// behaves: both clusters render with the right buttons, and long-pressing a
// submenu host (Injection) pops its submenu. It also opens a nested submenu and
// verifies the bridge event uses modifier2. Saves a screenshot to test/smoke.png.
//
//   node test/smoke.mjs
//
// Must be served over HTTP (not file://) because index.html fetch()es
// mvr_annotate.json — a file:// origin trips the CORS fail-loud banner.
import { createServer } from 'node:http';
import { readFile } from 'node:fs/promises';
import { fileURLToPath } from 'node:url';
import { dirname, join, normalize } from 'node:path';
import { chromium } from 'playwright';

const root = join(dirname(fileURLToPath(import.meta.url)), '..');
const TYPES = { '.html': 'text/html', '.json': 'application/json', '.png': 'image/png' };

// Scripted /ai/events feed: 20 packets per phase at a score that lands the
// class-0 moving average in the bad band, then the good one, then warn — i.e.
// across both traffic-light boundaries in both directions. The phases cycle for
// as long as the page is open, so the test can poll for each icon in turn
// whenever it gets there, with no timing assumptions.
const AI_PHASES = [0.9, 0.1, 0.55];
let aiFeed = false;
function serveAiEvents(res) {
  res.writeHead(200, {
    'content-type': 'text/event-stream',
    'cache-control': 'no-cache',
    connection: 'keep-alive',
  });
  let frm = 0;
  const t = setInterval(() => {
    const scr = AI_PHASES[Math.floor(frm / 20) % AI_PHASES.length];
    frm++;
    res.write(`data: ${JSON.stringify({
      ts_us: frm * 1000, cam: 100, frm, src: [1280, 720], aoi: [0, 0, 0, 0],
      mdl: 'smoke', det: [{ cls: 0, scr }, { cls: 1, scr: 0 }],
    })}\n\n`);
  }, 20);
  res.on('close', () => clearInterval(t));
}

// Minimal static file server rooted at the project dir.
const server = createServer(async (req, res) => {
  try {
    // normalize() yields backslashes on Windows; fold them back to '/' so the
    // route comparisons below work on every platform.
    const rel = normalize(decodeURIComponent(req.url.split('?')[0]))
      .replace(/^(\.\.[/\\])+/, '').replace(/\\/g, '/');
    // Off until the menu assertions are done: once packets flow the aiScope
    // indicators become visible and sit on top of the button clusters.
    if (rel === '/ai/events') {
      if (aiFeed) serveAiEvents(res);
      else { res.writeHead(204); res.end(); }
      return;
    }
    if (rel === '/study/events') {
      res.writeHead(204); res.end(); return;
    }
    // Pin the menu content to the colonoscopy profile so the test exercises the
    // CODE (nested submenus, ids, minimize) and not whichever profile currently
    // ships as mvr_annotate.json.
    const file = rel === '/' ? 'index.html'
      : rel === '/mvr_annotate.json' ? 'mvr_annotate_colonoscopy.json'
      : rel;
    const path = join(root, file);
    const body = await readFile(path);
    const ext = path.slice(path.lastIndexOf('.'));
    res.writeHead(200, { 'content-type': TYPES[ext] || 'application/octet-stream' });
    res.end(body);
  } catch {
    res.writeHead(404); res.end('not found');
  }
});

const EXPECT = {
  segments: ['Illeum', 'R.Colon', 'Tv.Colon', 'L.Colon', 'S.Colon', 'Rectum'],
  actions: ['Status', 'Withdrawal', 'Injection', 'Hemostasis', 'Biopsy', 'Polyp'],
  data: ['Trial Timeframe', 'Current Disease', 'Camera model', 'Open Forceps Size (mm)'],
  injectionSubmenu: ['Lift', 'Hemostasis', 'Botox', 'Steroid', 'Tattoo', 'Contrast'],
  hemostasisSubmenu: ['Hemoclip', 'Thermal', 'APC', 'Injection', 'Band', 'Topical', 'Surgical'],
  ids: ['Data', 'Segments', 'Actions'],
};

const fail = (msg) => { console.error('✗ ' + msg); process.exitCode = 1; };
const ok = (msg) => console.log('✓ ' + msg);
const eq = (a, b) => JSON.stringify(a) === JSON.stringify(b);
const buttonClusterLabels = () =>
  page.$$eval('.cluster', (els) => els
    .filter((el) => el.querySelector('.cluster-button'))
    .map((el) => [...el.querySelectorAll('.cluster-button .label')].map((s) => s.textContent)));

await new Promise((r) => server.listen(0, r));
const port = server.address().port;
const base = `http://localhost:${port}/`;

const browser = await chromium.launch();
const page = await browser.newPage({ viewport: { width: 1024, height: 768 } });
await page.addInitScript(() => {
  window.__events = [];
  window.MvrOverlay = {
    reportInteractive() {},
    isRecordingActive() { return false; },
    injectTimelineEvent(json) {
      window.__events.push(JSON.parse(json));
    },
  };
});

// Any console error or uncaught exception fails the test (e.g. the fail-loud banner).
const errors = [];
page.on('console', (m) => m.type() === 'error' && errors.push(m.text()));
page.on('pageerror', (e) => errors.push(String(e)));

async function longPress(locator) {
  const box = await locator.boundingBox();
  await page.mouse.move(box.x + box.width / 2, box.y + box.height / 2);
  await page.mouse.down();
  await page.waitForTimeout(750);
  await page.mouse.up();
}

try {
  // 'load' (not 'networkidle'): the /ai/events SSE stream never goes idle.
  await page.goto(base, { waitUntil: 'load' });
  await page.waitForSelector('.cluster-button', { state: 'attached' });

  // Read the built clusters' button labels straight from the DOM.
  const clusters = await buttonClusterLabels();

if (clusters.length !== 3) fail(`expected 3 clusters at load, got ${clusters.length}`);
  else ok('three clusters built from config');

  if (eq(clusters[0], EXPECT.data)) ok('cluster 1 = data buttons');
  else fail(`cluster 1 buttons mismatch: ${JSON.stringify(clusters[0])}`);

  if (eq(clusters[1], EXPECT.segments)) ok('cluster 2 = segments buttons');
  else fail(`cluster 2 buttons mismatch: ${JSON.stringify(clusters[1])}`);

  if (eq(clusters[2], EXPECT.actions)) ok('cluster 3 = actions buttons');
  else fail(`cluster 3 buttons mismatch: ${JSON.stringify(clusters[2])}`);

  const topIds = await page.$$eval('.cluster .cluster-id-label', (els) =>
    els.slice(0, 3).map((el) => el.textContent));
  if (eq(topIds, EXPECT.ids)) ok('top-level cluster ids rendered');
  else fail(`cluster id labels mismatch: ${JSON.stringify(topIds)}`);

  const minimizedIds = await page.$$eval('.cluster.minimized .cluster-min-toggle .label', (els) =>
    els.map((el) => el.textContent));
  if (eq(minimizedIds, EXPECT.ids)) ok('top-level clusters start minimized');
  else fail(`default minimized clusters mismatch: ${JSON.stringify(minimizedIds)}`);

  await page.locator('.cluster-min-toggle', { hasText: 'Actions' }).click();
  await page.waitForFunction(() => [...document.querySelectorAll('.cluster')]
    .some((el) => el.querySelector('.cluster-id-label')?.textContent === 'Actions' &&
      !el.classList.contains('minimized')));
  ok('tap on minimized cluster id restores actions cluster');

  const eventCountAfterRestore = await page.evaluate(() => window.__events.length);
  if (eventCountAfterRestore === 0) ok('restore cluster tap did not inject an event');
  else fail(`restore cluster tap injected events: ${eventCountAfterRestore}`);

  const actionsCluster = page.locator('.cluster', {
    has: page.locator('.cluster-id-label', { hasText: 'Actions' }),
  });
  await actionsCluster.locator('.cluster-id-label').click();
  await page.waitForFunction(() => [...document.querySelectorAll('.cluster')]
    .some((el) => el.querySelector('.cluster-id-label')?.textContent === 'Actions' &&
      el.classList.contains('minimized')));
  await page.locator('.cluster-min-toggle', { hasText: 'Actions' }).click();
  await page.waitForFunction(() => [...document.querySelectorAll('.cluster')]
    .some((el) => el.querySelector('.cluster-id-label')?.textContent === 'Actions' &&
      !el.classList.contains('minimized')));
  const eventCountAfterToggleRoundTrip = await page.evaluate(() => window.__events.length);
  if (eventCountAfterToggleRoundTrip === 0) ok('minimize/restore cluster taps did not inject events');
  else fail(`minimize/restore cluster taps injected events: ${eventCountAfterToggleRoundTrip}`);

  // Long-press the Injection host: hover, press, hold past LONG_PRESS_MS (500ms)
  // without moving so fireLongPress opens the submenu mid-hold, then release.
  const injection = actionsCluster.locator('.cluster-button', { hasText: 'Injection' });
  await longPress(injection);

  await page.waitForSelector('.recording-warning.visible', { timeout: 2000 })
    .then(() => ok('inactive recording warning appears on event injection'))
    .catch(() => fail('inactive recording warning did not appear'));
  const warningText = await page.locator('.recording-warning-message').textContent();
  if (warningText === 'Recording is not active. Start recording on the MVR before adding timeline annotations.') {
    ok('recording warning text comes from config');
  } else {
    fail(`recording warning text mismatch: ${JSON.stringify(warningText)}`);
  }
  const warning = page.locator('.recording-warning');
  const warningBg = await warning.evaluate((el) => getComputedStyle(el).backgroundColor);
  if (warningBg === 'rgb(245, 158, 11)') ok('recording warning background is orange');
  else fail(`recording warning background mismatch: ${warningBg}`);
  const warningBox = await warning.boundingBox();
  const viewport = page.viewportSize();
  const centred = Math.abs((warningBox.x + warningBox.width / 2) - viewport.width / 2) < 2;
  if (centred && warningBox.y + warningBox.height < viewport.height) {
    ok('recording warning sits centred near the bottom');
  } else {
    fail(`recording warning misplaced: ${JSON.stringify(warningBox)}`);
  }
  await page.locator('.recording-warning-dismiss').click();
  await page.waitForFunction(() => !document.querySelector('.recording-warning')?.classList.contains('visible'));
  ok('recording warning dismisses for the session');

  await page.waitForFunction(() =>
    [...document.querySelectorAll('.cluster')].filter((el) => el.querySelector('button')).length === 4,
    { timeout: 2000 })
    .then(() => ok('long-press Injection opened a submenu'))
    .catch(() => fail('submenu did not open on long-press'));

  const all = await buttonClusterLabels();
  const submenu = all.find((c) => eq(c, EXPECT.injectionSubmenu) ||
    JSON.stringify(c) === JSON.stringify(EXPECT.injectionSubmenu));
  if (submenu) ok('submenu = Injection modifiers');
  else fail(`Injection submenu buttons not found; clusters: ${JSON.stringify(all)}`);

  const submenuCluster = page.locator('.cluster', {
    has: page.locator('.cluster-button', { hasText: 'Lift' }),
  });
  const submenuTitle = await submenuCluster.locator('.cluster-id-label').textContent();
  const submenuMinimizeCount = await submenuCluster.locator('.cluster-min-toggle').count();
  if (submenuTitle === 'Injection' && submenuMinimizeCount === 0) {
    ok('submenu title uses host label and has no minimize control');
  } else {
    fail(`submenu title/minimize mismatch: title=${JSON.stringify(submenuTitle)}, minimize=${submenuMinimizeCount}`);
  }

  const injectionCluster = submenuCluster;
  await longPress(injectionCluster.locator('.cluster-button', { hasText: 'Hemostasis' }));

  await page.waitForFunction(() =>
    [...document.querySelectorAll('.cluster')].filter((el) => el.querySelector('button')).length === 5,
    { timeout: 2000 })
    .then(() => ok('long-press nested Hemostasis opened a sub-submenu'))
    .catch(() => fail('nested submenu did not open on long-press'));

  const nestedAll = await buttonClusterLabels();
  const nested = nestedAll.find((c) => JSON.stringify(c) === JSON.stringify(EXPECT.hemostasisSubmenu));
  if (nested) ok('sub-submenu = Hemostasis modifiers');
  else fail(`Hemostasis sub-submenu buttons not found; clusters: ${JSON.stringify(nestedAll)}`);

  const hemostasisCluster = page.locator('.cluster', {
    has: page.locator('button', { hasText: 'Hemoclip' }),
  });
  const nestedTitle = await hemostasisCluster.locator('.cluster-id-label').textContent();
  const nestedMinimizeCount = await hemostasisCluster.locator('.cluster-min-toggle').count();
  if (nestedTitle === 'Hemostasis' && nestedMinimizeCount === 0) {
    ok('sub-submenu title uses parent label and has no minimize control');
  } else {
    fail(`sub-submenu title/minimize mismatch: title=${JSON.stringify(nestedTitle)}, minimize=${nestedMinimizeCount}`);
  }
  await hemostasisCluster.locator('.cluster-button', { hasText: 'Hemoclip' }).click();
  const lastEvent = await page.evaluate(() => window.__events.at(-1));
  if (eq(lastEvent, { marker: 'Injection', modifier: 'Hemostasis', modifier2: 'Hemoclip' })) {
    ok('nested event uses modifier2');
  } else {
    fail(`nested event mismatch: ${JSON.stringify(lastEvent)}`);
  }
  const warningVisibleAfterDismiss = await page.locator('.recording-warning.visible').count();
  if (warningVisibleAfterDismiss === 0) ok('dismissed recording warning does not reappear in session');
  else fail('dismissed recording warning reappeared');

  // aiScope traffic light: reload with the scripted feed on (EventSource does
  // not retry the 204 it got on first load) and watch the class-0 moving average
  // walk bad -> good -> warn; the icon must follow in that order.
  aiFeed = true;
  await page.reload({ waitUntil: 'load' });
  const icon0 = '.cluster .indicator:first-of-type .ind-icon';
  for (const want of ['bad', 'ok', 'warn']) {
    await page.waitForFunction(
      ([sel, name]) => (document.querySelector(sel)?.getAttribute('src') || '').includes(`-${name}-`),
      [icon0, want], { timeout: 5000 },
    ).then(() => ok(`aiScope icon switched to "${want}"`))
      .catch(() => fail(`aiScope icon never became "${want}"`));
  }

  // Each band change is also a timeline event: {marker, event: class label, note: band}.
  const bandEvents = await page.evaluate(() => window.__events
    .filter((e) => e.marker === 'aiScope' && e.event === 'Blur')
    .map((e) => e.note));
  if (['bad', 'good', 'warn'].every((b) => bandEvents.includes(b))) {
    ok('aiScope band changes injected timeline events');
  } else {
    fail(`aiScope band events missing: ${JSON.stringify(bandEvents)}`);
  }
  const summaries = await page.evaluate(() => window.__events.filter((e) => e.event === 'video_summary'));
  if (summaries.length === 0) ok('per-video summary event stays disabled');
  else fail(`unexpected video_summary events: ${JSON.stringify(summaries)}`);

  const shot = join(root, 'test', 'smoke.png');
  await page.screenshot({ path: shot });
  ok(`screenshot saved: ${shot}`);

  if (errors.length) fail(`console/page errors: ${JSON.stringify(errors)}`);
  else ok('no console or page errors');
} finally {
  await browser.close();
  server.close();
}

console.log(process.exitCode ? '\nSMOKE TEST FAILED' : '\nSMOKE TEST PASSED');
