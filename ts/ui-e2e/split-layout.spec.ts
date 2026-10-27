import { expect, test } from "@playwright/test";

test("portable split layout story resizes using the keyboard", async ({
  page,
}) => {
  await page.goto("/iframe.html?id=layout-splitlayout--nested&viewMode=story");
  const handles = page.getByRole("separator", { name: "Resize panes" });
  await expect(handles).toHaveCount(2);
  await handles.first().focus();
  await page.keyboard.press("Shift+ArrowRight");
  await expect(handles.first()).toHaveAttribute("aria-valuenow", "60");
  await handles.last().focus();
  await page.keyboard.press("ArrowUp");
  await expect(handles.last()).toHaveAttribute("aria-valuenow", "48");
});
