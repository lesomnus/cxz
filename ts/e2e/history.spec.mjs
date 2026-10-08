import { test, expect } from "@playwright/test";
import { readFileSync } from "node:fs";
import { resolve } from "node:path";
// Optional local replay uses private recorded data without checking it into git.
// Preserve event identity, text, payloads and timing; only convert wire names.
function replayEvents() {
  if (process.env.CXZ_REPLAY_EVENTS) {
    const rows = JSON.parse(
      readFileSync(process.env.CXZ_REPLAY_EVENTS, "utf8"),
    );
    const through = Number(process.env.CXZ_REPLAY_THROUGH ?? Infinity);
    return rows
      .flat()
      .filter((e) => Number(e.seq) <= through)
      .map((e) => ({
        seq: String(e.seq),
        kind: e.kind,
        text: e.text ?? "",
        runId: e.run_id ?? e.runId ?? "",
        requestId: e.request_id ?? e.requestId ?? "",
        timeMs: String(e.time_ms ?? e.timeMs ?? 0),
        payload: Buffer.from(JSON.stringify(e.payload ?? {})).toString(
          "base64",
        ),
        ...(e.response
          ? {
              response: {
                model: e.response.model ?? "",
                effort: e.response.effort ?? "",
                phase: e.response.phase ?? "",
                modelSource: e.response.model_source ?? "",
                effortSource: e.response.effort_source ?? "",
                completionJson: e.response.completion_json
                  ? Buffer.from(
                      typeof e.response.completion_json === "string"
                        ? e.response.completion_json
                        : JSON.stringify(e.response.completion_json),
                    ).toString("base64")
                  : "",
              },
            }
          : {}),
      }));
  }
  // A protocol-heavy journal: most RPC pages add no visible content.
  return Array.from({ length: 10000 }, (_, index) => {
    const seq = index + 1;
    const kind =
      seq % 200 === 0 ? "input" : seq % 200 === 1 ? "assistant" : "raw";
    return {
      seq: String(seq),
      kind,
      runId: "run",
      text: kind === "raw" ? "" : `Recorded ${kind} ${seq}`,
      timeMs: String(1700000000000 + seq),
    };
  });
}
async function openReplay(page, summary = false, options = {}) {
  const events = options.events ?? replayEvents();
  const latest = Number(events.at(-1).seq);
  // Serve the current build even when an already-running fixture embeds an old one.
  await page.route("https://127.0.0.1:18081/**", async (route) => {
    const path = new URL(route.request().url()).pathname;
    if (
      route.request().method() !== "GET" ||
      (path !== "/" && !path.startsWith("/assets/"))
    )
      return route.continue();
    const file = resolve(
      "../internal/webui/assets",
      path === "/" ? "index.html" : path.slice(1),
    );
    const extension = file.slice(file.lastIndexOf("."));
    const contentType = {
      ".html": "text/html",
      ".js": "application/javascript",
      ".css": "text/css",
      ".ttf": "font/ttf",
    }[extension];
    await route.fulfill({ body: readFileSync(file), contentType });
  });
  if (!summary)
    await page.route("**/cxz.SessionService/Transcript", (route) =>
      route.fulfill({
        status: 501,
        json: { code: "unimplemented", message: "legacy project runtime" },
      }),
    );
  else if (!process.env.CXZ_REPLAY_EVENTS)
    await page.route("**/cxz.SessionService/Transcript", async (route) => {
      const q = route.request().postDataJSON();
      if (options.delay && q.beforeSeq && q.limit > 1)
        await new Promise((resolve) => setTimeout(resolve, options.delay));
      const all = events.filter((e) => e.kind !== "raw");
      const eligible = all.filter(
        (e) =>
          (!q.beforeSeq || Number(e.seq) < Number(q.beforeSeq)) &&
          (!q.afterSeq || Number(e.seq) > Number(q.afterSeq)),
      );
      const limit = q.limit || 256;
      const rows = q.afterSeq
        ? eligible.slice(0, limit)
        : eligible.slice(-limit);
      const first = Number(rows[0]?.seq ?? 0),
        last = Number(rows.at(-1)?.seq ?? 0);
      await route.fulfill({
        json: {
          events: rows,
          snapshotSeq: String(latest),
          hasOlder: all.some((e) => Number(e.seq) < first),
          hasNewer: all.some((e) => Number(e.seq) > last),
          precedingInput: all
            .filter((e) => e.kind === "input" && Number(e.seq) < first)
            .at(-1),
        },
      });
    });
  await page.route("**/cxz.SessionService/History", async (route) => {
    const after = Number(route.request().postDataJSON().afterSeq ?? 0);
    await route.fulfill({
      json: {
        events: events.filter((e) => Number(e.seq) > after).slice(0, 128),
      },
    });
  });

  await page.route("**/cxz.SessionService/Get", async (route) => {
    const response = await route.fetch();
    const json = await response.json();
    json.status.lastSeq = String(latest);
    json.status.runId = events.at(-1).runId;
    json.status.pending = [];
    json.agent = "codex";
    await route.fulfill({ response, json });
  });
  // A quiet stream after the recorded snapshot: no simulated new responses.
  await page.route("**/cxz.SessionService/Events", (route) =>
    route.fulfill({
      contentType: "application/connect+json",
      body: Buffer.from([2, 0, 0, 0, 2, 123, 125]),
    }),
  );
  await page.request.post("/auth/login", {
    headers: { Origin: "https://127.0.0.1:18081" },
    data: { token: "a".repeat(32) },
  });
  await page.goto("/");
  await page.getByRole("button", { name: /demo-chat/ }).click();
  return latest;
}
const geometry = (page) =>
  page.locator(".transcript").evaluate((el) => ({
    top: el.scrollTop,
    max: el.scrollHeight - el.clientHeight,
    height: el.clientHeight,
  }));
