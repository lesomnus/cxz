import { expect, test } from "@playwright/test";

test("shared fields use tighter spacing and label help escapes clipping with keyboard support", async ({
  page,
}) => {
  await page.goto("/iframe.html?id=pages-projects--default&viewMode=story");
  for (const name of ["Information", "Environment", "Diagnostics"]) {
    const row = page
      .getByRole("region", { name, exact: true })
      .locator("dl > div")
      .first();
    await expect(row).toBeVisible();
    const spacing = await row.evaluate((el) => ({
      gap: parseFloat(getComputedStyle(el).rowGap),
      inset: parseFloat(
        getComputedStyle(el).getPropertyValue("--ui-control-inset"),
      ),
    }));
    expect(spacing.gap).toBe(spacing.inset);
  }
  for (const label of ["Provisioning", "Project ID"]) {
    const icon = page.getByRole("img", { name: `About ${label}`, exact: true });
    await icon.hover();
    const tooltip = page.getByRole("tooltip");
    await expect(tooltip).toContainText(
      label === "Provisioning"
        ? "preparing the project container"
        : "Canonical UUID",
    );
    expect(
      await tooltip.evaluate((el) => el.parentElement === document.body),
    ).toBe(true);
    await page.mouse.move(0, 0);
    await expect(tooltip).toHaveCount(0);
    const anchor = icon.locator("..");
    await anchor.focus();
    await expect(tooltip).toBeVisible();
    await anchor.press("Escape");
    await expect(tooltip).toHaveCount(0);
  }
});
