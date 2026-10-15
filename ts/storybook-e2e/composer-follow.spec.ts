import { expect, test } from "@playwright/test";

test("growing and shrinking the draft keeps the latest response visible", async ({
  page,
}) => {
  await page.goto(
    "/iframe.html?id=conversation-playground--long-conversation&viewMode=story",
  );
  const input = page.getByRole("textbox", { name: "Message", exact: true });
  await expect(input).toBeVisible();
  const pane = page.locator(".transcript");
  const gap = () =>
    pane.evaluate((el) => el.scrollHeight - el.clientHeight - el.scrollTop);
  await expect.poll(gap).toBeLessThan(2);
  for (let lines = 1; lines <= 20; lines++) {
    await input.fill(
      Array.from({ length: lines }, (_, i) => `Draft line ${i}`).join("\n"),
    );
    await page.waitForTimeout(400);
    await expect.poll(gap).toBeLessThan(2);
  }
  await input.fill("Short again");
  await expect.poll(gap).toBeLessThan(2);
});

for (const distance of [48, 500]) {
  test(`composer resize preserves ${distance === 48 ? "the visible tail" : "history position"}`, async ({
    page,
  }) => {
    await page.goto(
      "/iframe.html?id=conversation-playground--long-conversation&viewMode=story",
    );
    const input = page.getByRole("textbox", { name: "Message", exact: true });
    await expect(input).toBeVisible();
    const pane = page.locator(".transcript");
    const gap = () =>
      pane.evaluate((el) => el.scrollHeight - el.clientHeight - el.scrollTop);
    await expect.poll(gap).toBeLessThan(2);
    await pane.evaluate((el, distance) => {
      el.scrollTop = el.scrollHeight - el.clientHeight - distance;
    }, distance);
    await expect.poll(gap).toBeGreaterThan(distance - 2);
    const before = await pane.evaluate((el) => el.scrollTop);
    const height = await input.evaluate((el) => el.clientHeight);
    await input.fill(
      Array.from({ length: 20 }, (_, i) => `Draft line ${i}`).join("\n"),
    );
    await expect
      .poll(() => input.evaluate((el) => el.clientHeight))
      .toBeGreaterThan(height + 40);
    if (distance === 48) await expect.poll(gap).toBeLessThan(50);
    else
      await expect
        .poll(() => pane.evaluate((el) => el.scrollTop))
        .toBeCloseTo(before, 0);
    await input.fill("Short again");
    await expect
      .poll(() => input.evaluate((el) => el.clientHeight))
      .toBe(height);
    if (distance === 48) await expect.poll(gap).toBeLessThan(50);
    else
      await expect
        .poll(() => pane.evaluate((el) => el.scrollTop))
        .toBeCloseTo(before, 0);
  });
}
