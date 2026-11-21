#!/usr/bin/env node
//
// Regenerate the README screenshots.
//
//   npm --prefix web run screenshots
//
// Starts the demo, docker-compose.dev.yml, as a project of its own on a free port, with the
// stand-in for App Store Connect making its account's days from a fixed moment. Once the seed has
// given two apps their webhooks, a mail relay and a recipient are added, and every moment the
// server keeps is set from that same one. Then headless Chromium is driven over the DevTools
// protocol, and the PNGs in docs/screenshots are overwritten. Everything it starts is stopped and
// removed on the way out.
//
//   SCALE=2      device pixels per CSS pixel
//   THEME=light  or dark
//   OUT=dir      where the PNGs land, so a look at dark need not overwrite the committed set
//   ONLY=a,b     just these shots
//   NOTES=0      without the notes
//   TIMEOUT=60   seconds a shot may take before the run gives up, cleaning up as it goes
//
// Needs node, docker and Chromium or Chrome.
import { execFileSync, spawnSync } from "node:child_process";
import {
  existsSync,
  mkdirSync,
  mkdtempSync,
  readdirSync,
  readFileSync,
  rmSync,
  writeFileSync,
} from "node:fs";
import { createServer } from "node:net";
import { tmpdir } from "node:os";
import { dirname, join, resolve } from "node:path";
import { setTimeout as sleep } from "node:timers/promises";
import { fileURLToPath } from "node:url";
import { createHash } from "node:crypto";
import { crc32, inflateSync } from "node:zlib";

import { Browser, findChromium, waitFor } from "../cdp.mjs";

const here = dirname(fileURLToPath(import.meta.url));
const root = resolve(here, "..", "..", "..");
const work = mkdtempSync(join(tmpdir(), "appstoreanalytics-shots-"));

const SCALE = Number(process.env.SCALE ?? 2);
const THEME = process.env.THEME ?? "light";
const OUT = process.env.OUT ?? join(root, "docs", "screenshots");
const ONLY = (process.env.ONLY ?? "").split(",").filter(Boolean);
const NOTES = process.env.NOTES !== "0";
const TIMEOUT = Number(process.env.TIMEOUT ?? 60);

// The moment the account's days are made from, the server's times are set from, and the page's
// clock starts at: the same on every run, so the same pictures.
const MOMENT = "2026-11-20T18:00:00Z";
const moment = Date.parse(MOMENT);
// The address appstoreanalytics says it is at, which its webhook addresses are shown under.
const SHOWN = "https://stats.example.com";
const TIDEPOOL = "1498172630";
// What each webhook address's random token is shown as.
const TOKENS = [
  "q7Rk2TfX9vLmB4nWc8YpE1sHjD6aZ0uG",
  "N3wJ5bQx8cKf1RtV7mYs2HgL9pAe4UdZ",
];

/**
 * What the account read from the stand-in depends on: the stand-in, the server, the demo's
 * Compose file and the moment. Its hash names the Compose project, whose volumes are kept, so a
 * run with none of them changed starts from the database the last one read, and one with any
 * changed reads afresh.
 */
function key() {
  const hash = createHash("sha256").update(MOMENT);
  const walk = (dir) => {
    for (const entry of readdirSync(dir, { withFileTypes: true }).toSorted(
      (a, b) => a.name.localeCompare(b.name),
    )) {
      const path = join(dir, entry.name);
      if (entry.isDirectory()) walk(path);
      else if (!entry.name.endsWith("_test.go"))
        hash.update(path.slice(root.length)).update(readFileSync(path));
    }
  };
  walk(join(root, "dev", "appstoreconnect"));
  walk(join(root, "internal"));
  hash.update(readFileSync(join(root, "docker-compose.dev.yml")));
  return hash.digest("hex").slice(0, 10);
}
const project = `appstoreanalytics-shots-${key()}`;
let up = false;
let browser;
function cleanup() {
  browser?.close();
  if (up)
    spawnSync("docker", [...composeArgs(), "down", "--remove-orphans"], {
      stdio: "ignore",
      timeout: 2 * 60_000,
    });
  // Chromium is still writing its profile as it exits.
  rmSync(work, {
    recursive: true,
    force: true,
    maxRetries: 5,
    retryDelay: 300,
  });
}
process.on("exit", cleanup);
process.on("SIGINT", () => process.exit(1));
process.on("SIGTERM", () => process.exit(1));