// A mounted message's position, not scrollTop, survives prepending history.
async function readingAnchor(page) {
  return page.locator(".transcript").evaluate((el) => {
    const bounds = el.getBoundingClientRect();
    const rows = [...el.querySelectorAll("[data-row]")];
    const row = rows.find(
      (row) => row.getBoundingClientRect().bottom > bounds.top,
    );
    return {
      id: row.dataset.row,
      offset: row.getBoundingClientRect().top - bounds.top,
    };
  });
}
test.use({
  viewport: { width: 1440, height: 1300 },
  isMobile: false,
  hasTouch: false,
  deviceScaleFactor: 1,
});
test.afterEach(async ({ page }) => {
  // Session refreshes can still be fetching when the scroll assertions finish.
  // Complete those handlers before Playwright disposes their API responses.
  await page.unrouteAll({ behavior: "wait" });
});
for (const mode of ["legacy", "summary"])
  test(`${mode} recorded pages fill a tall pane and keep scrolling usable through resize`, async ({
    page,
  }) => {
    const errors = [];
    page.on("pageerror", (e) => errors.push(e.message));
    const calls = { history: 0, transcript: 0, details: 0 };
    let latest = Infinity;
    page.on("request", (request) => {
      if (
        request.url().endsWith("/History") &&
        Number(request.postDataJSON().afterSeq ?? 0) < latest
      )
        calls.history++;
      if (request.url().endsWith("/Transcript")) calls.transcript++;
      if (request.url().endsWith("/EventDetails")) calls.details++;
    });
    latest = await openReplay(page, mode === "summary");
    await expect
      .poll(async () => (await geometry(page)).max)
      .toBeGreaterThan(0);
    await expect
      .poll(async () => {
        const g = await geometry(page);
        return g.max - g.top;
      })
      .toBeLessThan(1);
    await expect
      .poll(async () =>
        Number(
          await page.locator(".virtual-messages").getAttribute("data-first"),
        ),
      )
      .toBeLessThan(latest - 256);
    await page.locator(".transcript").hover();
    await page.mouse.wheel(0, -240);
    await expect
      .poll(async () => {
        const g = await geometry(page);
        return g.max - g.top;
      })
      .toBeGreaterThan(100);
    // Wait for near-edge prefetch and row measurements to settle.
    await page.waitForTimeout(400);
    const anchor = await readingAnchor(page);
    await page.setViewportSize({ width: 1100, height: 750 });
    await expect
      .poll(async () =>
        page
          .locator(`[data-row="${anchor.id}"]`)
          .evaluate(
            (el) =>
              el.getBoundingClientRect().top -
              el.closest(".transcript").getBoundingClientRect().top,
          ),
      )
      .toBeCloseTo(anchor.offset, 0);
    const g = await geometry(page);
    expect(g.top).toBeGreaterThanOrEqual(0);
    expect(g.top).toBeLessThanOrEqual(g.max);
    await page.locator(".transcript").hover();
    const thumb = page.locator(".scroll-thumb");
    const bounds = await thumb.boundingBox();
    const area = await page.locator(".transcript-area").boundingBox();
    await page.mouse.move(
      bounds.x + bounds.width / 2,
      bounds.y + bounds.height / 2,
    );
    await page.mouse.down();
    await page.mouse.move(bounds.x + bounds.width / 2, area.y - 80);
    await page.waitForTimeout(250);
    const pulled = await thumb.boundingBox();
    expect(pulled.y).toBeGreaterThanOrEqual(area.y);
    expect(pulled.y + pulled.height).toBeLessThanOrEqual(area.y + area.height);
    await page.mouse.move(bounds.x + bounds.width / 2, area.y + 100);
    await page.mouse.up();
    await page.waitForTimeout(400);
    const read = await readingAnchor(page);
    await page.waitForTimeout(400);
    expect((await readingAnchor(page)).id).toBe(read.id);
    expect(
      Number(
        await page.locator(".virtual-messages").getAttribute("data-cached"),
      ),
    ).toBeLessThanOrEqual(8192);
    expect(await page.locator(".transcript [data-row]").count()).toBeLessThan(
      60,
    );
    expect(errors).toEqual([]);
    if (process.env.CXZ_REPLAY_EVENTS)
      console.log(
        `${mode} replay RPCs: History=${calls.history}, Transcript=${calls.transcript}, EventDetails=${calls.details}`,
      );
    if (mode === "summary") {
      expect(calls.history).toBe(0);
      expect(calls.transcript).toBeLessThan(10);
      expect(calls.details).toBe(0);
      const task = page.locator(".tool-activity").first();
      if (await task.count()) {
        await task.click();
        await expect.poll(() => calls.details).toBe(1);
        await expect(
          page.locator(".card-body").getByText("Loading…"),
        ).toHaveCount(0);
        await expect(page.locator(".card-body pre").first()).toBeVisible();
      }
    }
  });

