import { expect, test } from "@playwright/test";

for (const action of ["Cancel", "Close", "Escape", "backdrop", "Confirm"]) {
  test(`account-login Add account uses the shared modal exit after ${action}`, async ({
    page,
  }) => {
    await page.goto(
      "/iframe.html?id=accounts-workflows--account-login&viewMode=story",
    );
    const launch = page.getByRole("button", {
      name: "Add account",
      exact: true,
    });
    await launch.click();
    const dialog = page.locator("dialog").filter({
      has: page.getByRole("textbox", { name: "Alias", exact: true }),
    });
    await dialog.evaluate(async (node) => {
      await Promise.all(
        node
          .getAnimations({ subtree: true })
          .map((animation) => animation.finished),
      );
      node.style.setProperty("--ui-dialog-exit-duration", "2s");
    });
    await expect(dialog.locator(".floating-card")).toHaveCount(1);
    const alias = dialog.getByRole("textbox", { name: "Alias", exact: true });
    await alias.fill("after-exit");
    await dialog
      .getByRole("textbox", { name: "Display name", exact: true })
      .fill("After exit");
    if (action === "Escape") await page.keyboard.press("Escape");
    else if (action === "backdrop") await page.mouse.click(1, 1);
    else
      await dialog.getByRole("button", { name: action, exact: true }).click();
    // Use DOM locators because the closing modal is intentionally aria-hidden.
    const closing = page.locator('dialog[data-closing="true"]');
    await expect(closing).toHaveCount(1);
    await expect(closing).toHaveJSProperty("open", true);
    await expect(closing).toHaveJSProperty("inert", true);
    const opacity = await closing.evaluate((node) => {
      const surface = node.querySelector<HTMLElement>(
        '[data-motion="default"]',
      )!;
      for (const animation of node.getAnimations({ subtree: true })) {
        const target = (animation.effect as KeyframeEffect | null)?.target;
        if (target === node || target === surface) {
          animation.pause();
          animation.currentTime = 1000;
        }
      }
      return Number(getComputedStyle(surface).opacity);
    });
    expect(opacity).toBeGreaterThan(0);
    expect(opacity).toBeLessThan(1);
    await expect(closing.locator("input[pattern]")).toHaveValue("after-exit");
    const added = page.getByRole("article", {
      name: "After exit",
      exact: true,
      includeHidden: true,
    });
    await expect(added).toHaveCount(0);
    await closing.evaluate((node) => {
      const surface = node.querySelector('[data-motion="default"]')!;
      for (const animation of node.getAnimations({ subtree: true })) {
        const target = (animation.effect as KeyframeEffect | null)?.target;
        if (target === node || target === surface) animation.finish();
      }
    });
    await expect(page.locator("dialog")).toHaveCount(0);
    await expect(launch).toBeFocused();
    await expect(added).toHaveCount(action === "Confirm" ? 1 : 0);
  });
}

test("account login cancel and successful authentication wait for the modal exit", async ({
  page,
}) => {
  await page.goto(
    "/iframe.html?id=accounts-workflows--account-login&viewMode=story",
  );
  for (const complete of [false, true]) {
    const launch = page.getByRole("button", { name: "Log in", exact: true });
    await launch.click();
    const dialog = page.locator("dialog");
    await dialog.evaluate((node) =>
      node.style.setProperty("--ui-dialog-exit-duration", "2s"),
    );
    if (complete) {
      await dialog
        .getByRole("textbox", { name: "Authentication code", exact: true })
        .fill("demo-code");
      await dialog
        .getByRole("button", { name: "Submit code", exact: true })
        .click();
    } else
      await dialog.getByRole("button", { name: "Cancel", exact: true }).click();
    await expect(dialog).toHaveAttribute("data-closing", "true");
    await expect(dialog).toHaveJSProperty("open", true);
    await dialog.evaluate((node) => {
      const surface = node.querySelector('[data-motion="default"]')!;
      for (const animation of node.getAnimations({ subtree: true })) {
        const target = (animation.effect as KeyframeEffect | null)?.target;
        if (target === node || target === surface) animation.finish();
      }
    });
    await expect(dialog).toHaveCount(0);
  }
  await expect(
    page.getByRole("button", { name: "Log out", exact: true }),
  ).toBeVisible();
});
