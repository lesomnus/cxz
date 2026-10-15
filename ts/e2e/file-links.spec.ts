import { test, expect } from "@playwright/test";

for (const width of [2000, 390]) {
  test(`response file links open in the workspace preview at ${width}px`, async ({
    page,
    context,
  }) => {
    await page.setViewportSize({ width, height: 1000 });
    const popups: string[] = [];
    context.on("page", (popup) => popups.push(popup.url()));
    await page.goto("/");
    await page.getByLabel("Web access token").fill("a".repeat(32));
    await page.getByRole("button", { name: "Connect", exact: true }).click();
    await page
      .getByRole("link", { name: /demo-chat/ })
      .first()
      .click();
    const original = page.url();
    const draft = page.getByRole("textbox", { name: "Message", exact: true });
    await draft.fill("Keep this draft");
    const fileLink = page.getByRole("link", {
      name: "report file",
      exact: true,
    });
    await expect(fileLink).not.toHaveAttribute("target", "_blank");
    for (const name of ["External", "Network"]) {
      const external = page.getByRole("link", { name, exact: true });
      await expect(external).toHaveAttribute("target", "_blank");
      await expect(external).toHaveAttribute("rel", "noopener noreferrer");
    }
    await fileLink.locator("strong").click();
    const editor = page.getByRole("complementary", {
      name: "Workspace editor",
    });
    await expect(editor).toBeVisible();
    await expect(editor.locator(".view-lines")).toContainText(
      "Read from /tmp/report.md",
    );
    await expect(
      editor.getByRole("tab", { name: "report.md", exact: true }),
    ).toHaveAttribute("aria-selected", "true");
    await expect(editor.locator(".monaco-editor textarea")).toHaveJSProperty(
      "readOnly",
      true,
    );
    expect(page.url()).toBe(original);
    expect(popups).toEqual([]);
    if (width < 1600) {
      const bounds = (await editor.boundingBox())!;
      expect(bounds.x).toBeGreaterThanOrEqual(0);
      expect(bounds.x + bounds.width).toBeLessThanOrEqual(width);
      await editor
        .getByRole("button", { name: "Close file preview", exact: true })
        .click();
      await expect(editor).toBeHidden();
      await expect(draft).toHaveValue("Keep this draft");
    }
    await page
      .getByRole("link", { name: "encoded report", exact: true })
      .press("Enter");
    await expect(editor).toBeVisible();
    await expect(editor.locator(".view-lines")).toContainText(
      "Read from /tmp/a report.md",
    );
    if (width < 1600)
      await editor
        .getByRole("button", { name: "Close file preview", exact: true })
        .click();
    await page
      .getByRole("link", { name: "missing report", exact: true })
      .click();
    await expect(editor.getByRole("alert")).toContainText(
      "report does not exist",
    );
    if (width < 1600)
      await editor
        .getByRole("button", { name: "Close file preview", exact: true })
        .click();
    await fileLink.click();
    await expect(editor.locator(".view-lines")).toContainText(
      "Read from /tmp/report.md",
    );
    await expect(editor.getByRole("alert")).toHaveCount(0);
    expect(page.url()).toBe(original);
    expect(popups).toEqual([]);
  });
}
