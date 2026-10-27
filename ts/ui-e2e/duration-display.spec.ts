import { expect, test } from "@playwright/test";

test("duration uses dim padding and intermediate units in nine fixed cells", async ({
  page,
}) => {
  let width: number | undefined;
  for (const story of ["days", "minutes", "zero", "unknown", "overflow"]) {
    await page.goto(
      `/iframe.html?id=components-durationdisplay--${story}&viewMode=story`,
    );
    const value = page.getByRole("img");
    await expect(value).toBeVisible();
    const text = await value.textContent();
    expect(text).toHaveLength(9);
    if (story === "minutes" || story === "zero") {
      const dayColors = await value.evaluate((el) =>
        Array.from(el.children[0].children).map(
          (child) => getComputedStyle(child).color,
        ),
      );
      expect(new Set(dayColors).size).toBe(1);
    }
    if (story === "minutes") {
      expect(text).toBe("00d00h05m");
      const colors = await value.evaluate((el) => {
        const field = el.children[2];
        return Array.from(field.children).map(
          (child) => getComputedStyle(child).color,
        );
      });
      expect(new Set(colors).size).toBe(3);
    }
    const box = (await value.boundingBox())!;
    if (width !== undefined) expect(box.width).toBeCloseTo(width, 0);
    width = box.width;
  }
});
