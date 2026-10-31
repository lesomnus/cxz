import { expect, test } from "@playwright/test";

test("outer quarters accelerate before rail ends and returning to center stops without a jump", async ({
  page,
}) => {
  const errors: string[] = [];
  page.on("pageerror", (error) => errors.push(error.message));
  await page.goto("/test-support/transcript-performance.html?count=8192");
  const pane = page.locator(".transcript");
  const thumb = page.getByRole("scrollbar", {
    name: "Conversation scroll",
    exact: true,
  });
  await expect(thumb).toBeVisible();
  await thumb.press("PageUp");
  await pane.evaluate((el) => {
    el.setAttribute("data-test-shift", "0");
    el.addEventListener("history-shift", (event) => {
      el.setAttribute(
        "data-test-shift",
        String(
          Number(el.getAttribute("data-test-shift")) +
            (event as CustomEvent<number>).detail,
        ),
      );
    });
    el.scrollTop = (el.scrollHeight - el.clientHeight) / 2;
  });
  await page.waitForTimeout(500);
  for (const direction of [-1, 1]) {
    await expect
      .poll(() =>
        thumb.evaluate((el) =>
          Math.abs(
            parseFloat((el as HTMLElement).style.top) -
              Number(el.getAttribute("data-target-top")),
          ),
        ),
      )
      .toBeLessThan(0.1);
    const grab = (await thumb.boundingBox())!;
    const rail = (await page.locator(".scroll-track").boundingBox())!;
    const travel = rail.height - grab.height;
    const x = grab.x + grab.width / 2;
    const pointer = (fraction: number) =>
      rail.y + grab.height / 2 + travel * fraction;
    await page.mouse.move(x, grab.y + grab.height / 2);
    await page.mouse.down();
    await page.mouse.move(x, pointer(direction < 0 ? 0.2 : 0.8));
    await expect
      .poll(() => thumb.getAttribute("data-stretch").then(Number))
      .toBeGreaterThan(0);
    const light = Number(await thumb.getAttribute("data-stretch"));
    await page.mouse.move(x, pointer(direction < 0 ? 0.1 : 0.9));
    await expect
      .poll(() => thumb.getAttribute("data-stretch").then(Number))
      .toBeGreaterThan(light);
    await page.waitForTimeout(150);
    const distance = await pane.evaluate(async (el) => {
      const position = () =>
        el.scrollTop - Number(el.getAttribute("data-test-shift"));
      const before = position();
      for (let frame = 0; frame < 16; frame++)
        await new Promise(requestAnimationFrame);
      return position() - before;
    });
    expect(direction * distance).toBeGreaterThan(40);
    await page.mouse.move(x, pointer(0.5));
    await expect(thumb).toHaveAttribute("data-stretch", "0.00");
    const drift = await pane.evaluate(async (el) => {
      const position = () =>
        el.scrollTop - Number(el.getAttribute("data-test-shift"));
      const before = position();
      for (let frame = 0; frame < 12; frame++)
        await new Promise(requestAnimationFrame);
      return position() - before;
    });
    expect(Math.abs(drift)).toBeLessThan(2);
    await page.mouse.up();
    await expect(thumb).toHaveAttribute("data-stretch", "0.00");
  }
  await thumb.press("Home");
  await expect.poll(() => pane.evaluate((el) => el.scrollTop)).toBe(0);
  await thumb.press("End");
  await expect
    .poll(() =>
      pane.evaluate((el) => el.scrollHeight - el.clientHeight - el.scrollTop),
    )
    .toBeLessThan(2);
  expect(errors).toEqual([]);
});
