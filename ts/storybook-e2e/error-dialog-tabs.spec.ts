import { expect, test } from "@playwright/test";
const story = "/iframe.html?id=workspace-errors--overview&viewMode=story";

for (const width of [1200, 390])
  test(`dialog title inset and dismiss padding/corners match their frames at ${width}px`, async ({
    page,
  }) => {
    await page.setViewportSize({ width, height: 850 });
    await page.goto(story);
    await page.getByRole("button", { name: "Errors", exact: true }).click();
    const dialog = page.getByRole("dialog", { name: "Errors", exact: true });
    await page.waitForTimeout(400);
    const header = dialog.locator(".card-heading");
    await expect(header.locator("strong")).toHaveCSS("margin-left", "4px");
    const row = dialog.getByRole("row").first();
    await row.hover();
    const button = row.getByRole("button", { name: "Dismiss", exact: true });
    const overlay = button.locator("..");
    await expect(overlay).toHaveCSS("padding-top", "2px");
    await expect(overlay).toHaveCSS("padding-right", "2px");
    await expect(overlay).toHaveCSS("padding-bottom", "2px");
    await expect(button).toHaveCSS("border-top-right-radius", "8px");
    const [r, b] = await Promise.all([row.boundingBox(), button.boundingBox()]);
    expect(b!.y - r!.y).toBeCloseTo(2, 0);
    expect(r!.y + r!.height - b!.y - b!.height).toBeCloseTo(2, 0);
    expect(r!.x + r!.width - b!.x - b!.width).toBeCloseTo(2, 0);
  });

test("Active/History indicator and body use the same duration/easing and both directions slide", async ({
  page,
}) => {
  await page.goto(story);
  await page.getByRole("button", { name: "Errors", exact: true }).click();
  const dialog = page.getByRole("dialog", { name: "Errors", exact: true });
  const indicator = dialog.locator(".segment-indicator");
  const active = dialog.locator('[data-sliding-page="active"]');
  const history = dialog.locator('[data-sliding-page="history"]');
  await expect(active).toHaveAttribute("data-active", "true");
  await expect(history).toHaveAttribute("inert");
  const timing = await indicator.evaluate((node) => ({
    duration: getComputedStyle(node).transitionDuration,
    easing: getComputedStyle(node).transitionTimingFunction,
  }));
  expect(timing).toEqual({ duration: "0.14s", easing: "ease-out" });
  for (const body of [active, history]) {
    const css = await body.evaluate((node) => ({
      duration: getComputedStyle(node).transitionDuration.split(",")[0].trim(),
      easing: getComputedStyle(node)
        .transitionTimingFunction.split(",")[0]
        .trim(),
    }));
    expect(css).toEqual(timing);
  }
  await dialog.evaluate((element) => {
    element.dataset.tabMotions = "";
    element.addEventListener("transitionrun", (event) => {
      if (
        !(event instanceof TransitionEvent) ||
        event.propertyName !== "transform" ||
        !(event.target instanceof HTMLElement)
      )
        return;
      if (event.target.matches(".segment-indicator"))
        element.dataset.tabMotions += " indicator";
      if (event.target.matches("[data-sliding-page]"))
        element.dataset.tabMotions += " body";
    });
  });
  await dialog.getByRole("radio", { name: "History 3", exact: true }).click();
  await expect(history).toHaveAttribute("data-active", "true");
  await expect(active).toHaveAttribute("inert");
  await expect(dialog).toHaveAttribute("data-tab-motions", /indicator/);
  await expect(dialog).toHaveAttribute("data-tab-motions", /body/);
  await expect(
    dialog.getByRole("table", { name: "Errors", exact: true }).getByRole("row"),
  ).toHaveCount(3);
  await page.waitForTimeout(200);
  await dialog.evaluate((element) => {
    element.dataset.tabMotions = "";
  });
  await dialog.getByRole("radio", { name: "Active 6", exact: true }).click();
  await expect(active).toHaveAttribute("data-active", "true");
  await expect(history).toHaveAttribute("inert");
  await expect(dialog).toHaveAttribute("data-tab-motions", /indicator/);
  await expect(dialog).toHaveAttribute("data-tab-motions", /body/);
});

test("body/tab transitions respect reduced motion", async ({ page }) => {
  await page.emulateMedia({ reducedMotion: "reduce" });
  await page.goto(story);
  await page.getByRole("button", { name: "Errors", exact: true }).click();
  const dialog = page.getByRole("dialog", { name: "Errors", exact: true });
  await dialog.getByRole("radio", { name: "History 3", exact: true }).click();
  await expect(dialog.locator(".segment-indicator")).toHaveCSS(
    "transition-duration",
    "0s",
  );
  for (const tab of ["active", "history"])
    await expect(dialog.locator(`[data-sliding-page="${tab}"]`)).toHaveCSS(
      "transition-duration",
      "0s",
    );
});
