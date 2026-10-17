import { expect, test, devices } from "@playwright/test";

test.use({
  ...devices["Desktop Chrome"],
  viewport: { width: 1440, height: 1000 },
});

test("the panel hides inactive sessions persistently and follows stop/resume updates", async ({
  page,
}) => {
  await page.addInitScript(() => {
    if (!localStorage.getItem("settings"))
      localStorage.setItem("settings", JSON.stringify({ "editor.tabSize": 3 }));
  });
  await page.goto("/sandbox.html#/sessions/session-1");
  const panel = page.getByRole("complementary", { name: "Session list" });
  const stopped = panel.locator('.tree-session[href$="/session-7"]');
  await expect(stopped).toBeVisible({ timeout: 45000 });
  await expect(panel.locator(".tree-session")).toHaveCount(8);
  const menu = page.getByRole("button", { name: "Session panel options" });
  const hide = page.getByRole("menuitemcheckbox", {
    name: "Hide stopped sessions",
    exact: true,
  });
  await menu.click();
  await expect(hide).not.toBeChecked();
  await hide.click();
  await expect(stopped).toHaveCount(0);
  await expect(panel.locator(".tree-session")).toHaveCount(7);
  await expect(
    panel.locator('.tree-session[data-state="idle"]'),
  ).not.toHaveCount(0);
  await expect(
    panel.getByRole("img", { name: "Awaiting answer", exact: true }),
  ).toBeVisible();

  await page.reload();
  await expect(panel.locator(".tree-session")).toHaveCount(7);
  await menu.click();
  await expect(hide).toBeChecked();
  await page.keyboard.press("Escape");

  // A direct link remains usable even when its row is hidden. Resume and Stop
  // must update the filtered panel through the ordinary session subscription.
  await page.goto("/sandbox.html#/sessions/session-7");
  const editor = page.getByRole("textbox", { name: "Message", exact: true });
  await expect(editor).toBeVisible();
  await expect(stopped).toHaveCount(0);
  const sessionMenu = page.getByRole("button", {
    name: "Session menu",
    exact: true,
  });
  await sessionMenu.click();
  await page.getByRole("menuitem", { name: "Resume", exact: true }).click();
  await expect(stopped).toBeVisible();
  await sessionMenu.click();
  await page.getByRole("menuitem", { name: "Stop", exact: true }).click();
  await page
    .getByRole("dialog", { name: "Stop session", exact: true })
    .getByRole("button", { name: "Stop", exact: true })
    .click();
  await expect(stopped).toHaveCount(0);
  await expect(editor).toBeVisible();
  await expect(page).toHaveURL(/#\/sessions\/session-7$/);

  await menu.click();
  await hide.click();
  await expect(stopped).toBeVisible();
  await expect(panel.locator(".tree-session")).toHaveCount(8);
  expect(
    await page.evaluate(() => JSON.parse(localStorage.getItem("settings")!)),
  ).toMatchObject({ "editor.tabSize": 3, "session.panel.hideStopped": false });
});
