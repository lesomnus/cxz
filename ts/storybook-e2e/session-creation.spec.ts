import { expect, test } from "@playwright/test";

for (const width of [1280, 390]) {
  test(`Sessions creation opens a modal, waits and selects the new conversation at ${width}px`, async ({
    page,
  }) => {
    const errors: string[] = [];
    page.on("pageerror", (error) => errors.push(error.message));
    await page.setViewportSize({ width, height: 900 });
    await page.goto(
      "/iframe.html?id=pages-sessions--session-creation&viewMode=story",
    );
    const table = page.getByRole("table", { name: "Sessions", exact: true });
    await expect(page.getByText("No sessions", { exact: true })).toBeVisible();
    const add = page.getByRole("button", { name: "Add session", exact: true });
    await add.click();
    const modal = page.getByRole("dialog", {
      name: "Add session",
      exact: true,
    });
    const title = modal.getByRole("textbox", { name: "Title", exact: true });
    await expect
      .poll(() =>
        modal.evaluate((node) => node.contains(document.activeElement)),
      )
      .toBe(true);
    const bounds = (await modal.boundingBox())!;
    expect(bounds.x + bounds.width / 2).toBeCloseTo(width / 2, 0);
    await modal.getByRole("button", { name: "Cancel", exact: true }).click();
    await expect(modal).toHaveCount(0);
    await expect(add).toBeFocused();
    for (const index of width === 1280 ? [1, 2] : [1]) {
      await add.click();
      await expect(
        modal.getByRole("combobox", { name: "Project", exact: true }),
      ).toContainText("Web workspace");
      await modal
        .getByRole("combobox", { name: "Account", exact: true })
        .click();
      await page.getByRole("option", { name: /Work account/ }).click();
      const name = `New conversation ${index}`;
      await title.fill(name);
      const confirm = modal.getByRole("button", {
        name: "Confirm",
        exact: true,
      });
      await modal.evaluate(async (node) => {
        const animations: Animation[] = [];
        for (
          let parent: Element | null = node;
          parent;
          parent = parent.parentElement
        )
          animations.push(...parent.getAnimations());
        await Promise.all(
          animations
            .filter(
              (animation) =>
                animation.effect?.getTiming().iterations !== Infinity,
            )
            .map((animation) => animation.finished.catch(() => {})),
        );
      });
      const readyBounds = (await modal.boundingBox())!;
      await confirm.click();
      await expect(confirm).toHaveAttribute("aria-busy", "true");
      const pendingBounds = (await modal.boundingBox())!;
      expect(pendingBounds.height).toBe(readyBounds.height);
      expect(pendingBounds.y).toBe(readyBounds.y);
      await expect(
        modal.getByRole("button", { name: "Close", exact: true }),
      ).toBeDisabled();
      await expect(title).toBeDisabled();
      await expect(
        modal.getByRole("button", { name: "Cancel", exact: true }),
      ).toBeDisabled();
      await expect(modal).toHaveCount(0);
      await expect(
        page.getByText("Loading conversation…", { exact: true }),
      ).toBeVisible();
      const message = page.getByRole("textbox", {
        name: "Message",
        exact: true,
      });
      await expect(message).toBeVisible();
      await expect(
        page.getByText("Loading conversation…", { exact: true }),
      ).toHaveCount(0);
      await expect(message).toBeFocused();
      const selected = page.locator(".tree-session.active");
      await expect(selected).toContainText(name);
      await expect(selected).toHaveAttribute(
        "href",
        `/c/_/session/new-${index}`,
      );
      await page
        .getByRole("navigation", { name: "Preview history" })
        .getByRole("button", { name: "Back", exact: true })
        .click();
      await expect(table).toBeVisible();
      await expect(table.locator("tbody tr")).toHaveCount(index);
      await expect(
        table.getByRole("link", { name: new RegExp(name) }),
      ).toBeVisible();
    }
    expect(errors).toEqual([]);
  });
}
