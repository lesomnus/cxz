import { expect, test, type Locator } from "@playwright/test";

// settled answers the scroll position once the pane has stopped changing shape:
// two consecutive reads of scrollHeight, clientHeight and scrollTop that agree.
// A transcript of estimated rows reaches its real height over several frames,
// and a baseline taken before it does is a baseline nothing will match.
async function settled(pane: Locator) {
  let previous = "";
  for (let i = 0; i < 100; i++) {
    const shape = await pane.evaluate((el) =>
      [el.scrollHeight, el.clientHeight, el.scrollTop].join("/"),
    );
    if (shape === previous) return Number(shape.split("/")[2]);
    previous = shape;
    await new Promise((resolve) => setTimeout(resolve, 50));
  }
  throw new Error("the transcript never stopped changing shape: " + previous);
}

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
    // What this test pins is that a composer resize does not move the reader,
    // which means comparing a position before it with the position after. Both
    // have to be measured against the same layout: a virtual transcript settles
    // its estimated row sizes asynchronously, and a position recorded while
    // that is still happening is a number the layout then walks away from. CI
    // caught this as a fixed 118px disagreement; see cxz#196.
    const before = await settled(pane);
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
