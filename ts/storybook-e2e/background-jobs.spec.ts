import { expect, test } from "@playwright/test";

test("background descriptions remain plain text while commands keep their own highlighting and tooltip", async ({
  page,
}) => {
  await page.goto(
    "/iframe.html?id=conversation-backgroundjobs--descriptions-and-commands&viewMode=story",
  );
  await page.locator("[data-background-jobs]").click();
  const report = page.getByRole("menu", {
    name: "Background tasks",
    exact: true,
  });
  const jobs = report.getByRole("listitem");
  await expect(jobs).toHaveCount(5);
  await expect(jobs.nth(0)).toContainText('Check timeout handling in "bash"');
  await expect(jobs.nth(0).locator("code, .hljs-string")).toHaveCount(0);
  await expect(jobs.nth(1)).toContainText("Watch source changes");
  await expect(jobs.nth(1).locator("code")).toHaveCount(0);
  await expect(jobs.nth(1).locator("[title]")).toHaveAttribute(
    "title",
    /timeout 60s bash -c/,
  );
  await expect(jobs.nth(2).locator("code")).toContainText("npm test");
  await expect(jobs.nth(2).locator("code")).not.toContainText("timeout");
});

test("an open background report stops polling when its project disappears and resumes after startup", async ({
  page,
}) => {
  await page.goto(
    "/iframe.html?id=conversation-backgroundjobs--live-telemetry&viewMode=story",
  );
  const requests = page.locator("[data-background-requests]");
  await expect(requests).toHaveAttribute("data-background-requests", "1");
  await page.locator("[data-background-jobs]").click();
  const report = page.getByRole("menu", {
    name: "Background tasks",
    exact: true,
  });
  await expect(report).toBeVisible();
  await expect(requests).toHaveAttribute("data-background-requests", "2");
  await page.clock.install();
  await page
    .getByRole("button", { name: "Stop project", exact: true })
    .dispatchEvent("click");
  await expect(report).toContainText("The session runtime is not running.");
  await expect(
    report.getByRole("menuitem", { name: "Refresh", exact: true }),
  ).toHaveCount(0);
  await page.clock.fastForward(30_000);
  await expect(requests).toHaveAttribute("data-background-requests", "2");
  await page
    .getByRole("button", { name: "Start project", exact: true })
    .dispatchEvent("click");
  await expect(requests).toHaveAttribute("data-background-requests", "4");
  await expect(
    report.getByRole("menuitem", { name: "Refresh", exact: true }),
  ).toHaveCount(0);
  await page.clock.fastForward(10_000);
  await expect(requests).toHaveAttribute("data-background-requests", "5");
});

test("stopping a project gates bootstrap, visibility and manual refresh until it starts again", async ({
  page,
}) => {
  await page.goto(
    "/iframe.html?id=conversation-backgroundjobs--live-telemetry&viewMode=story",
  );
  const requests = page.locator("[data-background-requests]");
  const indicator = page.locator("[data-background-jobs]");
  await expect(requests).toHaveAttribute("data-background-requests", "1");
  await page.getByRole("button", { name: "Start job", exact: true }).click();
  await expect(indicator).toHaveText("1");
  await page.getByRole("button", { name: "Stop project", exact: true }).click();
  await expect(indicator).toHaveText("—");
  await page
    .getByRole("button", { name: "Refresh snapshot", exact: true })
    .click();
  await page.evaluate(() =>
    document.dispatchEvent(new Event("visibilitychange")),
  );
  await page.getByRole("button", { name: "Start job", exact: true }).click();
  await expect(indicator).toHaveText("—");
  await expect(requests).toHaveAttribute("data-background-requests", "1");
  await page
    .getByRole("button", { name: "Start project", exact: true })
    .click();
  await expect(requests).toHaveAttribute("data-background-requests", "2");
  await expect(indicator).toHaveText("0");
  await page.getByRole("button", { name: "Start job", exact: true }).click();
  await expect(indicator).toHaveText("1");
});

