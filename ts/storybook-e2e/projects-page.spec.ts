import { expect, test, type Locator } from "@playwright/test";

test("project page and shared toolbar stories render their selector and states", async ({
  page,
}) => {
  const errors: string[] = [];
  page.on("pageerror", (error) => errors.push(error.message));
  await page.goto(
    "/iframe.html?id=components-pagetoolbar--project-selector&viewMode=story",
  );
  const picker = page.getByRole("combobox", {
    name: "Select project",
    exact: true,
  });
  await expect(picker).toHaveText("Web workspace");
  await picker.click();
  await page
    .getByRole("option", { name: "API workspace", exact: true })
    .click();
  await expect(picker).toHaveText("API workspace");
  await page.goto("/iframe.html?id=pages-projects--default&viewMode=story");
  await expect(
    page.getByRole("main", { name: "Project workspace" }),
  ).toBeVisible();
  await expect(picker).toHaveText("Web workspace");
  await page
    .getByRole("region", { name: "Session list", exact: true })
    .getByRole("button")
    .first()
    .click();
  await expect(
    page.getByRole("region", { name: "Session details", exact: true }),
  ).toBeVisible();
  await expect(picker).toHaveText("Web workspace");
  await page.goto("/iframe.html?id=pages-projects--empty&viewMode=story");
  await expect(picker).toBeDisabled();
  await expect(
    page.getByText("No projects yet.", { exact: true }),
  ).toBeVisible();
  await page.goto("/iframe.html?id=pages-projects--loading&viewMode=story");
  await expect(picker).toBeDisabled();
  await expect(
    page.locator("main > .panel-scroll-region > .panel-scroll-content"),
  ).toContainText("Loading projects…");
  expect(errors).toEqual([]);
});

test("session list reserves seven and a half rows and scrolls independently", async ({
  page,
}) => {
  await page.setViewportSize({ width: 1440, height: 1000 });
  await page.goto(
    "/iframe.html?id=pages-projects--long-sessions&viewMode=story",
  );
  const list = page.getByRole("region", { name: "Session list", exact: true });
  const viewport = list.locator(".panel-scroll-content");
  const rows = list.getByRole("button");
  await expect(rows).toHaveCount(16);
  const frame = (await viewport.boundingBox())!;
  const eighth = (await rows.nth(7).boundingBox())!;
  expect(
    Math.abs(frame.y + frame.height - eighth.y - eighth.height / 2),
  ).toBeLessThan(1);
  const outer = page.locator(
    "main > .panel-scroll-region > .panel-scroll-content",
  );
  const outerTop = await outer.evaluate((el) => el.scrollTop);
  await viewport.hover();
  await page.mouse.wheel(0, 300);
  await expect
    .poll(() => viewport.evaluate((el) => el.scrollTop))
    .toBeGreaterThan(100);
  expect(await outer.evaluate((el) => el.scrollTop)).toBe(outerTop);
  const scrolled = await viewport.evaluate((el) => el.scrollTop);
  const section = page.getByRole("region", {
    name: "Project sessions",
    exact: true,
  });
  const sectionBefore = await section.boundingBox();
  const diagnostics = page.getByRole("region", {
    name: "Diagnostics",
    exact: true,
  });
  const diagnosticsBefore = await diagnostics.boundingBox();
  const listBefore = await viewport.boundingBox();
  await rows.nth(10).click();
  await expect(
    page.getByRole("region", { name: "Session details", exact: true }),
  ).toContainText("current-model");
  expect(await viewport.evaluate((el) => el.scrollTop)).toBe(scrolled);
  expect(await viewport.boundingBox()).toEqual(listBefore);
  expect(await section.boundingBox()).toEqual(sectionBefore);
  expect(await diagnostics.boundingBox()).toEqual(diagnosticsBefore);
  const detailsViewport = section
    .locator(":scope > .panel-scroll-region")
    .first()
    .locator(".panel-scroll-content");
  expect(
    await detailsViewport.evaluate((el) => el.scrollHeight > el.clientHeight),
  ).toBe(true);
  await rows.nth(11).click();
  expect(await viewport.boundingBox()).toEqual(listBefore);
  expect(await section.boundingBox()).toEqual(sectionBefore);
  await rows.nth(10).click();
  await expect(rows.nth(10)).toHaveAttribute("aria-pressed", "true");
  for (const story of ["default", "empty-sessions"]) {
    await page.goto(`/iframe.html?id=pages-projects--${story}&viewMode=story`);
    await expect(list).toBeVisible();
    expect((await viewport.boundingBox())!.height).toBe(frame.height);
  }
  await expect(list).toContainText("No sessions");
});

