import { expect, test } from "@playwright/test";
import { SCROLL_WINDOW_SCREENS } from "../src/features/session/conversation/scroll-physics";
import { ROW_UNITS } from "../src/features/session/conversation/virtual-layout";
test("long conversations bound DOM rows, rail markers and React dev timing memory during repeated navigation", async ({
  page,
}) => {
  const errors: string[] = [];
  page.on("pageerror", (error) => errors.push(error.message));
  await page.goto("/test-support/transcript-performance.html");
  await expect(page.locator(".response-heading").first()).toBeVisible();
  const result = await page.evaluate(async () => {
    const pane = document.querySelector<HTMLElement>(".transcript")!;
    let maxMarkers = 0,
      maxRows = 0;
    for (let index = 0; index < 150; index++) {
      pane.scrollTop = Math.max(
        0,
        pane.scrollHeight - pane.clientHeight - 100 - index * 55,
      );
      await new Promise(requestAnimationFrame);
      maxMarkers = Math.max(
        maxMarkers,
        document.querySelectorAll(".scroll-tick").length,
      );
      maxRows = Math.max(
        maxRows,
        document.querySelectorAll(".transcript [data-row]").length,
      );
    }
    const measures = performance.getEntriesByType("measure").filter((entry) => {
      const track = (entry as PerformanceMeasure).detail?.devtools;
      return (
        track?.track === "Components ⚛" || track?.trackGroup === "Scheduler ⚛"
      );
    });
    return {
      maxMarkers,
      maxRows,
      measures: measures.length,
      canScroll: pane.scrollHeight > pane.clientHeight,
      viewport: pane.clientHeight,
    };
  });
  expect(result.canScroll).toBe(true);
  expect(result.maxRows).toBeLessThan(40);
  // Current and outgoing rail windows each retain an overscan band above and
  // below. Their limit follows viewport size, independently of history length.
  const markerBudget =
    6 * Math.ceil((result.viewport * (SCROLL_WINDOW_SCREENS + 1)) / ROW_UNITS) +
    2;
  expect(result.maxMarkers).toBeLessThanOrEqual(markerBudget);
  expect(result.measures).toBeLessThanOrEqual(128);
  await page.getByRole("button", { name: "Load more", exact: true }).click();
  await expect(page.locator(".response-heading").first()).toBeVisible();
  expect(errors).toEqual([]);
});

test("reading a full loaded history avoids animation position reads across rail rebases", async ({
  page,
}) => {
  await page.goto("/test-support/transcript-performance.html?count=8192");
  await expect(page.locator(".response-heading").first()).toBeVisible();
  await page
    .getByRole("scrollbar", { name: "Conversation scroll" })
    .press("PageUp");
  const result = await page.evaluate(async () => {
    const pane = document.querySelector<HTMLElement>(".transcript")!;
    const descriptor = Object.getOwnPropertyDescriptor(
      HTMLElement.prototype,
      "offsetTop",
    )!;
    let rowPositionReads = 0,
      maxRows = 0,
      maxMarkers = 0;
    const starts = new Set<string>();
    Object.defineProperty(HTMLElement.prototype, "offsetTop", {
      ...descriptor,
      get() {
        if (this.hasAttribute("data-row")) rowPositionReads++;
        return descriptor.get!.call(this);
      },
    });
    try {
      for (let index = 0; index < 100; index++) {
        pane.scrollTop = Math.max(
          0,
          pane.scrollHeight - pane.clientHeight - 300 - index * 300,
        );
        await new Promise(requestAnimationFrame);
        maxRows = Math.max(
          maxRows,
          document.querySelectorAll(".transcript [data-row]").length,
        );
        maxMarkers = Math.max(
          maxMarkers,
          document.querySelectorAll(".scroll-tick").length,
        );
        starts.add(
          document.querySelector<HTMLElement>(".scroll-thumb")!.dataset.start!,
        );
      }
    } finally {
      Object.defineProperty(HTMLElement.prototype, "offsetTop", descriptor);
    }
    return {
      rowPositionReads,
      maxRows,
      maxMarkers,
      rebases: starts.size,
      viewport: pane.clientHeight,
    };
  });
  expect(result.rebases).toBeGreaterThan(1);
  expect(result.maxRows).toBeLessThan(40);
  expect(result.rowPositionReads).toBe(0);
  const budget =
    6 * Math.ceil((result.viewport * (SCROLL_WINDOW_SCREENS + 1)) / ROW_UNITS) +
    2;
  expect(result.maxMarkers).toBeLessThanOrEqual(budget);
});

