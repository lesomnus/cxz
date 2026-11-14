import { expect, test } from "@playwright/test";

test("hover account usage opens to the right without stealing editor focus and reuses cached reads", async ({
  page,
}) => {
  await page.goto("/iframe.html?id=accounts-sidebar--default&viewMode=story");
  const trigger = page.getByRole("link", { name: "Accounts", exact: true });
  await expect(trigger).toBeVisible({ timeout: 20000 });
  await expect(page.getByLabel("Account preview reads")).toHaveText("0");
  const editor = page.getByRole("textbox", { name: "Conversation input" });
  await editor.fill("Draft stays focused");
  await trigger.hover();
  const menu = page.getByRole("dialog", { name: "Account usage" });
  await expect(menu).toBeVisible();
  await expect(editor).toBeFocused();
  await expect(menu.locator(".agent-brand-codex")).toBeVisible();
  await expect(menu.locator(".agent-brand-claude")).toBeVisible();
  await expect(menu.getByLabel("64% remaining")).toBeVisible();
  await expect(menu.getByLabel("Remaining usage unavailable")).toBeVisible();
  const buttonBox = (await trigger.boundingBox())!;
  const box = (await menu.boundingBox())!;
  expect(box.x).toBeGreaterThan(buttonBox.x + buttonBox.width);
  const sidebar = page.getByRole("navigation", { name: "Resources" });
  const sidebarBox = (await sidebar.boundingBox())!;
  const border = await sidebar.evaluate((node) =>
    parseFloat(getComputedStyle(node).borderRightWidth),
  );
  expect(box.x - sidebarBox.x - sidebarBox.width).toBeCloseTo(
    sidebarBox.x + sidebarBox.width - border - buttonBox.x - buttonBox.width,
    0,
  );
  await expect(menu.getByRole("link")).toHaveCount(0);
  await expect(menu.getByRole("button")).toHaveCount(0);
  expect(box.width).toBe(240);
  await expect(menu).toHaveCSS("border-top-width", "0px");
  await menu.getByRole("listitem").filter({ hasText: "Work" }).click();
  await expect(menu).toBeVisible();
  await page.mouse.move(box.x + 20, box.y + 15, { steps: 12 });
  await expect(menu).toBeVisible();
  await expect(trigger).toHaveAttribute("aria-expanded", "true");
  const count = await page.getByLabel("Account preview reads").textContent();
  await page.keyboard.press("Escape");
  await expect(menu).toHaveCount(0);
  await expect(trigger).toBeFocused();
  await page.mouse.move(500, 350);
  await trigger.hover();
  await expect(menu.getByLabel("64% remaining")).toBeVisible();
  await expect(page.getByLabel("Account preview reads")).toHaveText(count!);
  await page.mouse.move(500, 350);
  await expect(menu).toHaveCount(0);
  await trigger.click();
  await expect(trigger).toHaveAttribute("aria-expanded", "false");
});

test("reset countdown follows remaining usage, updates without reads and preserves row geometry", async ({
  page,
}) => {
  await page.clock.install({ time: new Date("2026-11-13T12:00:00Z") });
  await page.goto("/iframe.html?id=accounts-sidebar--default&viewMode=story");
  await page.getByRole("link", { name: "Accounts", exact: true }).hover();
  const dialog = page.getByRole("dialog", { name: "Account usage" });
  const row = dialog.getByRole("listitem").first();
  const usage = row.getByLabel("64% remaining");
  const reset = row.getByRole("timer", { name: "Time until usage reset" });
  await expect(usage).toBeVisible();
  await expect(reset).toHaveText("00d02h25m");
  const usageBox = (await usage.boundingBox())!;
  const resetBox = (await reset.boundingBox())!;
  expect(resetBox.x).toBeGreaterThanOrEqual(usageBox.x + usageBox.width);
  const before = (await dialog.boundingBox())!;
  const reads = await page.getByLabel("Account preview reads").textContent();
  await page.clock.setSystemTime(new Date("2026-11-13T12:01:00Z"));
  await page.clock.runFor(1000);
  await expect(reset).toHaveText("00d02h24m");
  expect((await dialog.boundingBox())!.height).toBe(before.height);
  expect((await reset.boundingBox())!.width).toBe(resetBox.width);
  await expect(page.getByLabel("Account preview reads")).toHaveText(reads!);
  const unknown = dialog.getByRole("listitem").nth(1);
  await expect(unknown.getByLabel("Usage reset unavailable")).toHaveText(
    "--d--h--m",
  );
  expect((await unknown.getByRole("timer").boundingBox())!.width).toBe(
    resetBox.width,
  );
});