test("dashboard scopes project data and shows the selected session's runtime snapshots", async ({
  page,
}) => {
  await page.setViewportSize({ width: 1440, height: 1000 });
  await page.goto("/iframe.html?id=pages-projects--default&viewMode=story");
  const main = page.getByRole("main", { name: "Project workspace" });
  const environment = main.getByRole("region", {
    name: "Environment",
    exact: true,
  });
  await expect(environment).toContainText("ui/dashboard");
  await expect(environment).toContainText("/host/workspace");
  await expect(environment).toContainText("/workspace");
  await expect(
    main.getByRole("heading", { name: "Web workspace", exact: true }),
  ).toHaveCount(0);
  const overview = main.getByRole("region", { name: "Session overview" });
  expect(
    (await overview.boundingBox())!.x - (await main.boundingBox())!.x,
  ).toBeLessThan(24);
  const tasks = main.getByRole("region", { name: "Project tasks" });
  const sessions = main.getByRole("region", {
    name: "Project sessions",
    exact: true,
  });
  const toolbar = (await main.locator(":scope > header").boundingBox())!;
  const sessionsBox = (await sessions.boundingBox())!;
  const sectionGap = await sessions
    .locator("..")
    .evaluate((el) => parseFloat(getComputedStyle(el).rowGap));
  expect(sessionsBox.y).toBe(toolbar.y + toolbar.height + sectionGap);
  expect((await environment.boundingBox())!.x).toBe(
    (await tasks.boundingBox())!.x,
  );
  expect((await overview.boundingBox())!.x).toBe(
    (await tasks.boundingBox())!.x,
  );
  expect(
    (await environment.boundingBox())!.x +
      (await environment.boundingBox())!.width,
  ).toBeLessThan(sessionsBox.x);
  await expect(tasks).toContainText("Review the release plan");
  await expect(
    tasks.getByRole("link", { name: "Open task board" }),
  ).toHaveAttribute("href", /project=project-1/);
  await main
    .getByRole("region", { name: "Session list", exact: true })
    .getByRole("button")
    .first()
    .click();
  const detail = main.getByRole("region", {
    name: "Session details",
    exact: true,
  });
  await expect(
    detail
      .locator("dl > div")
      .filter({ has: page.locator("dt", { hasText: /^Current model$/ }) })
      .locator("dd"),
  ).toHaveText("current-model");
  await expect(detail).toContainText("creation-model");
  await expect(detail).toContainText("high");
  await expect(detail).toContainText("75%");
  await expect(detail).toContainText("24K / 200K");
  await expect(detail).toContainText("Which release should we prepare?");
  await expect(detail).toContainText(
    "Provider rate limit reached; retrying shortly.",
  );
  await expect(
    detail.getByRole("link", { name: "Open conversation" }),
  ).toHaveAttribute("href", /sessions\/web-0/);
  const diagnostics = main.getByRole("region", {
    name: "Diagnostics",
    exact: true,
  });
  await expect(diagnostics).toBeVisible();
  const detailBox = (await sessions
    .locator(":scope > .panel-scroll-region")
    .first()
    .boundingBox())!;
  const listBox = (await main
    .getByRole("region", { name: "Session list", exact: true })
    .boundingBox())!;
  expect(detailBox.x).toBeGreaterThanOrEqual(sessionsBox.x);
  expect(detailBox.y + detailBox.height).toBeLessThan(listBox.y);
  expect((await diagnostics.boundingBox())!.x).toBe(
    (await environment.boundingBox())!.x,
  );
  await expect(diagnostics.locator("details, summary")).toHaveCount(0);
  await expect(diagnostics.getByText("provide", { exact: true })).toBeVisible();
  await main
    .getByRole("combobox", { name: "Select project", exact: true })
    .click();
  await page
    .getByRole("option", { name: "API workspace", exact: true })
    .click();
  await expect(environment).toContainText("api-main");
  await expect(detail).toHaveCount(0);
  await expect(tasks).toContainText("No outstanding tasks");
  await expect(tasks).not.toContainText("Review the release plan");
});

