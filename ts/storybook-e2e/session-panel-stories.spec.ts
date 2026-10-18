import { expect, test } from "@playwright/test";

test("project heading is muted and hover highlights the whole project", async ({
  page,
}) => {
  await page.goto("/iframe.html?id=sessions-panel--default&viewMode=story");
  const group = page.locator(".session-tree-group").first();
  const heading = group.locator(".tree-project");
  await expect(heading).toBeVisible();
  const tokens = await heading.evaluate((element) => {
    const probe = document.createElement("span");
    probe.style.color = "var(--ui-muted)";
    probe.style.fontFamily = "var(--ui-font-mono)";
    element.append(probe);
    const style = getComputedStyle(probe);
    const tokens = {
      muted: style.color,
      mono: style.fontFamily,
    };
    probe.remove();
    return tokens;
  });
  await expect(heading).toHaveCSS("font-weight", "400");
  await expect(heading).toHaveCSS("font-size", "11px");
  await expect(heading).toHaveCSS("font-family", tokens.mono);
  await expect(heading).toHaveCSS("color", tokens.muted);
  await page.mouse.move(0, 0);
  await expect(group).toHaveCSS("background-color", "rgba(0, 0, 0, 0)");
  await heading.hover();
  await expect(heading).toHaveCSS("background-color", "rgba(0, 0, 0, 0)");
  await expect(group).toHaveCSS("background-color", "rgb(32, 32, 32)");
  await group.locator(".tree-session").last().hover();
  await expect(group).toHaveCSS("background-color", "rgb(32, 32, 32)");
  await expect(group.locator(".tree-session").last()).toHaveCSS(
    "background-color",
    "rgb(40, 40, 40)",
  );
});

for (const [story, rows] of [
  ["default", 3],
  ["long-list", 24],
  ["collapsed", 0],
  ["empty", 0],
  ["loading", 0],
  ["unread-completed", 3],
  ["error-before-completion", 4],
  ["background-only", 3],
  ["background-with-unread", 3],
  ["lifecycle-states", 5],
  ["unread-beyond-scroll", 24],
] as const) {
  test(`session panel story ${story} renders`, async ({ page }) => {
    const errors: string[] = [];
    page.on("pageerror", (error) => errors.push(error.message));
    await page.goto(`/iframe.html?id=sessions-panel--${story}&viewMode=story`);
    await expect(page.locator(".tree-project")).toBeVisible();
    await expect(page.locator(".tree-session")).toHaveCount(rows);
    if (rows) {
      await page.locator(".tree-project").click();
      await expect(page.locator(".tree-session")).toHaveCount(0);
      await page.locator(".tree-project").click();
      await expect(page.locator(".tree-session")).toHaveCount(rows);
    }
    expect(errors).toEqual([]);
  });
}
