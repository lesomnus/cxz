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
    });
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
        const result = { outside: 0, border: 0, interior: 0 };
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