test("dashboard handles stopped and unavailable runtimes without hiding inventory", async ({
  page,
}) => {
  await page.goto("/iframe.html?id=pages-projects--stopped&viewMode=story");
  const main = page.getByRole("main", { name: "Project workspace" });
  await expect(
    main.getByRole("region", { name: "Environment", exact: true }),
  ).toContainText("Project is not running");
  await expect(
    main.getByRole("region", { name: "Session overview" }),
  ).toContainText("Stopped2");
  await page.goto("/iframe.html?id=pages-projects--unavailable&viewMode=story");
  await expect(
    main.getByRole("region", { name: "Environment", exact: true }),
  ).toContainText("Workspace is unavailable");
  await main
    .getByRole("region", { name: "Session list", exact: true })
    .getByRole("button")
    .first()
    .click();
  const details = main.getByRole("region", {
    name: "Session details",
    exact: true,
  });
  await expect(details.getByRole("alert")).toContainText(
    "Session runtime is unavailable",
  );
  await expect(details).toContainText("Work account");
  await expect(
    details
      .locator("dl > div")
      .filter({ has: page.locator("dt", { hasText: /^Current model$/ }) })
      .locator("dd"),
  ).toHaveText("—");
});

test("dashboard columns collapse on narrow screens and retain the fixed toolbar", async ({
  page,
}) => {
  await page.setViewportSize({ width: 390, height: 600 });
  await page.goto("/iframe.html?id=pages-projects--default&viewMode=story");
  const main = page.getByRole("main", { name: "Project workspace" });
  await expect(
    main.getByRole("combobox", { name: "Select project", exact: true }),
  ).toBeVisible();
  const toolbar = main.locator(":scope > header");
  const before = await toolbar.boundingBox();
  await main
    .getByRole("region", { name: "Session list", exact: true })
    .getByRole("button")
    .first()
    .click();
  await expect(
    main.getByRole("region", { name: "Session details", exact: true }),
  ).toBeVisible();
  expect(await page.evaluate(() => document.documentElement.scrollWidth)).toBe(
    390,
  );
  expect(await toolbar.boundingBox()).toEqual(before);
});

test("live inventory revisions do not starve slow dashboard reads or clear their last snapshot", async ({
  page,
}) => {
  await page.goto(
    "/iframe.html?id=pages-projects--slow-activity&viewMode=story",
  );
  await page
    .getByRole("region", { name: "Session list", exact: true })
    .getByRole("button")
    .first()
    .click();
  const detail = page.getByRole("region", {
    name: "Session details",
    exact: true,
  });
  await expect(detail.getByRole("status")).toHaveText(
    "Loading recent activity…",
  );
  await expect(detail.getByText("current-model", { exact: true })).toBeVisible({
    timeout: 8000,
  });
  await expect(detail.getByText("Work account", { exact: true })).toBeVisible();
  // The next throttled read is slow too; keep the previous snapshot visible.
  await expect(detail.getByRole("status")).toHaveText(
    "Loading recent activity…",
    { timeout: 5000 },
  );
  await expect(
    detail.getByText("current-model", { exact: true }),
  ).toBeVisible();
  await expect(detail.getByText("24K / 200K", { exact: true })).toBeVisible();
});