const step = (message) => console.log(`==> ${message}`);
function die(message, detail) {
  console.error(message);
  if (detail) console.error(detail);
  process.exit(1);
}

function run(command, args, options = {}) {
  try {
    return execFileSync(command, args, {
      stdio: "pipe",
      encoding: "utf8",
      maxBuffer: 64 << 20,
      ...options,
    });
  } catch (err) {
    die(
      `${command} ${args.join(" ")} failed`,
      [err.stdout, err.stderr].join("\n"),
    );
  }
}

/** A port nothing on this machine is listening on, a moment ago. */
function free() {
  return new Promise((done) => {
    const probe = createServer().listen(0, "127.0.0.1", () => {
      const { port } = probe.address();
      probe.close(() => done(port));
    });
  });
}

// --- the demo --------------------------------------------------------------------------------

const chromium = findChromium();
if (!chromium) die("no Chromium or Chrome found; set CHROMIUM");
run("docker", ["version", "--format", "{{.Server.Version}}"]);

const port = await free();
const BASE = `http://localhost:${port}`;
function composeArgs() {
  return ["compose", "-f", join(root, "docker-compose.dev.yml"), "-p", project];
}
// Each docker call has as long as it may take: a step past it is stuck, and the run ends.
const MINUTE = 60_000;
const compose = (limit, ...args) =>
  run("docker", [...composeArgs(), ...args], {
    timeout: limit,
    env: {
      ...process.env,
      DEV_PORT: String(port),
      DEV_URL: SHOWN,
      APPSTORECONNECT_NOW: MOMENT,
    },
  });

// What earlier runs kept of another account goes; with FRESH=1, this one's too.
for (const volume of run("docker", [
  "volume",
  "ls",
  "--quiet",
  "--filter",
  "name=appstoreanalytics-shots-",
])
  .split("\n")
  .filter(Boolean))
  if (process.env.FRESH === "1" || !volume.startsWith(`${project}_`))
    spawnSync("docker", ["volume", "rm", volume], { stdio: "ignore" });

step("building and starting the demo");
up = true;
compose(5 * MINUTE, "up", "--build", "--detach");
step("reading the account");
const seeded = spawnSync("docker", [...composeArgs(), "wait", "seed"], {
  encoding: "utf8",
  timeout: 3 * MINUTE,
});
if (seeded.status !== 0)
  die(
    "the demo could not be set up",
    spawnSync("docker", [...composeArgs(), "logs"], { encoding: "utf8" })
      .stdout,
  );

// --- the browser -----------------------------------------------------------------------------

browser = await Browser.launch({
  chromium,
  // Not a fixed one: another browser already on it would be driven instead, and its own work
  // would push this page to the back.
  port: await free(),
  profile: join(work, "chrome"),
  base: BASE,
});
await browser.scheme(THEME);
await browser.send("Emulation.setTimezoneOverride", { timezoneId: "UTC" });
await browser.send("Emulation.setLocaleOverride", { locale: "en-US" });
// The page's clock starts at the moment, so how long ago it says a time was is the same on
// every run. The server's own times after it, such as a session's last use, read "just now".
await browser.send("Page.addScriptToEvaluateOnNewDocument", {
  source: `(() => {
    const Real = Date;
    const shift = ${moment} - Real.now();
    globalThis.Date = class extends Real {
      constructor(...args) {
        if (args.length === 0) super(Real.now() + shift);
        else super(...args);
      }
      static now() {
        return Real.now() + shift;
      }
    };
  })()`,
});
// Without the Headless, which is what a list of browsers would otherwise name.
const agent = (await browser.eval("navigator.userAgent")).replace(
  "HeadlessChrome",
  "Chrome",
);
await browser.send("Emulation.setUserAgentOverride", { userAgent: agent });

/** What the API answers the signed-in browser. Any failure ends the run. */
async function api(method, path, body) {
  const said = await browser.eval(`fetch(${JSON.stringify(path)}, {
      method: ${JSON.stringify(method)},
      headers: { "Content-Type": "application/json" },
      body: ${body === undefined ? "undefined" : JSON.stringify(JSON.stringify(body))},
    }).then(async (r) => ({ status: r.status, text: await r.text() }))`);
  if (said.status >= 300)
    die(`${method} ${path} answered ${said.status}`, said.text);
  return said.text ? JSON.parse(said.text) : null;
}

