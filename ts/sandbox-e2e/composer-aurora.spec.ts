import { expect, test, devices } from "@playwright/test";

test.use({
  ...devices["Desktop Chrome"],
  viewport: { width: 1440, height: 1000 },
});

test("background glow mounts before the composer safely and follows layout without page overflow", async ({
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
          const style = getComputedStyle(node);
          const width = parseFloat(
            style.getPropertyValue("--aurora-anchor-width"),
          );
          if (!(width > 0)) return Infinity;
          const bar = document
            .querySelector(".composer-toolbar")!
            .getBoundingClientRect();
          const clusters = node
            .querySelector(".aurora-clusters")!
            .getBoundingClientRect();
          const offset = parseFloat(
            style.getPropertyValue("--aurora-anchor-offset"),
          );
          return Math.max(
            Math.abs(clusters.left - bar.left),
            Math.abs(clusters.width - bar.width),
            Math.abs(clusters.top + offset - bar.top),
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
  // Rotated wide ellipses can extend far below the viewport; only their bounded
  // background paint layer should contain them, without hiding the page scroll.
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
