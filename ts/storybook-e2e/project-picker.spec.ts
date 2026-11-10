import { expect, test } from "@playwright/test";

for (const [view, story] of [
  ["Projects", "pages-projects--project-icons"],
  ["Tasks", "resources-tasks--project-icons"],
  ["Memory", "pages-memory--project-icons"],
]) {
  test(`${view} uses one selected-project icon and taller small-icon choices`, async ({
    page,
  }) => {
    await page.goto(`/iframe.html?id=${story}&viewMode=story`);
    const trigger = page.getByRole("combobox", {
      name: "Select project",
      exact: true,
    });
    await expect(trigger).toBeVisible({ timeout: 20000 });
    await expect(trigger.locator("svg")).toHaveCount(0);
    const prefix = trigger
      .locator("..")
      .locator("..")
      .locator(":scope > span > svg");
    await expect(prefix).toHaveAttribute("data-icon", "leaf");
    const original = (await trigger.boundingBox())!;
    await trigger.click();
    const search = page.getByRole("combobox", {
      name: "Search Select project",
      exact: true,
    });
    await expect(search).toBeFocused();
    await expect(search.locator("..").locator("svg")).toHaveCount(0);
    const options = page.getByRole("listbox", {
      name: "Select project choices",
      exact: true,
    });
    const rocket = options
      .getByRole("option")
      .filter({ has: page.locator('svg[data-icon="rocket"]') })
      .first();
    await expect(rocket).toBeVisible();
    const row = (await rocket.boundingBox())!;
    const icon = (await rocket.locator("svg").boundingBox())!;
    expect(row.height).toBeGreaterThan(original.height);
    expect(icon.width).toBeLessThan(20);
    expect(icon.height).toBeCloseTo(icon.width, 1);
    const name = (await rocket.innerText()).trim();
    await rocket.click();
    await expect(options).toHaveCount(0);
    await expect(trigger).toHaveText(name);
    await expect(trigger.locator("svg")).toHaveCount(0);
    await expect(prefix).toHaveAttribute("data-icon", "rocket");
  });
}