step("signing in, and setting up mail");
await browser.open("/login");
const signedIn = await browser.eval(
  `fetch("/api/auth/login", { method: "POST", headers: { "Content-Type": "application/json" }, body: JSON.stringify({ username: "demo", password: "demodemo" }) }).then((r) => r.status)`,
);
if (signedIn !== 204) die(`signing in answered ${signedIn}`);
await api("PUT", "/api/settings/relay", {
  host: "smtp.example.com",
  port: 587,
  security: "starttls",
  username: "stats@example.com",
  password: "a relay password",
  from_address: "stats@example.com",
  from_name: "App Store analytics",
});
// A recipient a run kept is left alone.
const { endpoints, mails } = await api("GET", "/api/hooks");
const recipients = [
  {
    to: "releases@example.com",
    events: [
      "appStoreVersionAppVersionStateUpdated",
      "buildBetaDetailExternalBuildStateUpdated",
    ],
    endpoints: [],
  },
  {
    to: "qa@example.com",
    events: [
      "betaFeedbackScreenshotSubmissionCreated",
      "betaFeedbackCrashSubmissionCreated",
    ],
    endpoints: endpoints
      .filter((e) => e.label.startsWith("Knotwork"))
      .map((e) => e.id),
  },
];
for (const r of recipients)
  if (!mails.some((m) => m.to === r.to))
    await api("POST", "/api/hooks/mails", r);

// Every moment the server keeps, from the moment rather than from when this ran: a delivery at
// the time its event says it happened, the rest a few minutes before. Written beside the server,
// which is idle until its next hourly pass, through the same volume.
step("setting the server's times");
const at = Math.floor(moment / 1000);
const tokens = endpoints
  .map(
    (e, i) =>
      `UPDATE hook_endpoints SET token = '${TOKENS[i]}' WHERE id = ${e.id};`,
  )
  .join("\n");
const backdate = `
UPDATE sync_state SET ran_at = ${at - 240}, ok_at = CASE WHEN ok_at IS NULL THEN NULL ELSE ${at - 240} END;
UPDATE report_requests SET since = ${at - 300};
UPDATE analytics_instances SET read_at = ${at - 240};
UPDATE apps SET updated_at = ${at - 240};
UPDATE hooks SET received_at = CAST(strftime('%s', json_extract(CAST(body AS TEXT), '$.data.attributes.timestamp')) AS INTEGER) + 1;
UPDATE hook_endpoints SET created_at = ${at - 5 * 86400};
UPDATE hook_mails SET created_at = ${at - 3600};
UPDATE sessions SET created_at = ${at - 900};
${tokens}
`;
run(
  "docker",
  [
    "run",
    "--rm",
    "-v",
    `${project}_data:/data`,
    "node:26-alpine",
    "node",
    "-e",
    `const { DatabaseSync } = require("node:sqlite");
   const db = new DatabaseSync("/data/appstoreanalytics.db");
   db.exec("PRAGMA busy_timeout = 10000");
   db.exec(${JSON.stringify(backdate)});
   db.close();`,
  ],
  { timeout: MINUTE },
);

// --- the shots -----------------------------------------------------------------------------

/** What the shot under way is waiting for, which is what it says when it runs out of time. */
let awaiting = "";

/** Polls f until it holds, saying what for. The shot's own time limit is what ends it. */
function until(what, f) {
  awaiting = what;
  return waitFor(f, TIMEOUT * 4);
}

/** Everything drawn: fonts in, nothing still loading, and every chart drawn. */
async function settled() {
  await until("the fonts", () =>
    browser.eval("document.fonts.ready.then(() => true)"),
  );
  await until("what is loading", () =>
    browser.eval(`document.querySelector(".dummy") === null`),
  );
  // A chart draws its days once it comes into view: a bar or a line, beside its axis and the
  // transparent strip that takes the pointer.
  await until("the charts in view", () =>
    browser.eval(
      `[...document.querySelectorAll("[role=group]")].filter((g) => { const r = g.getBoundingClientRect(); return g.querySelector("svg") && r.bottom > 0 && r.top < innerHeight; }).every((g) => g.querySelector('svg path, svg rect:not([fill="transparent"])'))`,
    ),
  );
  await sleep(500);
}

/** Opens a page and waits for the day it ends on, which is the last thing a dashboard fills. */
async function dashboard(path) {
  await browser.open(path);
  await until("the days", () =>
    browser.eval(`document.body.textContent.includes("through 2026-11-18")`),
  );
}