test("summary history prefetches before the loaded edge even with delayed responses", async ({
  page,
}) => {
  const events = Array.from({ length: 12000 }, (_, index) => ({
    seq: String(index + 1),
    kind: index % 2 ? "assistant" : "input",
    runId: "run",
    text: `Cached message ${index + 1}`,
  }));
  const requests = [];
  page.on("request", (request) => {
    if (request.url().endsWith("/Transcript"))
      requests.push(request.postDataJSON());
  });
  await openReplay(page, true, { events, delay: 400 });
  const pane = page.locator(".transcript");
  const messages = page.locator(".virtual-messages");
  await expect(messages).toHaveAttribute("data-cached", "1024");
  await expect
    .poll(async () => (await geometry(page)).top)
    .toBeGreaterThan(10000);
  expect(requests[0].limit).toBe(1024);
  const first = BigInt(await messages.getAttribute("data-first"));
  await pane.evaluate((el) => {
    el.scrollTop = el.clientHeight * 5;
  });
  await expect
    .poll(() => requests.filter((q) => q.beforeSeq && q.limit > 1).length)
    .toBe(1);
  // We still have local content to read during the network round trip.
  expect((await geometry(page)).top).toBeGreaterThan(
    (await geometry(page)).height * 2,
  );
  await page.waitForTimeout(100);
  const anchor = await readingAnchor(page);
  await expect
    .poll(async () => BigInt(await messages.getAttribute("data-first")))
    .toBeLessThan(first);
  await expect
    .poll(async () => (await readingAnchor(page)).offset)
    .toBeCloseTo(anchor.offset, 0);
  expect((await readingAnchor(page)).id).toBe(anchor.id);
  expect(requests.filter((q) => q.beforeSeq && q.limit > 1)).toHaveLength(1);
  expect(await pane.locator("[data-row]").count()).toBeLessThan(60);
});