test("background details use the shared anchored menu and a public session UUID", async ({
  page,
}) => {
  const errors: string[] = [];
  page.on("pageerror", (error) => errors.push(error.message));
  await page.goto(
    "/iframe.html?id=conversation-backgroundjobs--job-list&viewMode=story",
  );
  const trigger = page.locator("[data-background-jobs]");
  await expect(trigger).toBeVisible();
  const before = await page.locator(".composer").boundingBox();
  await trigger.click();
  const report = page.getByRole("menu", {
    name: "Background tasks",
    exact: true,
  });
  await expect(report).toBeVisible();
  await expect(trigger).toHaveAttribute("aria-expanded", "true");
  await expect(report.locator("header")).toHaveCount(0);
  await expect(
    report.getByRole("button", { name: "Close", exact: true }),
  ).toHaveCount(0);
  await expect(
    page.getByRole("dialog", { name: "Background tasks", exact: true }),
  ).toHaveCount(0);
  await expect(report.getByRole("listitem")).toHaveCount(5);
  const jobs = report.getByRole("listitem");
  await expect(jobs.nth(0)).toContainText("npm run build");
  await expect(jobs.nth(1)).toContainText("npm run dev");
  await expect(jobs.nth(2)).toContainText("npm test");
  await expect(jobs.nth(3)).toContainText("npm run lint");
  await expect(jobs.nth(4)).toContainText("npm run docs");
  await expect(report).not.toContainText("Old test run");
  await expect(report).not.toContainText("Old build");
  const command = jobs.first().locator("code");
  await expect(command).not.toContainText("timeout");
  await expect(command).toHaveAttribute("title", /timeout.*zsh -lc/);
  await expect(command.locator(".hljs-string")).not.toHaveCount(0);
  expect(
    await command
      .locator(".hljs-string")
      .first()
      .evaluate((node) => getComputedStyle(node).color),
  ).not.toBe(await command.evaluate((node) => getComputedStyle(node).color));
  await expect(command).toHaveCSS("white-space", "nowrap");
  await expect(command).toHaveCSS("text-overflow", "ellipsis");
  expect(
    await command.evaluate((node) => node.scrollWidth > node.clientWidth),
  ).toBe(true);
  await expect(jobs.nth(2).locator("code")).toHaveCSS(
    "text-decoration-line",
    "none",
  );
  await expect(jobs.nth(2)).toHaveCSS("opacity", "0.5");
  await expect(jobs.first()).toHaveCSS("opacity", "1");
  await expect(jobs.first().locator("code")).toHaveCSS(
    "text-decoration-line",
    "none",
  );
  await expect(
    report.getByText(/^(?:working|inactive|completed|failed)$/),
  ).toHaveCount(0);
  await expect(jobs.first().getByRole("timer")).toHaveText(/\d{2}:\d{2}:\d{2}/);
  const frozen = jobs
    .nth(2)
    .getByRole("img", { name: "Background task elapsed time", exact: true });
  await expect(frozen).toHaveText("00:00:44");
  await expect(jobs.first().locator(".elapsed-time-leading")).not.toHaveCount(
    0,
  );
  expect(
    await jobs
      .first()
      .locator(".elapsed-time-leading")
      .first()
      .evaluate((node) => getComputedStyle(node).color),
  ).not.toBe(
    await jobs
      .first()
      .locator(".elapsed-time")
      .evaluate((node) => getComputedStyle(node).color),
  );
  const rowBox = (await jobs.first().boundingBox())!;
  const clockBox = (await jobs.first().getByRole("timer").boundingBox())!;
  expect(clockBox.x).toBeGreaterThan((await command.boundingBox())!.x);
  expect(rowBox.height).toBeLessThan(40);
  expect(await page.locator(".composer").boundingBox()).toEqual(before);
  const anchor = (await trigger.boundingBox())!;
  const menu = (await report.boundingBox())!;
  expect(menu.y + menu.height).toBeLessThan(anchor.y);
  expect(menu.x).toBeCloseTo(anchor.x, 1);
  expect(
    await report.evaluate((node) => getComputedStyle(node).backdropFilter),
  ).not.toBe("none");
  await expect(
    report.getByRole("menuitem", { name: "Refresh", exact: true }),
  ).toHaveCount(0);
  await expect(report.locator("code").first()).toContainText("npm run build");
  await expect(report).toBeVisible();
  await expect(page.getByRole("alert")).toHaveCount(0);
  await page.keyboard.press("Escape");
  await expect(report).toHaveCount(0);
  await expect(trigger).toBeFocused();
  await trigger.click();
  await page.getByRole("textbox", { name: "Message", exact: true }).click();
  await expect(report).toHaveCount(0);
  expect(errors).toEqual([]);
});

