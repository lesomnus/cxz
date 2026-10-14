import { test, expect } from "@playwright/test";

function stream(rows) {
  const frames = rows.map((row) => {
    const body = Buffer.from(JSON.stringify(row));
    const header = Buffer.alloc(5);
    header.writeUInt32BE(body.length, 1);
    return Buffer.concat([header, body]);
  });
  return Buffer.concat([...frames, Buffer.from([2, 0, 0, 0, 2, 123, 125])]);
}

// Exercise the real development app and RPC decoder, not an already populated
// preview. Approval metadata can dwarf the visible text in a Transcript page.
test("large Transcript pages stay responsive and preserve the reading anchor", async ({
  page,
}) => {
  const payload = Buffer.from(
    JSON.stringify({
      params: { _meta: { arguments: "x".repeat(192 * 1024) } },
    }),
  ).toString("base64");
  const events = Array.from({ length: 2400 }, (_, index) => {
    const seq = index + 1;
    const kind = seq % 16 === 0 ? "approval" : seq % 2 ? "input" : "assistant";
    return {
      seq: String(seq),
      kind,
      runId: "run",
      timeMs: String(1710000000000 + seq),
      text:
        kind === "approval"
          ? "mcpServer/elicitation/request"
          : `Recorded ${kind} ${seq}`,
      ...(kind === "approval" ? { payload } : {}),
    };
  });
  const id = (name) => Buffer.from(name.padEnd(16, "0")).toString("base64");
  const project = {
    id: id("project"),
    runtimeId: "project",
    alias: "project",
    name: "Project",
    listed: true,
    status: { state: "running" },
  };
  const session = {
    id: id("fixture"),
    runtimeId: "fixture",
    alias: "fixture",
    name: "Fixture",
    agent: "codex",
    project,
    listed: true,
    status: { lastSeq: "2400", runId: "run", state: "idle" },
  };
  const requests = [];
  await page.route("**/auth/status", (route) => route.fulfill({ json: {} }));
  await page.route("**/cxz.*/*", async (route) => {
    const path = new URL(route.request().url()).pathname;
    const raw = route.request().postDataBuffer();
    const q = raw?.length
      ? JSON.parse((raw[0] === 0 ? raw.subarray(5) : raw).toString())
      : {};
    const send = (json) => route.fulfill({ json });
    const sendStream = (rows) =>
      route.fulfill({
        contentType: "application/connect+json",
        body: stream(rows),
      });
    if (path === "/cxz.ProjectService/List") return send({ items: [project] });
    if (path === "/cxz.ProjectService/Get") return send(project);
    if (path === "/cxz.ProjectService/Watch")
      return sendStream([{ items: [{ id: project.id, value: project }] }]);
    if (path === "/cxz.SessionService/List") return send({ items: [session] });
    if (path === "/cxz.SessionService/Get") return send(session);
    if (path === "/cxz.SessionService/Watch")
      return sendStream([{ items: [{ id: session.id, value: session }] }]);
    if (
      path === "/cxz.SessionService/Events" ||
      path === "/cxz.SessionService/AuxEvents"
    )
      return sendStream([]);
    if (path === "/cxz.SessionService/Transcript") {
      requests.push(q);
      if (q.beforeSeq && q.limit > 1)
        await new Promise((resolve) => setTimeout(resolve, 150));
      const eligible = events.filter(
        (e) =>
          (!q.beforeSeq || Number(e.seq) < Number(q.beforeSeq)) &&
          (!q.afterSeq || Number(e.seq) > Number(q.afterSeq)),
      );
      const rows = q.afterSeq
        ? eligible.slice(0, q.limit)
        : eligible.slice(-q.limit);
      const first = Number(rows[0]?.seq ?? 0),
        last = Number(rows.at(-1)?.seq ?? 0);
      return send({
        events: rows,
        snapshotSeq: "2400",
        hasOlder: first > 1,
        hasNewer: last < 2400,
        precedingInput: events
          .filter((e) => e.kind === "input" && Number(e.seq) < first)
          .at(-1),
      });
    }
    if (path === "/cxz.SessionService/EventDetails")
      return send({ events: events.filter((e) => e.seq === q.seq) });
    return send({});
  });
  const errors = [];
  page.on("pageerror", (error) => errors.push(error.message));
  await page.goto("/");
  await page.locator('.tree-session[href="/sessions/fixture"]').click();
  await expect(page.locator(".virtual-messages")).toBeVisible();
  await page.waitForFunction(
    () => document.querySelector(".transcript").scrollTop > 0,
  );
  // Let the initial live-arrival motion settle before measuring historical positions.
  await page.waitForTimeout(600);
  const previousFirst = await page
    .locator(".virtual-messages")
    .getAttribute("data-first");
  const result = await page.evaluate(async () => {
    const pane = document.querySelector(".transcript");
    pane.scrollTop = 0;
    for (let i = 0; i < 3; i++) await new Promise(requestAnimationFrame);
    const bounds = pane.getBoundingClientRect();
    const anchor = [...pane.querySelectorAll("[data-row]")].find(
      (row) =>
        row.getBoundingClientRect().bottom > bounds.top &&
        row.getBoundingClientRect().top < bounds.bottom,
    );
    const id = anchor.dataset.row;
    const top = anchor.getBoundingClientRect().top - bounds.top;
    const started = performance.now();
    const frames = [];
    let previous = started;
    for (let i = 0; i < 120; i++) {
      await new Promise(requestAnimationFrame);
      const now = performance.now();
      frames.push(now - previous);
      previous = now;
    }
    const retained = pane.querySelector(`[data-row="${id}"]`);
    return {
      duration: performance.now() - started,
      longestFrame: Math.max(...frames),
      retained: !!retained,
      drift: retained
        ? retained.getBoundingClientRect().top - bounds.top - top
        : null,
      rows: pane.querySelectorAll("[data-row]").length,
    };
  });
  expect(result.duration).toBeLessThan(10000);
  expect(result.longestFrame).toBeLessThan(1500);
  expect(result.retained).toBe(true);
  // Newly mounted estimates may settle by a fraction of a line, never a page.
  expect(Math.abs(result.drift)).toBeLessThan(12);
  expect(result.rows).toBeLessThan(50);
  await expect
    .poll(async () =>
      Number(
        await page.locator(".virtual-messages").getAttribute("data-first"),
      ),
    )
    .toBeLessThan(Number(previousFirst));
  expect(
    requests.filter((q) => q.beforeSeq && q.limit > 1).length,
  ).toBeGreaterThan(0);
  expect(requests.length).toBeLessThan(8);
  expect(errors).toEqual([]);
});
