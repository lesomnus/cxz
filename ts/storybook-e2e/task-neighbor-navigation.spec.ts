import { expect, test, type Page } from "@playwright/test";

const story = (name = "task-details") =>
  `/iframe.html?id=resources-tasks--${name}&viewMode=story`;
const neighbor = (page: Page, direction: "previous" | "next") =>
  page.locator(`[data-task-neighbor="${direction}"]`);

async function emptyPoint(page: Page, area: string) {
  return page.evaluate((area) => {
    const panel = document.querySelector(
      "[data-task-detail-panel][data-active]",
    )!;
    const bounds = (name: string) =>
      panel.querySelector(`[aria-label="${name}"]`)!.getBoundingClientRect();
    const title = bounds("Task title"),
      description = bounds("Task description");
    const info = bounds("Task information");
    let x: number, y: number;
    if (area === "title/description gap") {
      x = title.x + title.width / 2;
      y = (title.bottom + description.top) / 2;
    } else if (area === "column gap") {
      x = (title.right + info.left) / 2;
      y = info.y + 20;
    } else if (area === "below sidebar") {
      const history = bounds("Task change history");
      x = info.x + info.width / 2;
      y = (history.bottom + panel.getBoundingClientRect().bottom) / 2;
    } else {
      const buttons = panel.querySelectorAll(
        '[aria-label="Task actions"] [data-section-layout="section"]',
      );
      const a = buttons[0].getBoundingClientRect(),
        b = buttons[1].getBoundingClientRect();
      x = (a.right + b.left) / 2;
      y = a.y + a.height / 2;
    }
    const target = document.elementFromPoint(x, y);
    if (
      !target ||
      !panel.contains(target) ||
      target.closest('[data-section-layout="section"]')
    )
      throw new Error(
        `${area} did not resolve to empty space inside the detail layout`,
      );
    return { x, y };
  }, area);
}