for (const content of ["plain", "mixed", "complex"]) {
  test(`${content} scroll windows retain row observers and consume delivered resize measurements`, async ({
    page,
  }) => {
    const errors: string[] = [];
    page.on("pageerror", (error) => errors.push(error.message));
    await page.goto(
      `/test-support/transcript-performance.html?count=8192&content=${content}`,
    );
    await expect(page.locator(".response-heading").first()).toBeVisible();
    if (content !== "plain")
      await expect(page.locator(".question-history").first()).toBeAttached();
    await page
      .getByRole("scrollbar", { name: "Conversation scroll" })
      .press("PageUp");
    const result = await page.evaluate(async () => {
      const pane = document.querySelector<HTMLElement>(".transcript")!;
      const originalObserve = ResizeObserver.prototype.observe;
      const originalUnobserve = ResizeObserver.prototype.unobserve;
      const originalRect = Element.prototype.getBoundingClientRect;
      let observations = 0,
        unobservations = 0,
        rowRectReads = 0;
      const nodes = new Set<Element>();
      ResizeObserver.prototype.observe = function (node, ...options) {
        if (node.hasAttribute("data-row")) {
          observations++;
          nodes.add(node);
        }
        return originalObserve.call(this, node, ...options);
      };
      ResizeObserver.prototype.unobserve = function (node) {
        if (node.hasAttribute("data-row")) unobservations++;
        return originalUnobserve.call(this, node);
      };
      Element.prototype.getBoundingClientRect = function () {
        if (this.hasAttribute("data-row")) rowRectReads++;
        return originalRect.call(this);
      };
      try {
        for (let index = 0; index < 80; index++) {
          pane.scrollTop = Math.max(
            0,
            pane.scrollHeight - pane.clientHeight - 600 - index * 120,
          );
          await new Promise(requestAnimationFrame);
        }
        return {
          observations,
          uniqueNodes: nodes.size,
          unobservations,
          rowRectReads,
        };
      } finally {
        ResizeObserver.prototype.observe = originalObserve;
        ResizeObserver.prototype.unobserve = originalUnobserve;
        Element.prototype.getBoundingClientRect = originalRect;
      }
    });
    expect(result.uniqueNodes).toBeGreaterThan(20);
    expect(result.observations).toBe(result.uniqueNodes);
    expect(result.unobservations).toBeGreaterThan(0);
    expect(result.rowRectReads).toBe(0);
    expect(errors).toEqual([]);
  });
}

test("height corrections reuse retained row content while replacing a snapshot updates it", async ({
  page,
}) => {
  await page.goto("/test-support/transcript-performance.html?probe=rows");
  const first = page.locator('[data-rendered-row="1"]');
  await expect(first).toBeVisible();
  await page.waitForTimeout(300);
  const read = async () => {
    await page
      .getByRole("button", { name: "Read render counts", exact: true })
      .click();
    return JSON.parse(
      (await page.getByLabel("Render counts").textContent()) ?? "{}",
    );
  };
  const before = await read();
  const height = await first
    .locator("..")
    .evaluate((el) => el.getBoundingClientRect().height);
  await page
    .getByRole("button", { name: "Grow first row", exact: true })
    .click();
  await expect
    .poll(() =>
      first.locator("..").evaluate((el) => el.getBoundingClientRect().height),
    )
    .toBeGreaterThan(height + 60);
  await page.waitForTimeout(100);
  const after = await read();
  const retained = await page
    .locator("[data-rendered-row]")
    .evaluateAll((nodes) =>
      nodes.map((el) => el.getAttribute("data-rendered-row")!),
    );
  expect(retained.length).toBeGreaterThan(4);
  for (const id of retained) if (before[id]) expect(after[id]).toBe(before[id]);
  await page
    .getByRole("button", { name: "Replace first row", exact: true })
    .click();
  await expect(first).toHaveText("Updated row");
  const replaced = await read();
  expect(replaced["1"]).toBe(before["1"] + 1);
});

