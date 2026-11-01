import { expect, test } from "@playwright/test";

test("an outward pull from a loaded history edge fetches and keeps advancing", async ({
  page,
}) => {
  await page.goto(
    "/test-support/transcript-performance.html?probe=history-edge",
  );
  const thumb = page.getByRole("scrollbar", {
    name: "Conversation scroll",
    exact: true,
  });
  await expect(thumb).toBeVisible();
  await expect(page.getByLabel("History loads")).toHaveText("0");
  const grab = (await thumb.boundingBox())!;
  await page.mouse.move(grab.x + grab.width / 2, grab.y + grab.height / 2);
  await page.mouse.down();
  await page.mouse.move(
    grab.x + grab.width / 2,
    grab.y + grab.height / 2 - 40,
    { steps: 8 },
  );
  await expect
    .poll(() => thumb.getAttribute("data-stretch").then(Number))
    .toBeGreaterThan(0);
  await expect(page.getByLabel("History loads")).toHaveText("1");
  await expect(page.locator(".virtual-messages")).toHaveAttribute(
    "data-first",
    "980",
  );
  await expect
    .poll(() =>
      page.locator(".transcript").evaluate((el) => {
        const top = el.getBoundingClientRect().top;
        const row = [...el.querySelectorAll<HTMLElement>("[data-row]")].find(
          (row) => row.getBoundingClientRect().bottom > top,
        )!;
        return Number(row.dataset.row);
      }),
    )
    .toBeLessThan(1000);
  await page.mouse.up();
});

for (const boundary of ["latest", "oldest"] as const) {
  test(`outward regrab at ${boundary} builds handle and tick tension from zero`, async ({
    page,
  }) => {
    await page.goto("/test-support/transcript-performance.html?count=8192");
    const pane = page.locator(".transcript");
    const thumb = page.getByRole("scrollbar", {
      name: "Conversation scroll",
      exact: true,
    });
    await expect(thumb).toBeVisible();
    if (boundary === "oldest") await thumb.press("Home");
    else
      await expect
        .poll(() =>
          pane.evaluate(
            (el) => el.scrollHeight - el.clientHeight - el.scrollTop,
          ),
        )
        .toBeLessThan(2);
    await page.waitForTimeout(500);
    const first = (await thumb.boundingBox())!;
    await page.mouse.move(
      first.x + first.width / 2,
      first.y + first.height / 2,
    );
    await page.mouse.down();
    await page.mouse.up();
    await page.waitForTimeout(200);
    const grab = (await thumb.boundingBox())!;
    const x = grab.x + grab.width / 2,
      y = grab.y + grab.height / 2;
    const direction = boundary === "latest" ? 1 : -1;
    await page.mouse.move(x, y);
    await page.mouse.down();
    await expect(thumb).toHaveAttribute("data-stretch", "0.00");
    const before = await page.evaluate(() => {
      const rail = document.querySelector<HTMLElement>(".scroll-track")!;
      const marker = [
        ...rail.querySelectorAll<HTMLElement>(".scroll-tick"),
      ].find(
        (el) =>
          parseFloat(el.style.top) > 0 &&
          parseFloat(el.style.top) < rail.clientHeight,
      )!;
      return {
        id: marker.dataset.event!,
        top: parseFloat(marker.style.top),
        scroll: document.querySelector(".transcript")!.scrollTop,
      };
    });
    await page.mouse.move(x, y + direction);
    await expect
      .poll(() => thumb.getAttribute("data-stretch").then(Number))
      .toBeGreaterThan(0);
    const light = Number(await thumb.getAttribute("data-stretch"));
    expect(light).toBeLessThan(1);
    await page.mouse.move(x, y + direction * 10, { steps: 5 });
    await expect
      .poll(() => thumb.getAttribute("data-stretch").then(Number))
      .toBeGreaterThan(light + 1);
    const medium = Number(await thumb.getAttribute("data-stretch"));
    await page.mouse.move(x, y + direction * 30, { steps: 5 });
    await expect
      .poll(() => thumb.getAttribute("data-stretch").then(Number))
      .toBeGreaterThan(medium + 2);
    const strength = Number(await thumb.getAttribute("data-stretch"));
    const moved = await page
      .locator(`.scroll-tick[data-event="${before.id}"]`)
      .evaluate((el) => parseFloat((el as HTMLElement).style.top));
    expect((moved - before.top) * direction).toBeCloseTo(strength, 0);
    expect(
      Math.abs((await pane.evaluate((el) => el.scrollTop)) - before.scroll),
    ).toBeLessThan(2);
    await page.mouse.up();
  });
}