async function settle(page: Page) {
  await expect(
    page
      .getByRole("region", { name: "Task details", exact: true })
      .getByRole("textbox", { name: "Title", exact: true }),
  ).toHaveValue("Connect the task table");
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

for (const area of [
  "title/description gap",
  "column gap",
  "below sidebar",
  "action button gap",
]) {
  test(`wheel navigation works over the ${area} inside the detail layout`, async ({
    page,
  }) => {
    await page.setViewportSize({ width: 1280, height: 900 });
    await page.goto(story());
    await settle(page);
    const point = await emptyPoint(page, area);
    await page.mouse.move(point.x, point.y);
    await page.mouse.wheel(0, 100);
    await expect(
      page
        .getByRole("region", { name: "Task details", exact: true })
        .getByRole("textbox", { name: "Title", exact: true }),
    ).toHaveValue("Review project isolation");
  });
}

test("wheel navigation from a gap still confirms unsaved edits", async ({
  page,
}) => {
  await page.setViewportSize({ width: 1280, height: 900 });
  await page.goto(story());
  await settle(page);
  const title = page
    .getByRole("region", { name: "Task details", exact: true })
    .getByRole("textbox", { name: "Title", exact: true });
  await title.fill("Preserve my draft");
  const point = await emptyPoint(page, "column gap");
  await page.mouse.move(point.x, point.y);
  await page.mouse.wheel(0, 100);
  const confirmation = page.getByRole("dialog", {
    name: "Discard unsaved changes",
    exact: true,
  });
  await expect(confirmation).toBeVisible();
  await confirmation
    .getByRole("button", { name: "Cancel", exact: true })
    .click();
  await expect(title).toHaveValue("Preserve my draft");
});

for (const width of [1280, 390]) {
  test(`neighbor cards keep vertical space and right-aligned ownership and priority at ${width}px`, async ({
    page,
  }) => {
    await page.setViewportSize({ width, height: 900 });
    await page.goto(story());
    await expect(neighbor(page, "next")).toContainText(
      "Review project isolation",
    );
    await page.evaluate(async () =>
      Promise.all(
        document
          .getAnimations()
          .filter(
            (animation) =>
              animation.effect?.getTiming().iterations !== Infinity,
          )
          .map((animation) => animation.finished.catch(() => {})),
      ),
    );
    for (const [direction, priority] of [
      ["previous", "Low"],
      ["next", "High"],
    ] as const) {
      const button = neighbor(page, direction);
      const content = button.locator(
        "[data-sliding-page][data-active] [data-task-neighbor-content]",
      );
      await expect(content).toBeVisible();
      await expect(
        content.getByLabel("Assigned to", { exact: true }),
      ).toContainText("Task API");
      await expect(
        content.getByLabel("Assigned to", { exact: true }),
      ).toContainText("oak");
      await expect(
        content.getByLabel(`${priority} priority`, { exact: true }),
      ).toHaveText(priority);
      const geometry = await content.evaluate((row) => {
        const button = row.closest("[data-task-neighbor]")!;
        const clip = button.parentElement!.getBoundingClientRect();
        const frame = row.getBoundingClientRect();
        const viewport = button
          .querySelector("[data-sliding-pages]")!
          .getBoundingClientRect();
        const buttonStyle = getComputedStyle(button);
        const rowStyle = getComputedStyle(row);
        const owner = row
          .querySelector('[aria-label="Assigned to"]')!
          .getBoundingClientRect();
        const title = row.querySelector("strong")!.getBoundingClientRect();
        const priority = row
          .querySelector('[aria-label$=" priority"]')!
          .getBoundingClientRect();
        return {
          paddingTop: frame.top - clip.top,
          paddingBottom: clip.bottom - frame.bottom,
          visibleHeight: clip.height,
          titleRight: title.right,
          ownerLeft: owner.left,
          ownerRight: owner.right,
          priorityLeft: priority.left,
          arrowGap:
            row.lastElementChild!.getBoundingClientRect().left - priority.right,
          buttonPadding: [buttonStyle.paddingTop, buttonStyle.paddingBottom],
          rowPadding: [rowStyle.paddingTop, rowStyle.paddingBottom],
          viewportTop: viewport.top - clip.top,
          viewportBottom: clip.bottom - viewport.bottom,
        };
      });
      expect(geometry.visibleHeight).toBeGreaterThan(36);
      expect(geometry.paddingTop).toBeCloseTo(0, 1);
      expect(geometry.paddingBottom).toBeCloseTo(0, 1);
      expect(geometry.viewportTop).toBeCloseTo(0, 1);
      expect(geometry.viewportBottom).toBeCloseTo(0, 1);
      expect(geometry.buttonPadding).toEqual(["0px", "0px"]);
      expect(geometry.rowPadding).toEqual(["8px", "8px"]);
      expect(geometry.arrowGap).toBeGreaterThanOrEqual(15.5);
      expect(geometry.titleRight).toBeLessThan(geometry.ownerLeft);
      expect(geometry.ownerRight).toBeLessThan(geometry.priorityLeft);
    }
    expect(
      await page.evaluate(
        () => document.documentElement.scrollWidth <= innerWidth,
      ),
    ).toBe(true);
  });
}

for (const [name, empty] of [
  ["first-task-details", ["previous"]],
  ["last-task-details", ["next"]],
  ["single-task-details", ["previous", "next"]],
] as const) {
  test(`${name} preserves empty disabled neighbor surfaces`, async ({
    page,
  }) => {
    await page.goto(story(name));
    await expect(page.locator("[data-task-neighbor]")).toHaveCount(2);
    for (const direction of empty) {
      const button = neighbor(page, direction);
      await expect(button).toBeVisible();
      await expect(button).toBeDisabled();
      await expect(button).toHaveAttribute("data-empty", "true");
      await expect(
        button.locator("[data-sliding-page][data-active]"),
      ).toHaveText("");
    }
  });
}

test("neighbor content slides with the Task in both directions using the same timing", async ({
  page,
}) => {
  await page.goto(story());
  await expect(
    page
      .getByRole("region", { name: "Task details", exact: true })
      .getByRole("textbox", { name: "Title", exact: true }),
  ).toHaveValue("Connect the task table");
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
  for (const direction of ["next", "previous"] as const) {
    const frames = await page.evaluate(async (direction) => {
      const root = document.querySelector("[data-task-deck]")!;
      const main = root.querySelector(":scope > [data-sliding-pages]")!;
      const previous = main.getAttribute("data-value");
      (
        root.querySelector(
          `[data-task-neighbor="${direction}"]`,
        ) as HTMLButtonElement
      ).click();
      for (let attempt = 0; attempt < 60; attempt++) {
        await new Promise(requestAnimationFrame);
        if (main.getAttribute("data-value") === previous) continue;
        const viewports = [
          main,
          ...root.querySelectorAll("[data-task-neighbor] [data-sliding-pages]"),
        ];
        const pages = viewports.map(
          (viewport) =>
            viewport.querySelector(
              ":scope > [data-sliding-page][data-active]",
            )!,
        );
        const motions = pages.map((page) =>
          page
            .getAnimations()
            .find(
              (animation) =>
                animation instanceof CSSTransition &&
                animation.transitionProperty === "transform",
            ),
        );
        if (motions.some((motion) => !motion)) continue;
        for (const motion of motions) {
          motion!.pause();
          motion!.currentTime =
            Number(motion!.effect!.getTiming().duration) / 2;
        }
        const frames = pages.map((page, index) => ({
          duration: motions[index]!.effect!.getTiming().duration,
          easing: motions[index]!.effect!.getTiming().easing,
          progress:
            new DOMMatrixReadOnly(getComputedStyle(page).transform).m42 /
            page.getBoundingClientRect().height,
        }));
        for (const motion of root.getAnimations({ subtree: true }))
          if (motion.effect?.getTiming().iterations !== Infinity)
            motion.finish();
        return frames;
      }
      throw new Error(
        "Task and both neighbor content transitions were not observed",
      );
    }, direction);
    expect(frames).toHaveLength(3);
    for (const frame of frames) {
      expect(frame.duration).toBe(380);
      expect(frame.easing).toBe(frames[0].easing);
      expect(frame.progress).toBeCloseTo(direction === "next" ? 0.5 : -0.5, 1);
    }
  }
});

test("reduced motion preserves neighbor navigation without sliding", async ({
  page,
}) => {
  await page.emulateMedia({ reducedMotion: "reduce" });
  await page.goto(story());
  await neighbor(page, "next").click();
  await expect(
    page
      .getByRole("region", { name: "Task details", exact: true })
      .getByRole("textbox", { name: "Title", exact: true }),
  ).toHaveValue("Review project isolation");
  for (const pageView of await page
    .locator("[data-task-deck] [data-sliding-page][data-active]")
    .all())
    await expect(pageView).toHaveCSS("transition-duration", "0s");
});
