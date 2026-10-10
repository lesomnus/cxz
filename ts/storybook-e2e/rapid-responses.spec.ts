import { expect, test, type Page } from "@playwright/test";

async function ready(page: Page, id = "rapid-responses") {
  const errors: string[] = [];
  page.on("pageerror", (error) => errors.push(error.message));
  await page.goto(
    `/iframe.html?id=conversation-playground--${id}&viewMode=story`,
  );
  await expect(
    page.getByRole("textbox", { name: "Message", exact: true }),
  ).toBeVisible();
  return errors;
}

async function send(page: Page) {
  await page
    .getByRole("textbox", { name: "Message", exact: true })
    .fill("Keep following this rapid stream.");
  await page.getByRole("button", { name: "Send", exact: true }).click();
}

async function received(page: Page) {
  return page
    .locator(".storybook-burst-status")
    .evaluate((node) => Number(node.textContent?.match(/Updates (\d+)/)?.[1]));
}

async function gap(page: Page) {
  return page
    .locator(".transcript")
    .evaluate((node) => node.scrollHeight - node.scrollTop - node.clientHeight);
}

for (const id of ["rapid-responses", "rapid-responses-from-empty"]) {
  test(`${id} follows every rapid update and the final reply`, async ({
    page,
  }) => {
    const errors = await ready(page, id);
    // Observe natural playback, including native scroll/ResizeObserver frames.
    await page.evaluate(() => {
      const samples: { following: boolean; gap: number; rows: number }[] = [];
      (window as any).burstSamples = samples;
      const sample = () => {
        const status = document.querySelector(".storybook-burst-status");
        const pane = document.querySelector<HTMLElement>(".transcript");
        const received = Number(
          status?.textContent?.match(/Updates (\d+)/)?.[1],
        );
        if (pane && received > 0)
          samples.push({
            following:
              status
                ?.querySelector("[data-following]")
                ?.getAttribute("data-following") === "true",
            gap: pane.scrollHeight - pane.scrollTop - pane.clientHeight,
            rows: pane.querySelectorAll("[data-row]").length,
          });
        requestAnimationFrame(sample);
      };
      requestAnimationFrame(sample);
    });
    await send(page);
    await expect.poll(() => received(page), { timeout: 15000 }).toBe(150);
    await expect(
      page.getByRole("button", { name: "Stop response", exact: true }),
    ).toBeDisabled();
    await expect(page.locator("article.response").last()).toContainText(
      "Message received in the Storybook preview.",
    );
    await expect.poll(() => gap(page)).toBeLessThan(1);
    const samples = await page.evaluate(
      () =>
        (window as any).burstSamples as {
          following: boolean;
          gap: number;
          rows: number;
        }[],
    );
    expect(samples.length).toBeGreaterThan(30);
    expect(samples.filter((sample) => !sample.following)).toEqual([]);
    expect(Math.max(...samples.map((sample) => sample.gap))).toBeLessThan(1);
    expect(Math.max(...samples.map((sample) => sample.rows))).toBeLessThan(150);
    expect(errors).toEqual([]);
  });
}

test("rapid updates respect reading history, and Latest resumes following during the stream", async ({
  page,
}) => {
  const errors = await ready(page);
  await send(page);
  await expect.poll(() => received(page)).toBeGreaterThan(15);
  const pane = page.locator(".transcript");
  await pane.hover();
  await page.mouse.wheel(0, -800);
  await expect(
    page.locator(".storybook-burst-status [data-following]"),
  ).toHaveText("Reading history");
  const before = await received(page);
  await expect.poll(() => received(page)).toBeGreaterThan(before + 10);
  expect(await gap(page)).toBeGreaterThan(300);
  await page.getByRole("button", { name: "Latest", exact: true }).click();
  await expect(
    page.locator(".storybook-burst-status [data-following]"),
  ).toHaveText("Following latest");
  const resumed = await received(page);
  await expect.poll(() => received(page)).toBeGreaterThan(resumed + 10);
  await expect.poll(() => gap(page)).toBeLessThan(1);
  await expect.poll(() => received(page), { timeout: 15000 }).toBe(150);
  await expect(
    page.getByRole("button", { name: "Stop response", exact: true }),
  ).toBeDisabled();
  expect(errors).toEqual([]);
});

test("Stop and Reset cancel pending rapid updates", async ({ page }) => {
  const errors = await ready(page);
  await send(page);
  await expect.poll(() => received(page)).toBeGreaterThan(5);
  await page.keyboard.press("Escape");
  await page.keyboard.press("Escape");
  await expect(
    page.getByRole("button", { name: "Stop response", exact: true }),
  ).toBeDisabled();
  const stopped = await received(page);
  await page.waitForTimeout(200);
  expect(await received(page)).toBe(stopped);
  await expect(
    page
      .locator("article.response")
      .filter({ hasText: "Message received in the Storybook preview." }),
  ).toHaveCount(0);
  await send(page);
  await expect.poll(() => received(page)).toBeGreaterThan(5);
  await page.getByRole("button", { name: "Reset preview" }).click();
  await expect.poll(() => received(page)).toBe(0);
  await page.waitForTimeout(200);
  expect(await received(page)).toBe(0);
  await expect(
    page.getByRole("button", { name: "Stop response", exact: true }),
  ).toBeDisabled();
  expect(errors).toEqual([]);
});