const click = (selector, text) =>
  browser.eval(`(() => {
    const el = [...document.querySelectorAll(${JSON.stringify(selector)})].find((e) => !${JSON.stringify(text ?? "")} || e.textContent.trim() === ${JSON.stringify(text ?? "")});
    if (!el) throw new Error("nothing to click: " + ${JSON.stringify(selector + " " + (text ?? ""))});
    el.click();
    return true;
  })()`);

/**
 * Marks the sections headed so for a shot of them alone: the dashboard's parts and cards and the
 * settings' panels are each a section under a heading. The window is made tall enough to hold
 * them where they are, since scrolled they would go under the sticky bar of choices.
 */
const mark = (...titles) =>
  browser.eval(`(() => {
    const titles = ${JSON.stringify(titles)};
    const found = titles.map((t) => [...document.querySelectorAll("section")].find((s) => s.querySelector("h1, h2, h3")?.textContent.trim() === t));
    if (found.some((s) => !s)) throw new Error("no section headed " + titles.join(", "));
    for (const s of found) s.dataset.shot = "";
    return true;
  })()`);

/** Marks the element a selector finds for a shot of it alone. */
const markElement = (selector) =>
  browser.eval(`(() => {
    const el = document.querySelector(${JSON.stringify(selector)});
    if (!el) throw new Error("no " + ${JSON.stringify(selector)});
    el.dataset.shot = "";
    return true;
  })()`);

/**
 * The region the marked elements take with their notes, and a margin, inside the window: above
 * them top, and side to the left, right and below. In the page's coordinates, which a capture's
 * region is in, rather than the window's.
 */
const around = ({ top = 16, side = 16 } = {}) =>
  browser.eval(`(() => {
    const rects = [...document.querySelectorAll("[data-shot], [data-note]")]
      .map((el) => el.getBoundingClientRect());
    const x = Math.max(0, Math.min(...rects.map((r) => r.left)) - ${side});
    const y = Math.max(0, Math.min(...rects.map((r) => r.top)) - ${top});
    const right = Math.min(innerWidth, Math.max(...rects.map((r) => r.right)) + ${side});
    const bottom = Math.min(innerHeight, Math.max(...rects.map((r) => r.bottom)) + ${side});
    return { x: x + scrollX, y: y + scrollY, width: right - x, height: bottom - y };
  })()`);

/**
 * The pointer on a chart's day, its readout showing. Found by the readout rather than worked out
 * from the chart's geometry: the pointer goes to the middle of the strip in view, reads the day
 * the readout says, and steps by the width of a day until it says the one asked for.
 */
async function point(label, day) {
  const [left, width, y, days] = await browser.eval(`(() => {
    const strip = document.querySelector('[role=group][aria-label^=${JSON.stringify(label)}] .overflow-x-auto');
    const r = strip.getBoundingClientRect();
    return [r.left, r.width, Math.round(r.top + r.height / 2), Number(new URLSearchParams(location.search).get("days") ?? 30)];
  })()`);
  const band = width / days;
  let x = left + width / 2;
  for (let tries = 0; tries < 10; tries++) {
    await browser.send("Input.dispatchMouseEvent", {
      type: "mouseMoved",
      x: Math.round(x),
      y,
    });
    const shown = await until("the readout", () =>
      browser.eval(
        `/\\d{4}-\\d{2}-\\d{2}/.exec([...document.querySelectorAll(".pointer-events-none.fixed.z-50")].map((e) => e.textContent).join(" "))?.[0]`,
      ),
    );
    if (shown === day) return;
    x += ((Date.parse(day) - Date.parse(shown)) / 86_400_000) * band;
  }
  die(`the pointer found no ${day} on ${label}`);
}

/** The charts drawn as Bars or Lines, which the page keeps from one visit to the next. */
const chartsAs = (mode) =>
  browser.eval(`(() => {
    const button = [...document.querySelectorAll('[role=group][aria-label="Charts as"] button')]
      .find((b) => (b.getAttribute("aria-label") ?? b.textContent.trim()) === ${JSON.stringify(mode)});
    if (!button) throw new Error("no " + ${JSON.stringify(mode)} + " button");
    button.click();
    return true;
  })()`);

