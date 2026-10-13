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
