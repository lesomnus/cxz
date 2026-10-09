import { expect, test, devices } from "@playwright/test";

test.use({
  ...devices["Desktop Chrome"],
  viewport: { width: 1440, height: 1000 },
});

test("toolbar glow follows composer layout and contains oversized orbs without page overflow", async ({
  page,
}) => {
  const errors: string[] = [];
  page.on("pageerror", (error) => errors.push(error.message));
  const input = page.getByRole("textbox", { name: "Message", exact: true });
  const field = page.locator(".composer-aurora");
  const aligned = async () => {
    await expect(input).toBeVisible();
    await expect
      .poll(() =>
        field.evaluate((node) => {
          const bar = document
            .querySelector(".composer-toolbar")!
            .getBoundingClientRect();
          const wrapper = document
            .querySelector(".composer-wrapper")!
            .getBoundingClientRect();
          const clip = node.parentElement!.getBoundingClientRect();
          const clusters = node
            .querySelector(".aurora-clusters")!
            .getBoundingClientRect();
          return Math.max(
            Math.abs(clip.left - wrapper.left),
            Math.abs(clip.width - wrapper.width),
            Math.abs(clip.top - wrapper.top),
            Math.abs(clip.bottom - bar.bottom),
            Math.abs(clusters.left - clip.left),
            Math.abs(clusters.width - clip.width),
          );
        }),
      )
      .toBeLessThan(0.5);
  };
  const noPageOverflow = async () => {
    expect(
      await page.evaluate(() => ({
        horizontal: document.documentElement.scrollWidth - innerWidth,
        vertical: document.documentElement.scrollHeight - innerHeight,
      })),
    ).toEqual({ horizontal: 0, vertical: 0 });
  };

  await page.goto("/sandbox.html");
  await aligned();
  await expect(
    page.getByRole("heading", { name: "Current status" }),
  ).toBeVisible();
  await page.reload();
  await aligned();
  await input.fill(
    Array.from({ length: 40 }, () => "Expanded draft").join("\n"),
  );
  await aligned();
  await noPageOverflow();
  // Even oversized orbs stay inside the toolbar and its border.
  await field.evaluate((node) => {
    node.setAttribute("data-active", "true");
    for (const orbit of node.querySelectorAll<HTMLElement>(".aurora-orbit"))
      orbit.style.transform = "rotate(90deg)";
    for (const orb of node.querySelectorAll<HTMLElement>(".aurora-orb"))
      orb.style.transform = "scale(2)";
  });
  await noPageOverflow();
  await page.setViewportSize({ width: 390, height: 844 });
  await aligned();
  await noPageOverflow();
  await page.getByLabel("Scenario", { exact: true }).selectOption("session-4");
  await aligned();
  await expect(
    page.getByText("Which environment?", { exact: true }),
  ).toBeVisible();
  expect(errors).toEqual([]);
});

test("the same glow colors the toolbar and its border without painting outside it", async ({
  page,
}) => {
  await page.emulateMedia({ reducedMotion: "reduce", colorScheme: "dark" });
  await page.goto("/sandbox.html");
  await expect(
    page.getByRole("heading", { name: "Current status" }),
  ).toBeVisible({ timeout: 45000 });
  await page.evaluate(async () => {
    await document.fonts.ready;
    (document.activeElement as HTMLElement | null)?.blur();
  });
  const field = page.locator(".composer-aurora");
  const bounds = (await page
    .locator(".composer-aurora-viewport")
    .boundingBox())!;
  const clip = {
    x: Math.floor(bounds.x - 16),
    y: Math.floor(bounds.y - 16),
    width: Math.ceil(bounds.width + 32),
    height: Math.ceil(bounds.height + 32),
  };
  for (const theme of ["dark", "light"]) {
    await page.evaluate((theme) => {
      document.documentElement.dataset.theme = theme;
    }, theme);
    await field.evaluate((node) => {
      node.setAttribute("data-active", "false");
    });
    const before = await page.screenshot({
      clip,
      scale: "css",
      animations: "disabled",
    });
    await field.evaluate((node) => {
      node.setAttribute("data-active", "true");
    });
    const after = await page.screenshot({
      clip,
      scale: "css",
      animations: "disabled",
      path: test.info().outputPath(`glow-${theme}.png`),
    });
    await test
      .info()
      .attach(`glow-${theme}`, { body: after, contentType: "image/png" });
    const painted = await page.evaluate(
      async ({ before, after, area }) => {
        const read = async (encoded: string) => {
          const image = new Image();
          image.src = `data:image/png;base64,${encoded}`;
          await image.decode();
          const canvas = document.createElement("canvas");
          canvas.width = image.width;
          canvas.height = image.height;
          const context = canvas.getContext("2d")!;
          context.drawImage(image, 0, 0);
          return context.getImageData(0, 0, canvas.width, canvas.height);
        };
        const a = await read(before),
          b = await read(after);
        const result = {
          outside: 0,
          border: 0,
          interior: 0,
          green: 0,
          cyan: 0,
          violet: 0,
        };
        for (let y = 0; y < a.height; y++)
          for (let x = 0; x < a.width; x++) {
            const i = (y * a.width + x) * 4;
            const difference = Math.max(
              ...[0, 1, 2].map((channel) =>
                Math.abs(a.data[i + channel] - b.data[i + channel]),
              ),
            );
            if (difference <= 3) continue;
            // Fractional layout edges can partially cover their boundary pixel.
            const inside =
              x + 1 > area.x &&
              x < area.x + area.width &&
              y + 1 > area.y &&
              y < area.y + area.height;
            if (inside) {
              const red = b.data[i],
                green = b.data[i + 1],
                blue = b.data[i + 2];
              if (green > red + 2 && green > blue + 2) result.green++;
              else if (red > green + 2 && blue > green + 2) result.violet++;
              else if (green > red + 2 && blue > red + 2) result.cyan++;
            }
            if (!inside) result.outside++;
            else if (
              y < area.y + 1 ||
              x < area.x + 1 ||
              x >= area.x + area.width - 1
            )
              result.border++;
            else result.interior++;
          }
        return result;
      },
      {
        before: before.toString("base64"),
        after: after.toString("base64"),
        area: { ...bounds, x: bounds.x - clip.x, y: bounds.y - clip.y },
      },
    );
    for (const color of ["green", "cyan", "violet"] as const)
      expect(
        painted[color],
        `${theme}: ${color} became indistinguishable`,
      ).toBeGreaterThan(0);
    expect(painted.outside, `${theme}: glow escaped the toolbar`).toBe(0);
    expect(
      painted.border,
      `${theme}: border did not pick up the glow`,
    ).toBeGreaterThan(0);
    expect(
      painted.interior,
      `${theme}: glow was not visible through the toolbar`,
    ).toBeGreaterThan(0);
  }
});