/**
 * Popovers over the page, each pointing at what it explains.
 *
 * A note is { at, text?, say, side, dx?, dy?, width? }: at is a selector, and text picks the one
 * whose words include it. The popover goes on that side of it, nudged by dx and dy, with an arrow
 * on the edge facing it. Shown as a popover, because a dialog is in the top layer and anything
 * else is drawn under it.
 */
const annotate = (notes) =>
  browser.eval(`(() => {
    const notes = ${JSON.stringify(notes)};
    const dark = matchMedia("(prefers-color-scheme: dark)").matches;
    const ground = dark ? "#f4f4f5" : "#1d1d20", ink = dark ? "#18181b" : "#fafafa";
    const host = document.createElement("div");
    host.popover = "manual";
    host.style.cssText = "position:fixed;inset:0;width:100vw;height:100vh;margin:0;padding:0;border:0;background:none;overflow:visible;pointer-events:none";
    document.body.append(host);
    host.showPopover();
    for (const n of notes) {
      const target = [...document.querySelectorAll(n.at)].find((el) => !n.text || el.textContent.includes(n.text));
      if (!target) throw new Error("no " + n.at + (n.text ? " saying " + n.text : ""));
      const r = target.getBoundingClientRect();
      const box = document.createElement("div");
      box.dataset.note = "";
      box.textContent = n.say;
      box.style.cssText = "position:absolute;max-width:" + (n.width ?? 240) + "px;padding:7px 11px;border-radius:9px;" +
        "background:" + ground + ";color:" + ink + ";font:500 13px/1.35 system-ui,sans-serif;" +
        "box-shadow:0 1px 2px rgb(0 0 0 / 0.12),0 8px 24px rgb(0 0 0 / 0.18)";
      host.append(box);
      const b = box.getBoundingClientRect();
      const gap = 10;
      let x = { right: r.right + gap, left: r.left - gap - b.width }[n.side] ?? r.left + r.width / 2 - b.width / 2;
      let y = { below: r.bottom + gap, above: r.top - gap - b.height }[n.side] ?? r.top + r.height / 2 - b.height / 2;
      x = Math.min(innerWidth - b.width - 10, Math.max(10, x + (n.dx ?? 0)));
      y = Math.min(innerHeight - b.height - 10, Math.max(10, y + (n.dy ?? 0)));
      box.style.left = x + "px";
      box.style.top = y + "px";
      // A square turned half over, on the edge that faces the target and level with its middle.
      const arrow = document.createElement("div");
      const along = (from, size, mid) => Math.min(size - 14, Math.max(14, mid - from));
      const horizontal = n.side === "right" || n.side === "left";
      const at = horizontal ? along(y, b.height, r.top + r.height / 2) : along(x, b.width, r.left + r.width / 2);
      arrow.style.cssText = "position:absolute;width:10px;height:10px;background:" + ground + ";transform:rotate(45deg);" +
        (horizontal ? "top:" + (at - 5) + "px;" + (n.side === "right" ? "left:-4px" : "right:-4px")
                    : "left:" + (at - 5) + "px;" + (n.side === "below" ? "top:-4px" : "bottom:-4px"));
      box.append(arrow);
    }
    return true;
  })()`);

/**
 * Everything at rest: the caret hidden and every transition and animation finished, which would
 * otherwise differ from run to run.
 */
const still = () =>
  browser.eval(`(() => {
    const style = document.createElement("style");
    style.textContent =
      "*, ::before, ::after { caret-color: transparent !important; transition: none !important; }";
    document.head.append(style);
    for (const animation of document.getAnimations()) animation.finish();
    return new Promise((done) => requestAnimationFrame(() => requestAnimationFrame(() => done(true))));
  })()`);