test("each provider shows the reset of its own displayed usage window", async ({
  page,
}) => {
  await page.clock.setFixedTime(new Date("2026-11-13T12:00:00Z"));
  await page.goto(
    "/iframe.html?id=accounts-sidebar--usage-reveal&viewMode=story",
  );
  await page.getByRole("link", { name: "Accounts", exact: true }).hover();
  const rows = page
    .getByRole("dialog", { name: "Account usage" })
    .getByRole("listitem");
  await expect(rows.nth(0).getByLabel("88% remaining")).toBeVisible();
  await expect(rows.nth(0).getByRole("timer")).toHaveText("00d02h25m");
  await expect(rows.nth(1).getByLabel("28% remaining")).toBeVisible();
  await expect(rows.nth(1).getByRole("timer")).toHaveText("00d02h00m");
});

test("keyboard users can open and dismiss the account preview, and unknown quota is not zero", async ({
  page,
}) => {
  await page.goto(
    "/iframe.html?id=accounts-sidebar--usage-unavailable&viewMode=story",
  );
  const trigger = page.getByRole("link", { name: "Accounts", exact: true });
  await expect(trigger).toBeVisible({ timeout: 20000 });
  await trigger.focus();
  const menu = page.getByRole("dialog", { name: "Account usage" });
  await expect(menu.getByLabel("Remaining usage unavailable")).toHaveCount(2);
  await expect(trigger).toBeFocused();
  await page.keyboard.press("Escape");
  await expect(menu).toHaveCount(0);
  await expect(trigger).toBeFocused();
});

test("an account-free connection shows an empty state", async ({ page }) => {
  await page.goto("/iframe.html?id=accounts-sidebar--empty&viewMode=story");
  const trigger = page.getByRole("link", { name: "Accounts", exact: true });
  await expect(trigger).toBeVisible({ timeout: 20000 });
  await trigger.hover();
  await expect(
    page.getByRole("dialog", { name: "Account usage" }),
  ).toContainText("No accounts");
});

test("usage lights start in row order, accelerate and take longer for longer fills", async ({
  page,
}, testInfo) => {
  await page.goto(
    "/iframe.html?id=accounts-sidebar--usage-reveal&viewMode=story",
  );
  await page.getByRole("link", { name: "Accounts", exact: true }).hover();
  const dialog = page.getByRole("dialog", { name: "Account usage" });
  const rows = dialog.getByRole("listitem");
  await expect(rows.nth(1).getByLabel("28% remaining")).toBeVisible();
  // A faster lower response must not overtake the still-loading first row.
  await expect(
    rows.nth(0).getByLabel("Remaining usage unavailable"),
  ).toHaveAttribute("aria-busy", "true");
  await expect(rows.nth(1)).toHaveAttribute("data-usage-visible", "false");
  await expect(rows.nth(0).getByLabel("88% remaining")).toBeVisible();
  await expect(rows.nth(1)).toHaveAttribute("data-usage-visible", "true");
  const motion = await rows.evaluateAll((elements) =>
    elements.map((element) => {
      const animation = element
        .getAnimations({ subtree: true })
        .find(
          (animation) =>
            animation instanceof CSSTransition &&
            animation.transitionProperty === "transform" &&
            (animation.effect as KeyframeEffect).target === element,
        )!;
      const duration = Number(animation.effect!.getTiming().duration);
      const start = Number(animation.startTime);
      animation.pause();
      const scales = [0.2, 0.4, 0.6, 0.8].map((progress) => {
        animation.currentTime = duration * progress;
        return new DOMMatrixReadOnly(
          getComputedStyle(element, "::before").transform,
        ).a;
      });
      return { duration, start, scales };
    }),
  );
  expect(motion[1].start).toBeGreaterThan(motion[0].start);
  expect(motion[0].duration).toBeGreaterThan(motion[1].duration);
  for (const { scales } of motion) {
    expect(scales[3] - scales[2]).toBeGreaterThan(scales[1] - scales[0]);
    expect(scales[0]).toBeGreaterThan(0);
    expect(scales[3]).toBeLessThan(1);
  }
  await dialog.screenshot({ path: testInfo.outputPath("usage-reveal.png") });
});