test("diagnostics show short IDs, copy complete values, and use machine-readable dates", async ({
  page,
  context,
}) => {
  await context.grantPermissions(["clipboard-read", "clipboard-write"]);
  await page.goto("/iframe.html?id=pages-projects--default&viewMode=story");
  const main = page.getByRole("main", { name: "Project workspace" });
  const diagnostic = main.getByRole("region", {
    name: "Diagnostics",
    exact: true,
  });
  await expect(diagnostic).toBeVisible();
  await expect(main.locator("h1, details, summary")).toHaveCount(0);
  const container = diagnostic
    .locator("dl > div")
    .filter({ has: page.locator("dt", { hasText: /^Container ID$/ }) });
  await expect(container.getByText("contain", { exact: true })).toBeVisible();
  const trigger = container.locator("[tabindex]");
  await trigger.hover();
  const full = page.getByRole("tooltip", { name: "container-1", exact: true });
  await expect(full).toBeVisible();
  await expectAlignedText(trigger);
  expect(await full.evaluate((el) => el.parentElement === document.body)).toBe(
    true,
  );
  await page.keyboard.press("Escape");
  await expect(full).toHaveCount(0);
  await trigger.focus();
  await expect(full).toBeVisible();
  await trigger.press("Escape");
  await expect(full).toHaveCount(0);
  await container.getByRole("button", { name: "Copy", exact: true }).click();
  await expect
    .poll(() => page.evaluate(() => navigator.clipboard.readText()))
    .toBe("container-1");
  const created = diagnostic
    .locator("dl > div")
    .filter({ has: page.locator("dt", { hasText: /^Created$/ }) })
    .locator("time");
  await expect(created).toHaveAttribute("datetime", /^\d{4}-\d\d-\d\dT.*Z$/);
  await main
    .getByRole("region", { name: "Session list", exact: true })
    .getByRole("button")
    .first()
    .click();
  const detail = main.getByRole("region", {
    name: "Session details",
    exact: true,
  });
  await expect(
    detail.getByText("current-model", { exact: true }),
  ).toBeVisible();
  expect(await detail.locator("li time").count()).toBeGreaterThan(0);
  expect(
    await main
      .locator("time")
      .evaluateAll((nodes) =>
        nodes.every(
          (node) =>
            !!node.textContent?.trim() &&
            Number.isFinite(Date.parse(node.getAttribute("datetime") ?? "")),
        ),
      ),
  ).toBe(true);
  await expect(diagnostic.getByText("provide", { exact: true })).toBeVisible();
});

test("full ID hints escape clipped containers and remain inside the viewport", async ({
  page,
}) => {
  await page.setViewportSize({ width: 390, height: 600 });
  await page.goto(
    "/iframe.html?id=components-overflowhint--default&viewMode=story",
  );
  const trigger = page.locator("[tabindex='0']");
  await trigger.hover();
  const hint = page.getByRole("tooltip");
  await expect(hint).toHaveText("1a2b3c4d-5e6f-7788-9900-aabbccddeeff");
  const card = (await trigger.locator("..").boundingBox())!;
  const bounds = (await hint.boundingBox())!;
  expect(bounds.x + bounds.width).toBeGreaterThan(card.x + card.width);
  expect(bounds.x).toBeGreaterThanOrEqual(0);
  expect(bounds.x + bounds.width).toBeLessThanOrEqual(390);
  expect(bounds.y + bounds.height).toBeLessThanOrEqual(600);
  expect(await hint.evaluate((el) => el.parentElement === document.body)).toBe(
    true,
  );
  await page.mouse.move(0, 0);
  await expect(hint).toHaveCount(0);
});

async function expectAlignedText(trigger: Locator) {
  const boxes = await trigger.evaluate((element) => {
    function firstCharacters(target: Element) {
      const walker = document.createTreeWalker(target, NodeFilter.SHOW_TEXT);
      const text = walker.nextNode()!;
      const range = document.createRange();
      range.setStart(text, 0);
      range.setEnd(text, Math.min(7, text.textContent!.length));
      const rect = range.getBoundingClientRect();
      return { x: rect.x, y: rect.y, width: rect.width, height: rect.height };
    }
    const hint = document.getElementById(
      element.getAttribute("aria-describedby")!,
    )!;
    return {
      original: firstCharacters(element),
      expanded: firstCharacters(hint),
    };
  });
  for (const key of ["x", "y", "width", "height"] as const)
    expect(Math.abs(boxes.original[key] - boxes.expanded[key])).toBeLessThan(
      0.25,
    );
}

