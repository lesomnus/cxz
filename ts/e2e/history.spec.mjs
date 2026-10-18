import { test, expect, devices } from "@playwright/test";
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
async function openReplay(page, options = {}) {
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
  if (!process.env.CXZ_REPLAY_EVENTS)
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
  await page
    .getByRole("link", { name: /demo-chat/ })
    .first()
    .click();
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

for (const reduced of [false, true]) {
  test(`conversation handle ${reduced ? "respects reduced motion" : "eases toward native scroll positions"} without moving message markers`, async ({
    page,
  }) => {
    await page.emulateMedia({
      reducedMotion: reduced ? "reduce" : "no-preference",
    });
    const events = Array.from({ length: 400 }, (_, i) => ({
      seq: String(i + 1),
      runId: "run",
      kind: i % 2 ? "assistant" : "input",
      text: `Scroll sample ${i + 1}`,
      timeMs: "1700000000000",
    }));
    await openReplay(page, { events });
    const pane = page.locator(".transcript");
    const thumb = page.locator(".scroll-thumb");
    await expect(pane).toHaveAttribute("aria-busy", "false");
    // Initial follow goes directly to its final position without flying in.
    await expect(page.locator(".transcript-area")).toHaveClass(
      /scroll-following/,
    );
    await expect(thumb).toHaveCSS("transition-duration", "0s");
    await pane.hover();
    await page.mouse.wheel(0, -1200);
    await expect(page.locator(".transcript-area")).not.toHaveClass(
      /scroll-following|scroll-rebasing/,
    );
    const settled = () =>
      thumb.evaluate((el) =>
        Math.abs(
          parseFloat(getComputedStyle(el).top) - Number(el.dataset.targetTop),
        ),
      );
    await expect.poll(settled).toBeLessThan(0.1);
    const samples = await page.evaluate(async () => {
      const pane = document.querySelector(".transcript");
      const thumb = document.querySelector(".scroll-thumb");
      // Wait for the rail to stop changing shape before recording anything
      // from it. A transcript of 400 rows reaches its real height over several
      // frames as estimated row sizes are replaced by measured ones, and the
      // markers move with it -- so a marker position recorded mid-measure
      // moves inside the sampling window below, which is the assertion. The
      // thumb is already waited for; this is the same wait for the rail. See
      // cxz#207.
      const shape = () =>
        `${pane.scrollHeight}|${[...document.querySelectorAll(".scroll-marker")]
          .map((el) => el.style.top)
          .join(",")}`;
      for (let last = "", agreed = 0, i = 0; agreed < 2 && i < 180; i++) {
        await new Promise(requestAnimationFrame);
        const now = shape();
        if (now === last) agreed++;
        else {
          last = now;
          agreed = 0;
        }
      }
      const before = parseFloat(getComputedStyle(thumb).top);
      const start = thumb.dataset.start;
      const marker = [...document.querySelectorAll(".scroll-marker")].find(
        (el) => {
          const top = parseFloat(el.style.top);
          return top > 0 && top < el.parentElement.clientHeight;
        },
      );
      const markerTop = marker.style.top;
      pane.scrollTop -= 800;
      const frames = [];
      const until = performance.now() + 350;
      do {
        await new Promise(requestAnimationFrame);
        frames.push({
          top: parseFloat(getComputedStyle(thumb).top),
          target: Number(thumb.dataset.targetTop),
          start: thumb.dataset.start,
          markerTop: marker.style.top,
        });
      } while (performance.now() < until);
      return { before, start, markerTop, frames };
    });
    expect(samples.frames.every((f) => f.start === samples.start)).toBe(true);
    expect(samples.frames.every((f) => f.markerTop === samples.markerTop)).toBe(
      true,
    );
    const changed = samples.frames.filter(
      (f) => Math.abs(f.target - samples.before) > 5,
    );
    expect(changed.length).toBeGreaterThan(0);
    const intermediate = changed.filter(
      (f) => f.top < samples.before - 0.1 && f.top > f.target + 0.1,
    );
    expect(intermediate.length > 0).toBe(!reduced);
    await expect.poll(settled).toBeLessThan(0.1);

    // Keep changing the target for multiple frames. An isolated jump does not
    // catch a handle that waits until scrolling stops before catching up.
    const continuous = await page.evaluate(async () => {
      const pane = document.querySelector(".transcript");
      const thumb = document.querySelector(".scroll-thumb");
      const before = parseFloat(getComputedStyle(thumb).top);
      const frames = [];
      for (let i = 0; i < 20; i++) {
        pane.scrollTop -= 48;
        await new Promise(requestAnimationFrame);
        frames.push({
          top: parseFloat(getComputedStyle(thumb).top),
          target: Number(thumb.dataset.targetTop),
        });
      }
      return { before, frames };
    });
    expect(continuous.frames[7].top).toBeLessThan(continuous.before - 5);
    const moving = continuous.frames.slice(5);
    expect(moving.every((f, i) => !i || f.top < moving[i - 1].top)).toBe(true);
    const last = moving.at(-1);
    expect(
      (continuous.before - last.top) / (continuous.before - last.target),
    ).toBeGreaterThan(0.8);
    await expect.poll(settled).toBeLessThan(0.1);
  });
}
test("recorded pages fill a tall pane and keep scrolling usable through resize", async ({
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
  latest = await openReplay(page);
  await expect.poll(async () => (await geometry(page)).max).toBeGreaterThan(0);
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
    Number(await page.locator(".virtual-messages").getAttribute("data-cached")),
  ).toBeLessThanOrEqual(8192);
  expect(await page.locator(".transcript [data-row]").count()).toBeLessThan(60);
  expect(errors).toEqual([]);
  if (process.env.CXZ_REPLAY_EVENTS)
    console.log(
      `replay RPCs: History=${calls.history}, Transcript=${calls.transcript}, EventDetails=${calls.details}`,
    );
  {
    expect(calls.history).toBe(0);
    expect(calls.transcript).toBeLessThan(10);
    expect(calls.details).toBe(0);
    const summary = page.locator("[data-work-summary] button").first();
    if (await summary.count()) {
      await summary.click();
      await page.locator(".tool-activity").first().click();
      await expect.poll(() => calls.details).toBe(1);
      await expect(
        page.locator(".card-body").getByText("Loading…"),
      ).toHaveCount(0);
      await expect(
        page.locator(".card-body .monaco-editor").first(),
      ).toBeVisible();
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
  await openReplay(page, { events, delay: 400 });
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

test("viewport fades cover response bodies while the scrollbar and controls remain available", async ({
  page,
}) => {
  await page.setViewportSize({ width: 1440, height: 700 });
  const events = Array.from({ length: 50 }, (_, index) => {
    const command = `/usr/bin/zsh -lc 'echo task-${index}'`;
    return ["tool_call", "tool_result", "assistant"].map((kind, offset) => ({
      seq: String(index * 3 + offset + 1),
      runId: "run",
      requestId: `task-${index}`,
      kind,
      text:
        kind === "assistant"
          ? `Task ${index} completed.\n\nA response with enough content to read while scrolling.`
          : command,
      timeMs: String(1700000000000 + index * 3 + offset),
      ...(kind !== "assistant"
        ? {
            payload: Buffer.from(
              JSON.stringify({
                item: {
                  type: "commandExecution",
                  command,
                  status: offset ? "completed" : "inProgress",
                  ...(offset ? { exitCode: 0, aggregatedOutput: "done" } : {}),
                },
              }),
            ).toString("base64"),
          }
        : {}),
    }));
  }).flat();
  await openReplay(page, { events });
  const pane = page.locator(".transcript");
  await expect(page.locator(".response").first()).toBeAttached();
  expect(await page.locator(".scroll-response-marker").count()).toBeGreaterThan(
    0,
  );
  await pane.evaluate((el) => {
    el.scrollTop = (el.scrollHeight - el.clientHeight) / 2;
  });
  for (const edge of ["top", "bottom"]) {
    const fade = page.locator(`.transcript-fade-${edge}`);
    await expect(fade).toHaveCSS("opacity", "1");
    await expect
      .poll(() => fade.evaluate((el) => el.getBoundingClientRect().height))
      .toBeGreaterThan(16);
    // Coverage is a box overlap, and the layering is hit-tested at a point
    // taken from inside that overlap. Neither depends on scrolling a chosen row
    // to a computed offset, which is what used to decide this test: a scroll
    // that landed a pixel short, or a point that fell in the gap between two
    // rows, read as a layering failure.
    const layers = await pane.evaluate((el, edge) => {
      const area = el.closest(".transcript-area");
      const fade = area.querySelector(`.transcript-fade-${edge}`);
      const box = fade.getBoundingClientRect();
      const row = [...el.querySelectorAll(".response .markdown")]
        .map((node) => ({ node, rect: node.getBoundingClientRect() }))
        .find(
          ({ rect }) => rect.top < box.bottom - 1 && rect.bottom > box.top + 1,
        );
      if (!row) {
        return { covered: false };
      }
      // Horizontally the row's own centre, which is inside the content column
      // and clear of the scrollbar; vertically the middle of the overlap.
      const x = row.rect.x + row.rect.width / 2;
      const y =
        (Math.max(row.rect.top, box.top) +
          Math.min(row.rect.bottom, box.bottom)) /
        2;
      fade.style.pointerEvents = "auto";
      const fadeOnTop = document.elementFromPoint(x, y) === fade;
      const thumb = area.querySelector(".scroll-thumb");
      const handle = thumb.getBoundingClientRect();
      const scrollbarAbove = thumb.contains(
        document.elementFromPoint(handle.x + handle.width / 2, handle.y + 2),
      );
      fade.style.pointerEvents = "none";
      const stillInteractive = row.node.contains(
        document.elementFromPoint(x, y),
      );
      return { covered: true, fadeOnTop, scrollbarAbove, stillInteractive };
    }, edge);
    expect(layers).toEqual({
      covered: true,
      fadeOnTop: true,
      scrollbarAbove: true,
      stillInteractive: true,
    });
  }
});

test("file changes show per-file actions and deltas without fetching details", async ({
  page,
}) => {
  const changes = [
    {
      path: "/workspace/ts/src/app.tsx",
      kind: { type: "update" },
      diff: "@@ -1,11 +1,25 @@\n" + "-old\n".repeat(11) + "+new\n".repeat(25),
    },
    { path: "/workspace/ts/src/send-motion.ts", kind: { type: "add" } },
    {
      path: "/workspace/ts/src/style.css",
      kind: { type: "update" },
      diff: "@@ -0,0 +1,32 @@\n" + "+style\n".repeat(32),
    },
  ];
  const native = [
    {
      seq: "1",
      kind: "input",
      text: "Change the UI",
      runId: "run",
      timeMs: "1700000000000",
    },
    {
      seq: "2",
      kind: "tool_call",
      requestId: "patch",
      runId: "run",
      payload: Buffer.from(
        JSON.stringify({
          item: { type: "fileChange", status: "inProgress", changes: [] },
        }),
      ).toString("base64"),
    },
    {
      seq: "3",
      kind: "tool_result",
      requestId: "patch",
      runId: "run",
      payload: Buffer.from(
        JSON.stringify({
          item: { type: "fileChange", status: "completed", changes },
        }),
      ).toString("base64"),
    },
    { seq: "4", kind: "assistant", text: "Changes completed", runId: "run" },
  ];
  native.push(
    {
      seq: "5",
      kind: "approval",
      text: "Write",
      requestId: "approve",
      runId: "run",
      payload: Buffer.from(
        JSON.stringify({ tool_use_id: "patch", file_path: changes[1].path }),
      ).toString("base64"),
    },
    {
      seq: "6",
      kind: "approval_resolved",
      text: "allowed",
      requestId: "approve",
      runId: "run",
    },
  );
  const events = [
    native[0],
    {
      ...native[1],
      payload: undefined,
      toolSummary: {
        name: "Files",
        state: "completed",
        files: [
          {
            path: changes[0].path,
            action: "update",
            added: 25,
            removed: 11,
            measure: "diff",
          },
          { path: changes[1].path, action: "add", measure: "unknown" },
          {
            path: changes[2].path,
            action: "update",
            added: 32,
            removed: 0,
            measure: "diff",
          },
        ],
      },
    },
    native[3],
  ];
  let details = 0;
  await openReplay(page, { events });
  await page.route("**/cxz.SessionService/EventDetails", (route) => {
    details++;
    return route.fulfill({
      json: {
        events: native.filter(
          (event) => !["input", "assistant"].includes(event.kind),
        ),
      },
    });
  });
  await page.locator("[data-work-summary] button").first().click();
  const dialog = page.getByRole("dialog", {
    name: "Tasks between responses",
  });
  const task = dialog.locator('.tool-activity[data-seq="2"]');
  await expect(task).toHaveAttribute("data-state", "completed");
  await expect(task.locator(".tool-file-row")).toHaveText([
    "✓Edit /workspace/ts/src/app.tsx · +25 -11",
    "✓Write /workspace/ts/src/send-motion.ts · Δ?",
    "✓Edit /workspace/ts/src/style.css · +32 -0",
  ]);
  for (const row of await task.locator(".tool-file-row").all())
    await expect(row).toBeVisible();
  await expect(page.locator(".tool-activity")).toHaveCount(1);
  expect(details).toBe(0);
  await expect(
    dialog.getByRole("region", { name: "Task details" }),
  ).toHaveCount(0);
  await task.click();
  const card = dialog.getByRole("region", { name: "Task details" });
  await expect(card).toBeVisible();
  await card.getByRole("tab", { name: "Result", exact: true }).click();
  await expect(card.locator(".view-lines")).toContainText(
    "/workspace/ts/src/send-motion.ts",
  );
  await expect(card.locator(".monaco-editor textarea")).toHaveJSProperty(
    "readOnly",
    true,
  );
  expect(details).toBe(1);
  await page.setViewportSize({ width: 390, height: 844 });
  await expect
    .poll(() => page.evaluate(() => document.documentElement.scrollWidth))
    .toBe(390);
  await expect
    .poll(async () => {
      const box = await card.boundingBox();
      return Math.max(
        0,
        -box.x,
        box.x + box.width - 390,
        box.y + box.height - 844,
      );
    })
    .toBeLessThan(1);
  const pane = page.locator(".transcript");
  const top = await pane.evaluate((el) => el.scrollTop);
  const editor = card.locator(".monaco-editor");
  await editor.hover();
  await page.mouse.wheel(0, 80);
  await expect
    .poll(() =>
      editor
        .locator(".scrollbar.vertical > .slider")
        .first()
        .evaluate((el) => parseFloat(el.style.top)),
    )
    .toBeGreaterThan(0);
  expect(
    await dialog.locator(".card-body").evaluate((el) => el.scrollTop),
  ).toBe(0);
  expect(await pane.evaluate((el) => el.scrollTop)).toBe(top);
  await card.getByRole("tab", { name: "Approval", exact: true }).click();
  await expect(card.locator(".view-lines")).toContainText("allowed");
  await expect(card.locator(".view-lines")).toContainText(
    "/workspace/ts/src/send-motion.ts",
  );
  await expect(card.locator(".detail-editor")).toHaveAttribute(
    "data-language",
    "json",
  );
  await task.click();
  await expect(card).toHaveCount(0);
  await dialog.getByRole("button", { name: "Close task dialog" }).click();
  expect(details).toBe(1);
});

test.describe("task detail editor", () => {
  // Exercise Monaco with Chromium's own identity. The fixture's default iPhone
  // Safari user agent enables a WebKit clipboard workaround in a Chromium page.
  const { defaultBrowserType: _browserType, ...desktop } =
    devices["Desktop Chrome"];
  test.use({
    ...desktop,
    viewport: { width: 1440, height: 900 },
  });
  test("task output detects syntax, scrolls only its Monaco viewport and restores tab reading positions", async ({
    page,
  }) => {
    await page
      .context()
      .grantPermissions(["clipboard-read", "clipboard-write"]);
    const output = Array.from(
      { length: 60 },
      (_, index) =>
        `def greeting_${index}(name):\n    # Print a greeting\n    return f"Hello, {name}"\n`,
    ).join("\n");
    const native = [
      {
        seq: "2",
        kind: "tool_call",
        requestId: "cat",
        runId: "run",
        payload: Buffer.from(
          JSON.stringify({
            item: {
              type: "commandExecution",
              command: "cat greetings.py",
              status: "inProgress",
            },
          }),
        ).toString("base64"),
      },
      {
        seq: "3",
        kind: "tool_result",
        requestId: "cat",
        runId: "run",
        payload: Buffer.from(
          JSON.stringify({
            item: {
              type: "commandExecution",
              status: "completed",
              aggregatedOutput: output,
              exitCode: 0,
            },
          }),
        ).toString("base64"),
      },
    ];
    const errors = [];
    page.on("pageerror", (error) => errors.push(error.stack ?? error.message));
    await openReplay(page, {
      events: [
        { seq: "1", kind: "input", text: "Show the source", runId: "run" },
        {
          ...native[0],
          payload: undefined,
          toolSummary: {
            name: "Bash",
            command: "cat greetings.py",
            state: "completed",
          },
        },
        { seq: "4", kind: "assistant", text: "Source printed", runId: "run" },
      ],
    });
    let requests = 0;
    await page.route("**/cxz.SessionService/EventDetails", (route) => {
      requests++;
      return route.fulfill({ json: { events: native } });
    });
    await page.locator("[data-work-summary] button").first().click();
    const dialog = page.getByRole("dialog", {
      name: "Tasks between responses",
    });
    await dialog.locator(".tool-activity").click();
    const card = dialog.getByRole("region", { name: "Task details" });
    const outputTab = card.getByRole("tab", { name: "Output", exact: true });
    await outputTab.click();
    await expect(card.locator(".detail-editor")).toHaveAttribute(
      "data-language",
      "python",
    );
    await expect(card.locator(".view-lines")).toContainText("def greeting_0");
    const copy = card.getByRole("button", { name: "Copy", exact: true });
    await copy.click();
    await expect
      .poll(() => page.evaluate(() => navigator.clipboard.readText()))
      .toBe(output);
    await expect(card).toBeVisible();
    const pane = page.locator(".transcript");
    const top = await pane.evaluate((el) => el.scrollTop);
    const editor = card.locator(".monaco-editor");
    await expect(editor.locator("textarea")).toHaveJSProperty("readOnly", true);
    await editor.hover();
    await page.mouse.wheel(0, 160);
    const readingPosition = () =>
      editor
        .locator(".scrollbar.vertical > .slider")
        .first()
        .evaluate((el) => parseFloat(el.style.top));
    await expect.poll(readingPosition).toBeGreaterThan(0);
    expect(
      await dialog.locator(".card-body").evaluate((el) => el.scrollTop),
    ).toBe(0);
    expect(await pane.evaluate((el) => el.scrollTop)).toBe(top);
    await card.getByRole("tab", { name: "Input", exact: true }).click();
    await expect(card.locator(".detail-editor")).toHaveAttribute(
      "data-language",
      "json",
    );
    await copy.click();
    await expect
      .poll(() => page.evaluate(() => navigator.clipboard.readText()))
      .toBe(
        JSON.stringify(
          {
            item: {
              type: "commandExecution",
              command: "cat greetings.py",
              status: "inProgress",
            },
          },
          null,
          2,
        ),
      );
    await outputTab.click();
    await expect.poll(readingPosition).toBeGreaterThan(0);
    expect(requests).toBe(1);
    expect(errors).toEqual([]);
  });
});