test("sanitized Markdown survives remounts without crossing link modes or retaining unlimited text", async ({
  page,
}) => {
  await page.goto("/test-support/transcript-performance.html?count=1");
  const result = await page.evaluate(async () => {
    // Browser-only sanitizer and the same module used by the actual cards.
    const moduleURL = "/src/shared/content/markdown-html.ts";
    const { markdownHTML } = await import(moduleURL);
    const text =
      '[Local](file:///workspace/test.md) [Web](https://example.com) <script>alert(1)</script><img src=x onerror=alert(1)><form><input></form><p style="color:red" onclick="alert(1)">Safe</p>';
    const local = markdownHTML(text, true),
      localAgain = markdownHTML(text, true),
      web = markdownHTML(text, false);
    const doc = new DOMParser().parseFromString(local.__html, "text/html");
    const webDoc = new DOMParser().parseFromString(web.__html, "text/html");
    const first = markdownHTML("eviction-probe", false);
    for (let i = 0; i < 256; i++) markdownHTML(`entry ${i}`, false);
    const entryEvicted = markdownHTML("eviction-probe", false) !== first;
    const large = "z".repeat(600000);
    const big = markdownHTML(large, false);
    return {
      reused: local === localAgain,
      separated: local !== web,
      safe: !doc.querySelector(
        "script,img,form,input,style,[style],[onclick],[onerror]",
      ),
      local: doc.querySelector("[data-container-file]")?.getAttribute("href"),
      webLocal: !!webDoc.querySelector("[data-container-file]"),
      target: doc
        .querySelector('a[href="https://example.com"]')
        ?.getAttribute("target"),
      entryEvicted,
      largeBypassed: markdownHTML(large, false) !== big,
      changed: markdownHTML(text + " **changed**", true).__html.includes(
        "<strong>changed</strong>",
      ),
    };
  });
  expect(result).toEqual({
    reused: true,
    separated: true,
    safe: true,
    local: "/workspace/test.md",
    webLocal: false,
    target: "_blank",
    entryEvicted: true,
    largeBypassed: true,
    changed: true,
  });
});

test("revisiting a virtualized Markdown card reuses its sanitized result", async ({
  page,
}) => {
  await page.addInitScript(() => performance.setResourceTimingBufferSize(4000));
  await page.goto(
    "/test-support/transcript-performance.html?count=500&content=complex",
  );
  const cards = page
    .locator(".transcript article.response:not(.question-history)")
    .filter({ has: page.locator(".markdown code") });
  await expect(cards.last()).toBeVisible();
  await page.waitForTimeout(500);
  const seq = await cards.last().getAttribute("data-seq");
  await page.evaluate(async (seq) => {
    const url = performance
      .getEntriesByType("resource")
      .map((entry) => entry.name)
      .find((url) => url.includes("/src/shared/content/markdown-parser.ts"))!;
    const { conversationMarkdown } = await import(url);
    const parse = conversationMarkdown.parse;
    Object.assign(window, { revisitedMarkdownParses: 0 });
    conversationMarkdown.parse = function (text: string, ...args: unknown[]) {
      if (text.startsWith(`### Review ${seq}\n`))
        (window as any).revisitedMarkdownParses++;
      return parse.call(this, text, ...args);
    };
  }, seq);
  const scroll = page.getByRole("scrollbar", {
    name: "Conversation scroll",
    exact: true,
  });
  await scroll.press("Home");
  await expect(
    page.locator(`.transcript article.response[data-seq="${seq}"]`),
  ).toHaveCount(0);
  await scroll.press("End");
  await expect(
    page.locator(
      `.transcript article.response[data-seq="${seq}"] .markdown code`,
    ),
  ).toBeVisible();
  expect(
    await page.evaluate(() => (window as any).revisitedMarkdownParses),
  ).toBe(0);
});
