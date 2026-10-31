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

for (const content of ["plain", "mixed"]) {
  test(`${content} scroll windows retain row observers and consume delivered resize measurements`, async ({
    page,
  }) => {
    const errors: string[] = [];
    page.on("pageerror", (error) => errors.push(error.message));
    await page.goto(
      `/test-support/transcript-performance.html?count=8192&content=${content}`,
    );
    await expect(page.locator(".response-heading").first()).toBeVisible();
    if (content === "mixed")
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
