import { expect, test, devices } from "@playwright/test";
import { SANDBOX_SESSIONS } from "./fixture-shape";

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
  await page.goto("/sandbox.html#/c/_/session/session-1");
  const panel = page.getByRole("complementary", { name: "Session list" });
  const stopped = panel.locator('.tree-session[href$="/session-7"]');
  await expect(stopped).toBeVisible({ timeout: 45000 });
  await expect(panel.locator(".tree-session")).toHaveCount(SANDBOX_SESSIONS);
  const menu = page.getByRole("button", { name: "Session panel options" });
  const hide = page.getByRole("menuitem", {
    name: "Hide stopped sessions",
    exact: true,
  });
  const show = page.getByRole("menuitem", {
    name: "Show stopped sessions",
    exact: true,
  });
  await menu.click();
  await expect(hide).toBeVisible();
  await hide.click();
  await expect(stopped).toHaveCount(0);
  await expect(panel.locator(".tree-session")).toHaveCount(
    SANDBOX_SESSIONS - 1,
  );
  await expect(
    panel.locator('.tree-session[data-state="idle"]'),
  ).not.toHaveCount(0);
  await expect(
    panel.getByRole("img", { name: "Awaiting answer", exact: true }),
  ).toBeVisible();

  await page.reload();
  await expect(panel.locator(".tree-session")).toHaveCount(
    SANDBOX_SESSIONS - 1,
  );
  await menu.click();
  await expect(show).toBeVisible();
  await page.keyboard.press("Escape");

  // A direct link remains usable even when its row is hidden. Resume and Stop
  // must update the filtered panel through the ordinary session subscription.
  await page.goto("/sandbox.html#/c/_/session/session-7");
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
  await expect(page).toHaveURL(/#\/c\/_\/session\/session-7$/);

  await menu.click();
  await show.click();
  await expect(stopped).toBeVisible();
  await expect(panel.locator(".tree-session")).toHaveCount(SANDBOX_SESSIONS);
  expect(
    await page.evaluate(() => JSON.parse(localStorage.getItem("settings")!)),
  ).toMatchObject({ "editor.tabSize": 3, "session.panel.hideStopped": false });
});

test("project collapse can be disabled persistently without losing the current collapsed choices", async ({
  page,
}) => {
  await page.addInitScript(() => {
    if (!localStorage.getItem("settings"))
      localStorage.setItem("settings", JSON.stringify({ "editor.tabSize": 3 }));
  });
  await page.goto("/sandbox.html#/c/_/session/session-1");
  const panel = page.getByRole("complementary", { name: "Session list" });
  await expect(panel.locator(".tree-session")).toHaveCount(SANDBOX_SESSIONS, {
    timeout: 45000,
  });
  const group = panel.locator(".session-tree-group").first();
  const heading = group.locator(".tree-project");
  const rows = group.locator(".tree-session");
  const count = await rows.count();
  expect(count).toBeGreaterThan(0);
  const menu = page.getByRole("button", { name: "Session panel options" });
  const toggle = page.getByRole("menuitemcheckbox", {
    name: "Enable project collapse",
    exact: true,
  });

  await heading.click();
  await expect(heading).toHaveAttribute("aria-expanded", "false");
  await expect(rows).toHaveCount(0);
  await menu.click();
  await expect(toggle).toBeChecked();
  await toggle.click();
  await expect(heading).toBeDisabled();
  await expect(heading).not.toHaveAttribute("aria-expanded");
  await expect(rows).toHaveCount(count);
  const rect = await heading.boundingBox();
  await page.mouse.click(rect!.x + rect!.width / 2, rect!.y + rect!.height / 2);
  await expect(rows).toHaveCount(count);
  await expect(page).toHaveURL(/session\/session-1$/);

  // Re-enabling restores this view's previous collapsed choice.
  await menu.click();
  await expect(toggle).not.toBeChecked();
  await toggle.click();
  await expect(heading).toBeEnabled();
  await expect(rows).toHaveCount(0);
  await menu.click();
  await toggle.click();
  await expect(rows).toHaveCount(count);
  expect(
    await page.evaluate(() => JSON.parse(localStorage.getItem("settings")!)),
  ).toMatchObject({
    "editor.tabSize": 3,
    "session.panel.projectCollapse": false,
  });

  await page.reload();
  await expect(panel.locator(".tree-session")).toHaveCount(SANDBOX_SESSIONS, {
    timeout: 45000,
  });
  await expect(heading).toBeDisabled();
  await menu.click();
  await expect(toggle).not.toBeChecked();
  await page.keyboard.press("Escape");

  // The normal storage event path also applies this setting immediately.
  await page.evaluate(() => {
    const oldValue = localStorage.getItem("settings");
    const next = {
      ...JSON.parse(oldValue!),
      "session.panel.projectCollapse": true,
    };
    const newValue = JSON.stringify(next);
    localStorage.setItem("settings", newValue);
    window.dispatchEvent(
      new StorageEvent("storage", {
        key: "settings",
        oldValue,
        newValue,
        storageArea: localStorage,
      }),
    );
  });
  await expect(heading).toBeEnabled();
  await heading.focus();
  await page.keyboard.press("Enter");
  await expect(rows).toHaveCount(0);
  await page.keyboard.press("Space");
  await expect(rows).toHaveCount(count);
});