// Each shot brings the page somewhere and is taken there. A shot is the whole window unless it
// marks sections or names an element, which are photographed alone; phone takes it as a phone
// does.
const shots = [
  {
    name: "dashboard",
    window: [1280, 960],
    go: async () => {
      await dashboard("/?days=30");
      await chartsAs("Bars");
      await settled();
      await point("First-time downloads per day", "2026-11-10");
    },
  },
  {
    name: "country",
    window: [1280, 900],
    element: "dialog[open]",
    go: async () => {
      await dashboard("/?days=90");
      await click('button[aria-label="United States: each app"]');
      await until("the country's dialog", () =>
        browser.eval(`!!document.querySelector("dialog[open]")`),
      );
    },
  },
  {
    name: "app",
    window: [1280, 960],
    go: async () => {
      await dashboard(`/apps/${TIDEPOOL}?days=365`);
      await chartsAs("Lines");
    },
  },
  {
    name: "store",
    window: [1280, 2600],
    go: async () => {
      await dashboard("/?days=90");
      await chartsAs("Bars");
      await mark("App Store");
    },
  },
  {
    name: "status",
    window: [1280, 760],
    go: async () => {
      await browser.open("/status");
      await until("the reports by app", () =>
        browser.eval(
          `document.body.textContent.includes("Analytics reports by app")`,
        ),
      );
    },
  },
  {
    name: "webhooks",
    window: [1280, 1400],
    // A panel's own space above its heading, without the one before it.
    top: 0,
    go: async () => {
      await browser.open("/settings");
      await until("the webhook addresses", () =>
        browser.eval(
          `document.body.textContent.includes("Tidepool: App Store Connect")`,
        ),
      );
      await settled();
      await mark("Webhooks", "Webhook mail");
    },
  },
  {
    name: "phone",
    window: [390, 844],
    phone: true,
    go: async () => {
      await dashboard("/?days=30");
      await chartsAs("Lines");
    },
  },
];

// The fonts in before any shot: a chart lays its labels out by their width when it is first
// drawn, so a first page drawn before them would set its labels apart from every later one's.
await browser.open("/?days=30");
await until("the fonts", () =>
  browser.eval(
    `document.fonts.ready.then(() => document.fonts.status === "loaded")`,
  ),
);

step(`photographing at ${SCALE}x, ${THEME}`);
mkdirSync(OUT, { recursive: true });
for (const shot of shots) {
  if (ONLY.length && !ONLY.includes(shot.name)) continue;
  // A step waiting on something that never comes would otherwise wait forever.
  awaiting = "";
  const limit = setTimeout(
    () =>
      die(
        `${shot.name} took more than ${TIMEOUT} seconds${awaiting ? `, waiting for ${awaiting}` : ""}`,
      ),
    TIMEOUT * 1000,
  );
  await browser.viewport(...shot.window, SCALE, shot.phone);
  await shot.go();
  await settled();
  awaiting = "the page to come to rest";
  await still();
  if (NOTES && shot.notes) await annotate(shot.notes);
  if (shot.element) await markElement(shot.element);
  const clip = (await browser.eval(`!!document.querySelector("[data-shot]")`))
    ? await around({ top: shot.top })
    : undefined;
  const file = join(OUT, `${shot.name}.png`);
  const png = stamp(await browser.png(clip), SCALE);
  const before = existsSync(file) ? readFileSync(file) : null;
  if (before && alike(before, png, SCALE)) {
    // The same picture, kept; its metadata brought up to date if the stamp has changed.
    const restamped = stamp(before, SCALE);
    if (!restamped.equals(before)) writeFileSync(file, restamped);
    console.log(`  ${shot.name}.png, unchanged`);
  } else {
    writeFileSync(file, png);
    console.log(`  ${shot.name}.png`);
  }
  clearTimeout(limit);
}

step("done");
process.exit(0);

/**
 * The density a PNG was drawn at, said the two ways a macOS screenshot says it: a pHYs chunk,
 * and an EXIF block with the same resolution in inches, which is what some of the system reads
 * instead. A 2x capture says 144 dpi, and a viewer that reads it shows it at the size it was on
 * screen rather than twice that.
 */