for (const direction of [-1, 1]) {
  test(`local rail regrab keeps scrolling and ticking while holding ${direction < 0 ? "up" : "down"}`, async ({
    page,
  }) => {
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
      el.addEventListener("history-shift", (event) =>
        el.setAttribute(
          "data-test-shift",
          String(
            Number(el.getAttribute("data-test-shift")) +
              (event as CustomEvent<number>).detail,
          ),
        ),
      );
      el.scrollTop = (el.scrollHeight - el.clientHeight) / 2;
    });
    await page.waitForTimeout(500);
    let grab = (await thumb.boundingBox())!;
    const rail = (await page.locator(".scroll-track").boundingBox())!;
    const outside =
      direction < 0
        ? rail.y + grab.height / 2 - 40
        : rail.y + rail.height - grab.height / 2 + 40;
    await page.mouse.move(grab.x + grab.width / 2, grab.y + grab.height / 2);
    await page.mouse.down();
    await page.mouse.move(grab.x + grab.width / 2, outside, { steps: 8 });
    await expect
      .poll(() => thumb.getAttribute("data-stretch").then(Number))
      .toBeGreaterThan(0);
    await page.mouse.up();
    // Release recenters the local rail. Regrab its current painted handle,
    // rather than coordinates sampled while that transition is still moving.
    await thumb.hover();
    grab = (await thumb.boundingBox())!;
    await page.mouse.move(grab.x + grab.width / 2, grab.y + grab.height / 2);
    await page.mouse.down();
    await expect(page.locator(".scroll-dragging")).toBeAttached();
    await expect(thumb).toHaveAttribute("data-stretch", "0.00");
    await page.mouse.move(grab.x + grab.width / 2, outside, { steps: 8 });
    await expect
      .poll(() => thumb.getAttribute("data-stretch").then(Number))
      .toBeGreaterThan(0);
    await page.waitForTimeout(150);
    const samples = await page.evaluate(async () => {
      const pane = document.querySelector(".transcript")!;
      const rail = document.querySelector<HTMLElement>(".scroll-track")!;
      const marker = [
        ...rail.querySelectorAll<HTMLElement>(".scroll-tick"),
      ].sort(
        (a, b) =>
          Math.abs(parseFloat(a.style.top) - rail.clientHeight / 2) -
          Math.abs(parseFloat(b.style.top) - rail.clientHeight / 2),
      )[0]!;
      const sample = () => ({
        scroll: pane.scrollTop - Number(pane.getAttribute("data-test-shift")),
        tick: parseFloat(marker.style.top),
        connected: marker.isConnected,
      });
      const values = [sample()];
      for (let part = 0; part < 2; part++) {
        for (let i = 0; i < 12; i++) await new Promise(requestAnimationFrame);
        values.push(sample());
      }
      return values;
    });
    for (let i = 1; i < samples.length; i++) {
      expect(
        direction * (samples[i].scroll - samples[i - 1].scroll),
      ).toBeGreaterThan(20);
      expect(
        -direction * (samples[i].tick - samples[i - 1].tick),
      ).toBeGreaterThan(1);
      expect(samples[i].connected).toBe(true);
    }
    await page.mouse.up();
  });
}

for (const boundary of ["latest", "oldest"] as const) {
  test(`first drag away from ${boundary} moves immediately and does not accelerate back`, async ({
    page,
  }) => {
    await page.goto("/test-support/transcript-performance.html?count=8192");
    const pane = page.locator(".transcript");
    const thumb = page.getByRole("scrollbar", {
      name: "Conversation scroll",
      exact: true,
    });
    await expect(thumb).toBeVisible();
    if (boundary === "oldest") await thumb.press("Home");
    else
      await expect
        .poll(() =>
          pane.evaluate(
            (el) => el.scrollHeight - el.clientHeight - el.scrollTop,
          ),
        )
        .toBeLessThan(2);
    await page.waitForTimeout(500);
    const grab = (await thumb.boundingBox())!;
    const rail = (await page.locator(".scroll-track").boundingBox())!;
    const direction = boundary === "latest" ? -1 : 1;
    const before = await thumb.getAttribute("aria-valuenow").then(Number);
    await page.mouse.move(grab.x + grab.width / 2, grab.y + grab.height / 2);
    await page.mouse.down();
    await page.waitForTimeout(100);
    await expect(thumb).toHaveAttribute("data-stretch", "0.00");
    // Stay in the same outer quarter: moving inward must remain ordinary drag.
    await page.mouse.move(
      grab.x + grab.width / 2,
      grab.y + grab.height / 2 + direction * (rail.height - grab.height) * 0.05,
      { steps: 4 },
    );
    await expect
      .poll(
        async () =>
          direction *
          (Number(await thumb.getAttribute("aria-valuenow")) - before),
      )
      .toBeGreaterThan(20);
    await expect(thumb).toHaveAttribute("data-stretch", "0.00");
    await page.waitForTimeout(200);
    const positions = await pane.evaluate(async (el) => {
      const values: number[] = [];
      for (let i = 0; i < 12; i++) {
        await new Promise(requestAnimationFrame);
        values.push(el.scrollTop);
      }
      return values;
    });
    expect(Math.abs(positions.at(-1)! - positions[0])).toBeLessThan(2);
    await page.mouse.up();
  });
}

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
