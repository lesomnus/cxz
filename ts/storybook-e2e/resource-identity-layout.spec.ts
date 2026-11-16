import { expect, test } from "@playwright/test";

for (const width of [1440, 390]) {
  for (const resource of ["project", "account"] as const) {
    test(`${resource} identity begins its ID at the row midpoint at ${width}px`, async ({
      page,
    }) => {
      await page.setViewportSize({ width, height: 1000 });
      await page.goto(
        resource === "project"
          ? "/iframe.html?id=pages-projects--default&viewMode=story"
          : "/iframe.html?id=resources-views--accounts&viewMode=story",
      );
      const info =
        resource === "project"
          ? page.getByRole("region", { name: "Information", exact: true })
          : page
              .locator('[data-account="work"]')
              .getByRole("region", { name: "Account details", exact: true });
      const row = info.locator("dl").first();
      const fields = row.locator(":scope > div");
      await expect(fields).toHaveCount(resource === "project" ? 4 : 3);
      const labels = await fields.locator("dt").allTextContents();
      expect(labels.slice(0, 3).map((label) => label.trim())).toEqual([
        "Name",
        "Alias",
        resource === "project" ? "Project ID" : "Account ID",
      ]);
      const geometry = await row.evaluate((node) => ({
        grid: node.getBoundingClientRect().toJSON(),
        fields: [...node.children].map((field) =>
          field.getBoundingClientRect().toJSON(),
        ),
      }));
      const [name, alias, id, preview] = geometry.fields;
      expect(name.width).toBeCloseTo(alias.width, 1);
      expect(alias.x - geometry.grid.x).toBeCloseTo(geometry.grid.width / 4, 1);
      expect(id.x - geometry.grid.x).toBeCloseTo(geometry.grid.width / 2, 1);
      await expect(
        info.getByRole("button", {
          name: `Copy: ${resource === "project" ? "Project ID" : "Account ID"}`,
          exact: true,
        }),
      ).toBeVisible();
      if (resource === "project") {
        expect(preview.width).toBe(40);
        expect(preview.height).toBe(preview.width);
        expect(preview.right).toBeCloseTo(geometry.grid.right, 1);
      } else {
        expect(id.width).toBeCloseTo(geometry.grid.width / 2, 1);
        expect(id.right).toBeCloseTo(geometry.grid.right, 1);
        await expect(info.getByRole("combobox", { name: "Icon" })).toHaveCount(
          0,
        );
      }
    });
  }
}
