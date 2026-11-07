import { expect, test, type Page } from "@playwright/test";

const story =
  "/iframe.html?id=resources-tasks--slow-change-history&viewMode=story";
const current = (page: Page) =>
  page.locator("[data-task-detail-panel][data-active]");
const history = (page: Page) =>
  current(page).getByRole("table", { name: "Task history", exact: true });
const requests = (page: Page) => page.locator("[data-history-requests]");
async function settle(page: Page) {
  await page.evaluate(async () =>
    Promise.all(
      document
        .getAnimations()
        .filter(
          (animation) => animation.effect?.getTiming().iterations !== Infinity,
        )
        .map((animation) => animation.finished.catch(() => {})),
    ),
  );
}

test("outgoing history and its loaded pages survive a Task slide without refetching unchanged data", async ({
  page,
}) => {
  await page.setViewportSize({ width: 1280, height: 900 });
  await page.goto(story);
  await expect(history(page).locator("tbody tr")).toHaveCount(20);
  await expect(requests(page)).toHaveAttribute(
    "data-history-requests",
    "task-3",
  );
  await current(page)
    .getByRole("button", { name: "Load more changes", exact: true })
    .click();
  await expect(history(page).locator("tbody tr")).toHaveCount(28);
  await settle(page);
  const during = await page.evaluate(async () => {
    const deck = document.querySelector("[data-task-deck]")!;
    const old = deck.querySelector("[data-task-detail-panel][data-active]")!;
    const selector =
      '[data-section-layout="section"][aria-label="Task change history"]';
    const oldHistory = old.querySelector(selector)!;
    const height = oldHistory.getBoundingClientRect().height;
    deck.dispatchEvent(
      new WheelEvent("wheel", { deltaY: 100, bubbles: true, cancelable: true }),
    );
    for (let frame = 0; frame < 60; frame++) {
      await new Promise(requestAnimationFrame);
      if (old.hasAttribute("data-active")) continue;
      const animations = old
        .getAnimations()
        .filter(
          (animation) =>
            animation instanceof CSSTransition &&
            animation.transitionProperty === "transform",
        );
      if (!animations.length) continue;
      for (const animation of deck.getAnimations({ subtree: true })) {
        if (animation instanceof CSSTransition) {
          animation.pause();
          animation.currentTime =
            (Number(animation.effect!.getTiming().duration) +
              (animation.effect!.getTiming().delay ?? 0)) /
            2;
        }
      }
      const retained = old.querySelector(selector);
      const incoming = deck.querySelector(
        "[data-task-detail-panel][data-active]",
      )!;
      const result = {
        visible: getComputedStyle(old).visibility,
        samePanel: retained === oldHistory,
        previousHeight: height,
        retainedHeight: retained?.getBoundingClientRect().height,
        retainedRows: retained?.querySelectorAll("tbody tr").length,
        incomingPanel: !!incoming.querySelector(selector),
        incomingText: incoming.querySelector(selector)?.textContent,
      };
      for (const animation of deck.getAnimations({ subtree: true }))
        if (animation.effect?.getTiming().iterations !== Infinity)
          animation.finish();
      return result;
    }
    throw new Error("Task navigation did not start");
  });
  expect(during.visible).toBe("visible");
  expect(during.samePanel).toBe(true);
  expect(during.retainedHeight).toBeCloseTo(during.previousHeight, 1);
  expect(during.retainedRows).toBe(28);
  expect(during.incomingPanel).toBe(true);
  expect(during.incomingText).toContain("Loading history");
  await expect(history(page).locator("tbody tr")).toHaveCount(20);
  await expect(requests(page)).toHaveAttribute(
    "data-history-requests",
    "task-3,task-3,task-2",
  );
  await page
    .getByRole("button", {
      name: "Previous task: Connect the task table",
      exact: true,
    })
    .click();
  await expect(history(page).locator("tbody tr")).toHaveCount(28);
  await settle(page);
  await expect(requests(page)).toHaveAttribute(
    "data-history-requests",
    "task-3,task-3,task-2",
  );
});

test("an interrupted history request retries on return and unseen neighbors are not fetched", async ({
  page,
}) => {
  await page.goto(story);
  await expect(requests(page)).toHaveAttribute(
    "data-history-requests",
    "task-3",
  );
  const next = page.getByRole("button", {
    name: "Next task: Review project isolation",
    exact: true,
  });
  await expect(next).toBeEnabled();
  await next.evaluate((button) => (button as HTMLButtonElement).click());
  await expect(requests(page)).toHaveAttribute(
    "data-history-requests",
    "task-3,task-2",
  );
  await expect(history(page).locator("tbody tr")).toHaveCount(20);
  await page
    .getByRole("button", {
      name: "Previous task: Connect the task table",
      exact: true,
    })
    .click();
  await expect(requests(page)).toHaveAttribute(
    "data-history-requests",
    "task-3,task-2,task-3",
  );
  await expect(history(page).locator("tbody tr")).toHaveCount(20);
});

test("scrolling the history table keeps the active Task and its history panel", async ({
  page,
}) => {
  await page.setViewportSize({ width: 1280, height: 900 });
  await page.goto(story);
  await expect(history(page).locator("tbody tr")).toHaveCount(20);
  await settle(page);
  const panel = current(page).locator(
    '[data-section-layout="section"][aria-label="Task change history"]',
  );
  const viewport = panel.locator('[data-scroll-area="viewport"]');
  await viewport.hover({ position: { x: 30, y: 30 } });
  await page.mouse.wheel(0, 300);
  await expect
    .poll(() => viewport.evaluate((node) => node.scrollTop))
    .toBeGreaterThan(0);
  await expect(
    current(page).getByRole("textbox", { name: "Title", exact: true }),
  ).toHaveValue("Connect the task table");
  await expect(panel).toBeVisible();
  await expect(history(page).locator("tbody tr")).toHaveCount(20);
  await expect(requests(page)).toHaveAttribute(
    "data-history-requests",
    "task-3",
  );
});
