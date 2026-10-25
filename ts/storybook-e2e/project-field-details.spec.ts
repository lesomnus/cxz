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

for (const sample of [
  {
    zone: "Asia/Kolkata",
    story: "local-offset",
    expected: "2026-10-20T08:08:19+05:30",
  },
  {
    zone: "America/New_York",
    story: "local-offset",
    expected: "2026-10-19T22:38:19-04:00",
  },
  {
    zone: "America/New_York",
    story: "winter",
    expected: "2026-01-19T21:38:19-05:00",
  },
]) {
  test.describe(`${sample.zone} ${sample.story}`, () => {
    test.use({ timezoneId: sample.zone });
    test("timestamps include the historical local offset and mute separators", async ({
      page,
    }) => {
      await page.goto(
        `/iframe.html?id=components-datetime--${sample.story}&viewMode=story`,
      );
      const time = page.locator("#storybook-root time");
      await expect(time).toHaveText(sample.expected);
      await expect(time).toHaveAttribute(
        "datetime",
        /^2026-\d\d-20T02:38:19\.000Z$/,
      );
      const numericColor = await time.evaluate(
        (el) => getComputedStyle(el).color,
      );
      const separator = time.locator("span").first();
      await expect(separator).toHaveText("-");
      await expect(separator).not.toHaveCSS("color", numericColor);
      expect(await time.locator("span").allTextContents()).toEqual([
        "-",
        "-",
        "T",
        ":",
        ":",
        sample.expected.includes("+") ? "+" : "-",
        ":",
      ]);
    });
  });
}
