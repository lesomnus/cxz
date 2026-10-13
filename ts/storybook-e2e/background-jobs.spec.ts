import { expect, test } from "@playwright/test";
test("background count follows native job lifecycle independently of the foreground turn", async ({
  page,
}) => {
  const errors: string[] = [];
  page.on("pageerror", (error) => errors.push(error.message));
  await page.goto(
    "/iframe.html?id=conversation-backgroundjobs--live-telemetry&viewMode=story",
  );
  const indicator = page.locator("[data-background-jobs]");
  await expect(indicator).toHaveText("0");
  await expect(indicator).toHaveAttribute("data-active", "false");
  await page.getByRole("button", { name: "Start job", exact: true }).click();
  await page.getByRole("button", { name: "Start job", exact: true }).click();
  await expect(indicator).toHaveText("2");
  await expect(indicator).toHaveAttribute("data-active", "true");
  await expect(
    page.getByRole("button", { name: "Stop response", exact: true }),
  ).toBeDisabled();
  await expect(indicator.locator("svg rect").nth(1)).toHaveAttribute(
    "stroke-dasharray",
    "2 6",
  );
  const send = page.getByRole("button", { name: "Send", exact: true });
  const before = (await indicator.boundingBox())!;
  const sendBox = (await send.boundingBox())!;
  expect(before.width).toBe(sendBox.width);
  expect(before.height).toBe(sendBox.height);
  const border = (await indicator.locator("svg").boundingBox())!;
  expect(border.width).toBe(before.width);
  expect(border.height).toBe(before.height);
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
  await expect(page.locator("[data-background-jobs] rect").nth(1)).toHaveCSS(
    "animation-name",
    "none",
  );
});
