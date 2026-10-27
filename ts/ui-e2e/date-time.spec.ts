import { expect, test } from "@playwright/test";

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
      const parts = time.locator('span[aria-hidden="true"] > span');
      const separators = parts.filter({ hasText: /^\D+$/ });
      const separator = separators.first();
      await expect(separator).toHaveText("-");
      await expect(separator).not.toHaveCSS("color", numericColor);
      expect(await separators.allTextContents()).toEqual([
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