test("reduced motion shows the final usage fill without animating", async ({
  page,
}) => {
  await page.emulateMedia({ reducedMotion: "reduce" });
  await page.goto("/iframe.html?id=accounts-sidebar--default&viewMode=story");
  await page.getByRole("link", { name: "Accounts", exact: true }).hover();
  const row = page
    .getByRole("dialog", { name: "Account usage" })
    .getByRole("listitem")
    .first();
  await expect(row).toHaveAttribute("data-usage-visible", "true");
  const result = await row.evaluate((element) => ({
    scale: new DOMMatrixReadOnly(
      getComputedStyle(element, "::before").transform,
    ).a,
    animated: element
      .getAnimations({ subtree: true })
      .some(
        (animation) =>
          animation instanceof CSSTransition &&
          animation.transitionProperty === "transform" &&
          (animation.effect as KeyframeEffect).target === element,
      ),
  }));
  expect(result.scale).toBeCloseTo(0.64);
  expect(result.animated).toBe(false);
});

async function fillFrames(row: import("@playwright/test").Locator) {
  return row.evaluate((element) => {
    const animation = element
      .getAnimations({ subtree: true })
      .find(
        (animation) =>
          animation instanceof CSSTransition &&
          animation.transitionProperty === "transform" &&
          (animation.effect as KeyframeEffect).target === element,
      );
    return (animation?.effect as KeyframeEffect | undefined)
      ?.getKeyframes()
      .map((frame) => new DOMMatrixReadOnly(frame.transform as string).a);
  });
}

test("reopening cached usage shows the complete fill and number immediately", async ({
  page,
}) => {
  await page.goto("/iframe.html?id=accounts-sidebar--default&viewMode=story");
  const trigger = page.getByRole("link", { name: "Accounts", exact: true });
  await trigger.hover();
  const row = page
    .getByRole("dialog", { name: "Account usage" })
    .getByRole("listitem")
    .first();
  await expect(row).toHaveAttribute("data-usage-animate", "true");
  await page.keyboard.press("Escape");
  await page.mouse.move(500, 350);
  await trigger.hover();
  await expect(row.getByLabel("64% remaining")).toBeVisible();
  await expect(row).toHaveAttribute("data-usage-animate", "false");
  expect(await fillFrames(row)).toBeUndefined();
  expect(
    await row.evaluate(
      (element) =>
        new DOMMatrixReadOnly(getComputedStyle(element, "::before").transform)
          .a,
    ),
  ).toBeCloseTo(0.64);
  await expect(row.locator("[data-changing]")).toHaveCount(0);
});

test("fresh quota transitions the cached fill and digits, and retargets from an in-flight value", async ({
  page,
}) => {
  await page.goto(
    "/iframe.html?id=accounts-sidebar--cached-updates&viewMode=story",
  );
  const trigger = page.getByRole("link", { name: "Accounts", exact: true });
  await trigger.hover();
  const row = page
    .getByRole("dialog", { name: "Account usage" })
    .getByRole("listitem")
    .first();
  await expect(row.getByLabel("64% remaining")).toBeVisible();
  await page.keyboard.press("Escape");
  await page.getByRole("button", { name: "Update to 40%" }).click();
  await trigger.hover();
  await expect(row.getByLabel("64% remaining")).toBeVisible();
  await expect(row).toHaveAttribute("data-usage-animate", "false");
  await expect(row.getByLabel("40% remaining")).toBeVisible();
  const first = (await fillFrames(row))!;
  expect(first[0]).toBeCloseTo(0.64);
  expect(first[1]).toBeCloseTo(0.4);
  await expect(row.locator("[data-changing]").first()).toBeVisible();
  await page
    .getByRole("button", { name: "Update to 80%" })
    .dispatchEvent("click");
  await expect(row.getByLabel("80% remaining")).toBeVisible();
  const next = (await fillFrames(row))!;
  expect(next[0]).toBeGreaterThan(0.4);
  expect(next[0]).toBeLessThanOrEqual(0.64);
  expect(next[1]).toBeCloseTo(0.8);
});