test("a real working turn advances animations and visibly moves the colored glow", async ({
  page,
}) => {
  await page.emulateMedia({
    reducedMotion: "no-preference",
    colorScheme: "dark",
  });
  await page.goto("/sandbox.html");
  await expect(
    page.getByRole("heading", { name: "Current status" }),
  ).toBeVisible({ timeout: 45000 });
  const pace = page.getByLabel("Pace", { exact: true });
  // Keep the fake turn working long enough to separate motion from entry/exit.
  await pace.evaluate((el) =>
    (el as HTMLSelectElement).add(new Option("Long test turn", "2000")),
  );
  await pace.selectOption("2000");
  await page
    .locator(".sandbox-controls")
    .getByRole("button", { name: "Reset sandbox", exact: true })
    .click();
  await expect(
    page.getByRole("heading", { name: "Current status" }),
  ).toBeVisible({ timeout: 45000 });
  await page
    .getByRole("textbox", { name: "Message", exact: true })
    .fill("Observe the working glow");
  await page.getByRole("button", { name: "Send", exact: true }).click();
  const field = page.locator(".composer-aurora");
  await expect(field).toHaveAttribute("data-active", "true");
  await expect(field).toHaveAttribute("data-running", "true");
  await expect
    .poll(() => field.evaluate((el) => Number(getComputedStyle(el).opacity)))
    .toBeCloseTo(0.5, 2);
  const animationTime = () =>
    field.evaluate((el) => {
      const animations = el.getAnimations({ subtree: true });
      return {
        playing: animations.filter((a) => a.playState === "running").length,
        time: Math.max(...animations.map((a) => Number(a.currentTime) || 0)),
      };
    });
  const start = await animationTime();
  expect(start.playing).toBeGreaterThan(0);
  // Hide foreground clock/buttons so only the light can change the pixels.
  await page.locator(".composer-toolbar").evaluate((el) => {
    for (const child of el.children)
      (child as HTMLElement).style.visibility = "hidden";
  });
  const clip = (await page.locator(".composer-aurora-viewport").boundingBox())!;
  const before = await page.screenshot({
    clip,
    scale: "css",
    path: test.info().outputPath("glow-motion-before.png"),
  });
  await page.waitForTimeout(1800);
  const after = await page.screenshot({
    clip,
    scale: "css",
    path: test.info().outputPath("glow-motion-after.png"),
  });
  await expect(field).toHaveAttribute("data-active", "true");
  await test
    .info()
    .attach("glow-motion-before", { body: before, contentType: "image/png" });
  await test
    .info()
    .attach("glow-motion-after", { body: after, contentType: "image/png" });
  expect((await animationTime()).time).toBeGreaterThan(start.time + 1000);
  const changed = await page.evaluate(
    async ({ before, after }) => {
      const read = async (value: string) => {
        const image = new Image();
        image.src = `data:image/png;base64,${value}`;
        await image.decode();
        const canvas = document.createElement("canvas");
        canvas.width = image.width;
        canvas.height = image.height;
        const context = canvas.getContext("2d")!;
        context.drawImage(image, 0, 0);
        return context.getImageData(0, 0, canvas.width, canvas.height).data;
      };
      const a = await read(before),
        b = await read(after);
      let changed = 0;
      for (let i = 0; i < a.length; i += 4)
        if (
          Math.max(...[0, 1, 2].map((c) => Math.abs(a[i + c] - b[i + c]))) > 3
        )
          changed++;
      return changed;
    },
    { before: before.toString("base64"), after: after.toString("base64") },
  );
  expect(
    changed,
    "running animations did not visibly move the glow",
  ).toBeGreaterThan(30);
  await page.locator(".composer-toolbar").evaluate((el) => {
    for (const child of el.children)
      (child as HTMLElement).style.removeProperty("visibility");
  });
  const stop = page.getByRole("button", { name: "Stop response", exact: true });
  await stop.click();
  await stop.click();
  await expect(field).toHaveAttribute("data-active", "false");
  await expect(field).toHaveAttribute("data-running", "false", {
    timeout: 5000,
  });
  await expect
    .poll(() => field.evaluate((el) => Number(getComputedStyle(el).opacity)))
    .toBe(0);
});