test("in-place ID overlays keep the text origin and baseline with inherited typography", async ({
  page,
}) => {
  await page.setViewportSize({ width: 390, height: 600 });
  await page.goto(
    "/iframe.html?id=components-overflowhint--in-place&viewMode=story",
  );
  const trigger = page.locator("[tabindex='0']");
  await trigger.hover();
  const hint = page.getByRole("tooltip");
  await expect(hint).toBeVisible();
  await expectAlignedText(trigger);
  await expect(hint).toHaveCSS("font-size", "18px");
  await expect(hint).toHaveCSS("line-height", "36px");
  await page.mouse.move(0, 0);
  await expect(hint).toHaveCount(0);
  await trigger.hover();
  await expect(hint).toBeVisible();
  await expectAlignedText(trigger);
  const frame = (await hint.boundingBox())!;
  expect(frame.x + frame.width).toBeLessThanOrEqual(390);
  expect(await hint.evaluate((el) => el.parentElement === document.body)).toBe(
    true,
  );
});

for (const scale of [1, 1.25, 1.5, 2]) {
  test.describe(`in-place text at device scale ${scale}`, () => {
    test.use({ deviceScaleFactor: scale });
    test("aligns the complete overlay and preserves glyph shape across paint contexts", async ({
      page,
    }, testInfo) => {
      await page.goto("/iframe.html?id=pages-projects--default&viewMode=story");
      const row = page
        .getByRole("region", { name: "Diagnostics", exact: true })
        .locator("dl > div")
        .filter({ has: page.locator("dt", { hasText: /^Container ID$/ }) });
      const trigger = row.locator("[tabindex]");
      await trigger.waitFor();
      // Equal paint colors isolate position/raster changes from background contrast.
      await page.addStyleTag({
        content: ":root { --ui-overflow-hint-background: var(--ui-shade-24); }",
      });
      await page.mouse.move(0, 0);
      await trigger.scrollIntoViewIfNeeded();
      await page.evaluate(() => document.fonts.ready);
      // Compare the text, excluding the anchor's antialiased background corners.
      const clip = await trigger.evaluate((element) => {
        const range = document.createRange();
        const text = document
          .createTreeWalker(element, NodeFilter.SHOW_TEXT)
          .nextNode()!;
        range.selectNodeContents(text);
        const { x, y, width, height } = range.getBoundingClientRect();
        return { x, y, width, height };
      });
      const before = await page.screenshot({
        clip,
        path: testInfo.outputPath("original.png"),
      });
      await trigger.hover();
      await expect(
        page.getByRole("tooltip", { name: "container-1", exact: true }),
      ).toBeVisible();
      await expectAlignedText(trigger);
      const after = await page.screenshot({
        clip,
        path: testInfo.outputPath("overlay.png"),
      });
      // The CSS origins are exact. Separate paint contexts can round the baseline
      // to adjacent physical pixels at fractional device scales; the glyph shape
      // must remain identical, with no clipping or horizontal displacement.
      const difference = await page.evaluate(
        async ([original, overlay]) => {
          async function decode(base64: string) {
            const image = new Image();
            image.src = `data:image/png;base64,${base64}`;
            await image.decode();
            const canvas = document.createElement("canvas");
            canvas.width = image.width;
            canvas.height = image.height;
            const context = canvas.getContext("2d")!;
            context.drawImage(image, 0, 0);
            return context.getImageData(0, 0, canvas.width, canvas.height);
          }
          const [a, b] = await Promise.all([decode(original), decode(overlay)]);
          if (a.width !== b.width || a.height !== b.height) return Infinity;
          let best = Infinity;
          for (const offset of [-1, 0, 1]) {
            let error = 0;
            for (let y = 1; y < a.height - 1; y++) {
              for (let x = 0; x < a.width; x++) {
                const source = (y * a.width + x) * 4;
                const target = ((y + offset) * b.width + x) * 4;
                for (let channel = 0; channel < 4; channel++)
                  error += Math.abs(
                    a.data[source + channel] - b.data[target + channel],
                  );
              }
            }
            best = Math.min(best, error);
          }
          return best;
        },
        [before.toString("base64"), after.toString("base64")],
      );
      if (difference !== 0) {
        await testInfo.attach("original", {
          body: before,
          contentType: "image/png",
        });
        await testInfo.attach("overlay", {
          body: after,
          contentType: "image/png",
        });
      }
      expect(difference).toBe(0);
    });
  });
}
