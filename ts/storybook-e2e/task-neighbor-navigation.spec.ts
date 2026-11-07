import { expect, test, type Page } from "@playwright/test";

const story = (name = "task-details") =>
  `/iframe.html?id=resources-tasks--${name}&viewMode=story`;
const neighbor = (page: Page, direction: "previous" | "next") =>
  page.locator(`[data-task-neighbor="${direction}"]`);

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
        };
      });
      expect(geometry.visibleHeight).toBeGreaterThan(36);
      expect(geometry.paddingTop).toBeGreaterThanOrEqual(7.5);
      expect(geometry.paddingBottom).toBeGreaterThanOrEqual(7.5);
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