test("a recent reveal is suppressed even without quota cache, and resumes after the cooldown", async ({
  page,
}) => {
  await page.goto(
    "/iframe.html?id=accounts-sidebar--cached-updates&viewMode=story",
  );
  const trigger = page.getByRole("link", { name: "Accounts", exact: true });
  const row = page
    .getByRole("dialog", { name: "Account usage" })
    .getByRole("listitem")
    .first();
  await trigger.hover();
  await expect(row).toHaveAttribute("data-usage-animate", "true");
  await page.keyboard.press("Escape");
  await page.getByRole("button", { name: "Clear usage cache" }).click();
  await trigger.hover();
  await expect(row.getByLabel("64% remaining")).toBeVisible();
  await expect(row).toHaveAttribute("data-usage-animate", "false");
  expect(await fillFrames(row)).toBeUndefined();
  await page.keyboard.press("Escape");
  await page.clock.setFixedTime(
    new Date((await page.evaluate(() => Date.now())) + 61_000),
  );
  await page.getByRole("button", { name: "Clear usage cache" }).click();
  await trigger.hover();
  await expect(row).toHaveAttribute("data-usage-animate", "true");
  const frames = (await fillFrames(row))!;
  expect(frames[0]).toBe(0);
  expect(frames[1]).toBeCloseTo(0.64);
});

for (const deviceScaleFactor of [1, 1.5]) {
  test(`loading, digit slides and settled usage keep the same dialog height at ${deviceScaleFactor}x`, async ({
    browser,
    baseURL,
  }) => {
    const context = await browser.newContext({
      baseURL,
      deviceScaleFactor,
      viewport: { width: 1100, height: 800 },
    });
    try {
      const page = await context.newPage();
      await page.goto(
        "/iframe.html?id=accounts-sidebar--usage-reveal&viewMode=story",
      );
      await page.getByRole("link", { name: "Accounts", exact: true }).hover();
      const dialog = page.getByRole("dialog", { name: "Account usage" });
      await expect(dialog.getByRole("listitem")).toHaveCount(2);
      // A dependency's CSS may be injected later by a route load or HMR.
      const numberClass = await dialog
        .locator('[data-mode="integer"]')
        .first()
        .evaluate((node) => node.classList[0]);
      await page.addStyleTag({
        content: `.${numberClass} { display: inline-block; }`,
      });
      const samples = await dialog.evaluate(
        (element) =>
          new Promise<{ height: number; rows: number[]; phase: string }[]>(
            (resolve) => {
              const samples: {
                height: number;
                rows: number[];
                phase: string;
              }[] = [];
              const started = performance.now();
              const sample = () => {
                const rows = [
                  ...element.querySelectorAll<HTMLElement>('[role="listitem"]'),
                ];
                samples.push({
                  height: element.getBoundingClientRect().height,
                  rows: rows.map((row) => row.getBoundingClientRect().height),
                  phase: rows.map((row) => row.dataset.usageVisible).join(),
                });
                if (performance.now() - started < 2600)
                  requestAnimationFrame(sample);
                else resolve(samples);
              };
              sample();
            },
          ),
      );
      expect(samples.some((sample) => sample.phase.includes("false"))).toBe(
        true,
      );
      expect(samples.at(-1)!.phase).toBe("true,true");
      expect(
        Math.max(...samples.map((sample) => sample.height)) -
          Math.min(...samples.map((sample) => sample.height)),
      ).toBeLessThan(0.05);
      for (let index = 0; index < 2; index++) {
        expect(
          Math.max(...samples.map((sample) => sample.rows[index])) -
            Math.min(...samples.map((sample) => sample.rows[index])),
        ).toBeLessThan(0.05);
      }
    } finally {
      await context.close();
    }
  });
}

test("the initial account-loading placeholder occupies the same height as one account", async ({
  page,
}) => {
  await page.goto(
    "/iframe.html?id=accounts-sidebar--loading-account&viewMode=story",
  );
  await page.getByRole("link", { name: "Accounts", exact: true }).hover();
  const dialog = page.getByRole("dialog", { name: "Account usage" });
  await expect(dialog.getByRole("status")).toHaveText("Loading accounts…");
  const initial = (await dialog.boundingBox())!.height;
  await expect(dialog.getByRole("listitem")).toHaveCount(1);
  expect((await dialog.boundingBox())!.height).toBeCloseTo(initial, 2);
  await expect(dialog.getByLabel("64% remaining")).toBeVisible();
  expect((await dialog.boundingBox())!.height).toBeCloseTo(initial, 2);
});