function stamp(png, scale) {
  const chunk = (type, data) => {
    const head = Buffer.alloc(4);
    head.writeUInt32BE(data.length);
    const body = Buffer.concat([Buffer.from(type), data]);
    const tail = Buffer.alloc(4);
    tail.writeUInt32BE(crc32(body));
    return Buffer.concat([head, body, tail]);
  };
  const dpi = 72 * scale;
  const phys = Buffer.alloc(9);
  phys.writeUInt32BE(Math.round(dpi / 0.0254), 0);
  phys.writeUInt32BE(Math.round(dpi / 0.0254), 4);
  phys[8] = 1; // the unit is the metre

  // A big-endian TIFF header and one directory of three entries: XResolution and YResolution,
  // each a rational stored after the directory, and ResolutionUnit, 2 being the inch.
  const exif = Buffer.alloc(66);
  exif.write("MM", 0, "latin1");
  exif.writeUInt16BE(42, 2);
  exif.writeUInt32BE(8, 4);
  exif.writeUInt16BE(3, 8);
  const entry = (at, tag, type, value) => {
    exif.writeUInt16BE(tag, at);
    exif.writeUInt16BE(type, at + 2);
    exif.writeUInt32BE(1, at + 4);
    if (type === 3) exif.writeUInt16BE(value, at + 8);
    else exif.writeUInt32BE(value, at + 8);
  };
  entry(10, 0x011a, 5, 50);
  entry(22, 0x011b, 5, 58);
  entry(34, 0x0128, 3, 2);
  exif.writeUInt32BE(0, 46); // no next directory
  for (const at of [50, 58]) {
    exif.writeUInt32BE(dpi, at);
    exif.writeUInt32BE(1, at + 4);
  }

  // After IHDR, which is always first, and in place of any already there.
  const parts = [png.subarray(0, 8)];
  for (let at = 8; at < png.length;) {
    const length = png.readUInt32BE(at);
    const type = png.toString("latin1", at + 4, at + 8);
    const end = at + 12 + length;
    if (type !== "pHYs" && type !== "eXIf") parts.push(png.subarray(at, end));
    if (type === "IHDR") parts.push(chunk("pHYs", phys), chunk("eXIf", exif));
    at = end;
  }
  return Buffer.concat(parts);
}

/**
 * Whether two captures are one picture, give or take the rasteriser.
 *
 * A Chromium update redraws the edges of letters a pixel over here and there, all over the
 * picture, which pixel by pixel reads as a change. Compared instead block by block, 8 by 8 CSS
 * pixels, by average colour: a letter that moved inside its block leaves the average where it
 * was, and a changed word, icon or colour moves it by far more than 24 of 255.
 */
function alike(before, after, scale) {
  const a = decode(before);
  const b = decode(after);
  if (
    !a ||
    !b ||
    a.width !== b.width ||
    a.height !== b.height ||
    a.channels !== b.channels
  ) {
    return false;
  }
  const size = 8 * scale;
  for (let top = 0; top < a.height; top += size) {
    for (let left = 0; left < a.width; left += size) {
      for (let c = 0; c < a.channels; c++) {
        let sum = 0;
        let n = 0;
        for (let y = top; y < Math.min(top + size, a.height); y++) {
          for (let x = left; x < Math.min(left + size, a.width); x++) {
            const at = (y * a.width + x) * a.channels + c;
            sum += a.pixels[at] - b.pixels[at];
            n++;
          }
        }
        if (Math.abs(sum) / n > 24) return false;
      }
    }
  }
  return true;
}

/** The pixels of an 8-bit, non-interlaced RGB or RGBA PNG, which is what Chromium writes. */
function decode(png) {
  let width, height, channels;
  const data = [];
  for (let at = 8; at < png.length;) {
    const length = png.readUInt32BE(at);
    const type = png.toString("latin1", at + 4, at + 8);
    const body = png.subarray(at + 8, at + 8 + length);
    if (type === "IHDR") {
      width = body.readUInt32BE(0);
      height = body.readUInt32BE(4);
      channels = { 2: 3, 6: 4 }[body[9]];
      if (body[8] !== 8 || !channels || body[12] !== 0) return null;
    }
    if (type === "IDAT") data.push(body);
    at += 12 + length;
  }
  const raw = inflateSync(Buffer.concat(data));
  const stride = width * channels;
  const pixels = Buffer.alloc(height * stride);
  for (let y = 0; y < height; y++) {
    const filter = raw[y * (stride + 1)];
    const line = y * (stride + 1) + 1;
    for (let x = 0; x < stride; x++) {
      const a = x >= channels ? pixels[y * stride + x - channels] : 0;
      const b = y > 0 ? pixels[(y - 1) * stride + x] : 0;
      const c =
        x >= channels && y > 0 ? pixels[(y - 1) * stride + x - channels] : 0;
      let value = raw[line + x];
      if (filter === 1) value += a;
      else if (filter === 2) value += b;
      else if (filter === 3) value += (a + b) >> 1;
      else if (filter === 4) {
        const p = a + b - c;
        const pa = Math.abs(p - a),
          pb = Math.abs(p - b),
          pc = Math.abs(p - c);
        value += pa <= pb && pa <= pc ? a : pb <= pc ? b : c;
      }
      pixels[y * stride + x] = value & 255;
    }
  }
  return { width, height, channels, pixels };
}