test("background menu height stays fixed during loading and cached reopen does not request again", async ({
  page,
}) => {
  await page.clock.install();
  await page.goto(
    "/iframe.html?id=conversation-backgroundjobs--loading-job-list&viewMode=story",
  );
  const requests = page.locator("[data-background-list-requests]");
  const trigger = page.locator("[data-background-jobs]");
  const report = page.getByRole("menu", {
    name: "Background tasks",
    exact: true,
  });
  await trigger.click();
  await expect(report.getByRole("status")).toHaveText("Loading…");
  const viewport = report.locator(".background-jobs-menu-viewport");
  const loadingHeight = await viewport.evaluate((node) => node.clientHeight);
  await expect(report.getByRole("listitem")).toHaveCount(5);
  expect(await viewport.evaluate((node) => node.clientHeight)).toBe(
    loadingHeight,
  );
  const rowHeight = await report
    .getByRole("listitem")
    .first()
    .evaluate((node) => node.getBoundingClientRect().height);
  expect(loadingHeight / rowHeight).toBeCloseTo(5.5, 1);
  await expect(requests).toHaveAttribute("data-background-list-requests", "1");
  await page.keyboard.press("Escape");
  await trigger.click();
  expect(await report.getByRole("listitem").count()).toBe(5);
  await expect(report.getByRole("status")).toHaveCount(0);
  await expect(requests).toHaveAttribute("data-background-list-requests", "1");
  await page.keyboard.press("Escape");
  await page.clock.fastForward(11_000);
  await expect(requests).toHaveAttribute("data-background-list-requests", "1");
  await trigger.click();
  expect(await report.getByRole("listitem").count()).toBe(5);
  await expect(report.getByRole("status")).toHaveCount(0);
  await expect(requests).toHaveAttribute("data-background-list-requests", "2");
  expect(await viewport.evaluate((node) => node.clientHeight)).toBe(
    loadingHeight,
  );
});

test("background menu shows five and a half rows and scrolls additional active jobs", async ({
  page,
}) => {
  await page.goto(
    "/iframe.html?id=conversation-backgroundjobs--scrolling-job-list&viewMode=story",
  );
  await page.locator("[data-background-jobs]").click();
  const report = page.getByRole("menu", {
    name: "Background tasks",
    exact: true,
  });
  const rows = report.getByRole("listitem");
  await expect(rows).toHaveCount(9);
  const viewport = report.locator(".background-jobs-menu-viewport");
  const rowHeight = await rows
    .first()
    .evaluate((node) => node.getBoundingClientRect().height);
  const height = await viewport.evaluate((node) => node.clientHeight);
  expect(height / rowHeight).toBeCloseTo(5.5, 1);
  expect(
    await viewport.evaluate((node) => node.scrollHeight > node.clientHeight),
  ).toBe(true);
  await expect(report.getByRole("separator")).toHaveCount(0);
  await expect(report.getByRole("menuitem")).toHaveCount(0);
});

test("zero background jobs still opens the menu's empty state on a narrow screen", async ({
  page,
}) => {
  await page.setViewportSize({ width: 390, height: 800 });
  await page.goto(
    "/iframe.html?id=conversation-backgroundjobs--empty-job-list&viewMode=story",
  );
  const trigger = page.locator("[data-background-jobs]");
  await expect(trigger).toHaveText("0");
  await trigger.click();
  const report = page.getByRole("menu", {
    name: "Background tasks",
    exact: true,
  });
  await expect(report).toContainText(
    "No provider-reported background tasks for this run.",
  );
  const box = (await report.boundingBox())!;
  expect(box.x).toBeGreaterThanOrEqual(0);
  expect(box.x + box.width).toBeLessThanOrEqual(390);
});

