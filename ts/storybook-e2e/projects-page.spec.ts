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
    .getByRole("region", { name: "Project sessions", exact: true })
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
  await expect(page.locator("main .panel-scroll-content")).toContainText(
    "Loading projects…",
  );
  expect(errors).toEqual([]);
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
  await expect(tasks).toContainText("Review the release plan");
  await expect(
    tasks.getByRole("link", { name: "Open task board" }),
  ).toHaveAttribute("href", /project=project-1/);
  await main
    .getByRole("region", { name: "Project sessions", exact: true })
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
    .getByRole("region", { name: "Project sessions", exact: true })
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
    .getByRole("region", { name: "Project sessions", exact: true })
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
    .getByRole("region", { name: "Project sessions", exact: true })
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
    .getByRole("region", { name: "Project sessions", exact: true })
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
