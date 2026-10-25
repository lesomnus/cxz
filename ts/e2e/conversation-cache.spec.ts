import { Buffer } from "node:buffer";
import { expect, test } from "@playwright/test";
import { browserProject } from "../test-support/projects";

test.use({
  viewport: { width: 1440, height: 900 },
  isMobile: false,
  hasTouch: false,
});
function frames(rows: unknown[]) {
  return Buffer.concat([
    ...rows.map((row) => {
      const body = Buffer.from(JSON.stringify(row)),
        header = Buffer.alloc(5);
      header.writeUInt32BE(body.length, 1);
      return Buffer.concat([header, body]);
    }),
    Buffer.from([2, 0, 0, 0, 2, 123, 125]),
  ]);
}
function gate() {
  let release!: () => void;
  const promise = new Promise<void>((r) => {
    release = r;
  });
  return { release, promise };
}
const b64 = (s: string) => Buffer.from(s.padEnd(16, "0")).toString("base64");

test("switching views retains one native stream and returns cached offscreen responses without another transcript load", async ({
  page,
}) => {
  await page.request.post("/auth/login", {
    headers: { Origin: "https://127.0.0.1:18081" },
    data: { token: "a".repeat(32) },
  });
  const held = gate();
  const auxHeld = gate();
  let deliveredAux = 0;
  const auxStreams: string[] = [];
  const project = await browserProject(page.request);
  const sessions = ["alpha", "beta"].map((id) => ({
    id: b64(id),
    runtimeId: id,
    alias: id,
    name: `${id} conversation`,
    agent: "claude",
    listed: true,
    project,
    status: { runId: "run", state: "idle", lastSeq: "1" },
  }));
  const histories: string[] = [],
    streams: { id: string; after: string }[] = [],
    rowStreams: { id: string; after: string }[] = [];
  await page.route("**/cxz.SessionService/*", async (route) => {
    const request = route.request(),
      method = new URL(request.url()).pathname.split("/").at(-1);
    const raw = request.postDataBuffer();
    const q = raw?.length
      ? JSON.parse((raw[0] === 0 ? raw.subarray(5) : raw).toString())
      : {};
    const id = q.ref?.runtimeId ?? "alpha";
    const session = sessions.find((s) => s.runtimeId === id)!;
    if (method === "List") return route.fulfill({ json: { items: sessions } });
    if (method === "Watch")
      return route.fulfill({
        contentType: "application/connect+json",
        body: frames([
          { items: sessions.map((value) => ({ id: value.id, value })) },
        ]),
      });
    if (method === "Get") return route.fulfill({ json: session });
    if (method === "Transcript") {
      histories.push(id);
      return route.fulfill({
        json: {
          events: [
            {
              seq: "1",
              runId: "run",
              kind: "assistant",
              text: `Cached ${id} response`,
            },
          ],
          snapshotSeq: "1",
        },
      });
    }
    if (method === "History") return route.fulfill({ json: { events: [] } });
    if (method === "AuxEvents") {
      auxStreams.push(id);
      if (id !== "alpha")
        return route.fulfill({
          contentType: "application/connect+json",
          body: frames([{}]),
        });
      const summaries = [
        {
          runId: "run",
          turn: "1",
          responseSeq: "1",
          text: "Summary already received",
        },
      ];
      if (auxStreams.filter((s) => s === "alpha").length > 1) {
        await auxHeld.promise;
        summaries.push({
          runId: "run",
          turn: "3",
          responseSeq: "2",
          text: "Summary generated while beta is open",
        });
        deliveredAux++;
      }
      return route.fulfill({
        contentType: "application/connect+json",
        body: frames([{ summaries }]),
      });
    }
    // Rows is where a response arrives now. The record stream still carries
    // what the session is doing, so a live answer needs both: the record moves
    // the cursor, the row is what is read.
    if (method === "Rows") {
      rowStreams.push({ id, after: q.afterUpdated ?? "0" });
      if (id === "alpha" && q.afterUpdated === "1") {
        await held.promise;
        const answer = {
          seq: "2",
          runId: "run",
          kind: "assistant",
          text: "Response produced while beta is open",
        };
        return route.fulfill({
          contentType: "application/connect+json",
          body: frames([
            { event: answer, updated: "2" },
            // The completing record republishes the response row rather than
            // adding one of its own.
            { event: answer, updated: "3" },
          ]),
        });
      }
      return route.fulfill({
        contentType: "application/connect+json",
        body: frames([]),
      });
    }
    if (method === "Events") {
      streams.push({ id, after: q.afterSeq ?? "0" });
      if (id === "alpha" && q.afterSeq === "1") {
        await held.promise;
        session.status.lastSeq = "3";
        return route.fulfill({
          contentType: "application/connect+json",
          body: frames([
            { seq: "2", runId: "run", kind: "assistant", text: "" },
            { seq: "3", runId: "run", kind: "turn_end", text: "completed" },
          ]),
        });
      }
      return route.fulfill({
        contentType: "application/connect+json",
        body: frames([]),
      });
    }
    return route.continue();
  });
  try {
    await page.goto("/c/_/session/alpha");
    await expect(page.locator(".transcript")).toContainText(
      "Cached alpha response",
    );
    await expect.poll(() => streams.some((s) => s.id === "alpha")).toBe(true);
    await expect(page.locator(".turn-summary")).toContainText(
      "Summary already received",
    );
    await page.locator('.tree-session[href="/c/_/session/beta"]').click();
    await expect(page.locator(".transcript")).toContainText(
      "Cached beta response",
    );
    held.release();
    auxHeld.release();
    // The mocked stream ends after each frame; its next reconnect delivers an
    // offscreen summary before returning to alpha. No session view owns it.
    await expect.poll(() => deliveredAux).toBeGreaterThan(0);
    const alpha = page.locator('.tree-session[href="/c/_/session/alpha"]');
    await expect(
      alpha.getByRole("img", { name: "Unread response", exact: true }),
    ).toBeVisible();
    await page.evaluate(() => {
      (window as any).cacheLoading = [];
      (window as any).cacheSummaryMissing = [];
      new MutationObserver(() => {
        const pane = document.querySelector(".transcript");
        if (pane?.getAttribute("aria-busy") === "true")
          (window as any).cacheLoading.push(true);
        if (
          pane?.textContent?.includes("Cached alpha response") &&
          !pane.textContent.includes("Summary already received")
        )
          (window as any).cacheSummaryMissing.push(true);
      }).observe(document.body, {
        subtree: true,
        childList: true,
        attributes: true,
        attributeFilter: ["aria-busy"],
      });
    });
    await alpha.click();
    await expect(page.locator(".transcript")).toContainText(
      "Response produced while beta is open",
    );
    await expect(page.locator(".transcript")).not.toContainText(
      "Cached beta response",
    );
    await expect(page.locator(".transcript")).toHaveAttribute(
      "aria-busy",
      "false",
    );
    expect(await page.evaluate(() => (window as any).cacheLoading)).toEqual([]);
    await expect(page.locator(".transcript")).toContainText(
      "Summary generated while beta is open",
    );
    expect(
      await page.evaluate(() => (window as any).cacheSummaryMissing),
    ).toEqual([]);
    expect(histories.filter((id) => id === "alpha")).toHaveLength(1);
    expect(histories.filter((id) => id === "beta")).toHaveLength(1);
    expect(
      streams.filter((s) => s.id === "alpha" && s.after === "1"),
    ).toHaveLength(1);
    expect(
      rowStreams.filter((s) => s.id === "alpha" && s.after === "1"),
    ).toHaveLength(1);
    await expect(
      alpha.getByRole("img", { name: "Unread response", exact: true }),
    ).toHaveCount(0);
    const storage = await page.evaluate(() =>
      JSON.stringify({ ...localStorage, ...sessionStorage }),
    );
    expect(storage).not.toContain("Response produced while beta is open");
  } finally {
    held.release();
    auxHeld.release();
    await page.unrouteAll({ behavior: "ignoreErrors" });
  }
});