test("background count follows native job lifecycle independently of the foreground turn", async ({
  page,
}) => {
  const errors: string[] = [];
  page.on("pageerror", (error) => errors.push(error.message));
  await page.goto(
    "/iframe.html?id=conversation-backgroundjobs--live-telemetry&viewMode=story",
  );
  const indicator = page.locator("[data-background-jobs]");
  const requests = page.locator("[data-background-requests]");
  await expect(indicator).toHaveText("0");
  await expect(requests).toHaveAttribute("data-background-requests", "1");
  await expect(indicator).toHaveAttribute("data-active", "false");
  await page.getByRole("button", { name: "Start job", exact: true }).click();
  await page.getByRole("button", { name: "Start job", exact: true }).click();
  await expect(indicator).toHaveText("2");
  await expect(indicator).toHaveAttribute("data-active", "true");
  await expect(
    page.getByRole("button", { name: "Stop response", exact: true }),
  ).toBeDisabled();
  const segments = indicator.locator('[data-border-trail="segment"]');
  await expect(segments).toHaveCount(1);
  await expect(indicator.locator('[data-border-trail="sheen"]')).toHaveCount(1);
  const send = page.getByRole("button", { name: "Send", exact: true });
  const before = (await indicator.boundingBox())!;
  const sendBox = (await send.boundingBox())!;
  expect(before.width).toBe(sendBox.width);
  expect(before.height).toBe(sendBox.height);
  const border = (await indicator.locator("svg").boundingBox())!;
  expect(border.width).toBe(before.width);
  expect(border.height).toBe(before.height);
  const content = indicator.locator(".button-content");
  const contentWidth = (await content.boundingBox())!.width;
  await indicator.hover();
  await page.mouse.down();
  await expect
    .poll(async () => (await content.boundingBox())!.width)
    .toBeLessThan(contentWidth);
  expect(await indicator.locator("svg").boundingBox()).toEqual(border);
  expect(await indicator.boundingBox()).toEqual(before);
  await page.mouse.move(before.x - 10, before.y - 10);
  await page.mouse.up();
  expect(
    await indicator.evaluate((node) => getComputedStyle(node).borderRadius),
  ).toBe(await send.evaluate((node) => getComputedStyle(node).borderRadius));
  const timer = page.getByRole("timer", { name: "Response elapsed time" });
  expect(before.x).toBeGreaterThan(
    (await timer.boundingBox())!.x + (await timer.boundingBox())!.width,
  );
  for (const width of [390, 640, 1280]) {
    await page.setViewportSize({ width, height: 800 });
    const jobBox = (await indicator.boundingBox())!;
    const sendBox = (await send.boundingBox())!;
    const toolbarBox = (await page.locator(".composer-toolbar").boundingBox())!;
    expect(jobBox.width).toBe(sendBox.width);
    expect(jobBox.height).toBe(sendBox.height);
    expect(sendBox.x + sendBox.width).toBeLessThanOrEqual(
      toolbarBox.x + toolbarBox.width,
    );
    expect(
      await page.evaluate(
        () => document.documentElement.scrollWidth <= innerWidth,
      ),
    ).toBe(true);
  }
  await indicator.hover();
  await expect(
    page.getByRole("tooltip", {
      name: "2 active background jobs",
      exact: true,
    }),
  ).toBeVisible();
  await page.getByRole("button", { name: "Finish job", exact: true }).click();
  await expect(indicator).toHaveText("1");
  expect((await indicator.boundingBox())!.width).toBe(before.width);
  await page.getByRole("button", { name: "Finish job", exact: true }).click();
  await expect(indicator).toHaveText("0");
  await expect(indicator).toHaveAttribute("data-active", "false");
  // Fresh Uint8Arrays with equal UUID contents must not reload the snapshot
  // or reinitialize the job counter when this fixture rerenders.
  await expect(requests).toHaveAttribute("data-background-requests", "1");
  expect(errors).toEqual([]);
});
test("unknown status stays distinct from zero and reduced motion stops the luminous outline", async ({
  page,
}) => {
  await page.goto(
    "/iframe.html?id=conversation-backgroundjobs--unknown&viewMode=story",
  );
  await expect(page.locator("[data-background-jobs]")).toHaveText("—");
  await page.emulateMedia({ reducedMotion: "reduce" });
  await page.goto(
    "/iframe.html?id=conversation-backgroundjobs--active&viewMode=story",
  );
  await expect(
    page
      .locator('[data-background-jobs] [data-border-trail="segment"]')
      .first(),
  ).toHaveCSS("animation-name", "none");
});
test("Codex snapshots replace the count and unavailable reads recover", async ({
  page,
}) => {
  await page.goto(
    "/iframe.html?id=conversation-backgroundjobs--codex-telemetry&viewMode=story",
  );
  const indicator = page.locator("[data-background-jobs]");
  await expect(indicator).toHaveText("0");
  await page.getByRole("button", { name: "Start job", exact: true }).click();
  await page.getByRole("button", { name: "Start job", exact: true }).click();
  await expect(indicator).toHaveText("2");
  await page.getByRole("button", { name: "Simulate unavailable" }).click();
  await expect(indicator).toHaveText("—");
  await page.getByRole("button", { name: "Refresh terminals" }).click();
  await expect(indicator).toHaveText("2");
  await page.getByRole("button", { name: "Finish job", exact: true }).click();
  await page.getByRole("button", { name: "Finish job", exact: true }).click();
  await expect(indicator).toHaveText("0");
});
