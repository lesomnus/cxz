import { expect, test } from "@playwright/test";

test("inline edits keep value bounds and font, support hover and never submit the parent form", async ({
  page,
}) => {
  await page.goto(
    "/iframe.html?id=components-editablevalue--alias&viewMode=story",
  );
  const trigger = page.getByRole("button", { name: "Edit Alias", exact: true });
  const bounds = (await trigger.boundingBox())!;
  const font = await trigger.evaluate(
    (node) => getComputedStyle(node).fontFamily,
  );
  await expect(trigger).toHaveCSS("background-color", "rgba(0, 0, 0, 0)");
  await trigger.hover();
  await expect(trigger).not.toHaveCSS("background-color", "rgba(0, 0, 0, 0)");
  await trigger.click();
  const dialog = page.getByRole("dialog", { name: "Edit Alias" });
  const input = dialog.getByRole("textbox", { name: "Alias", exact: true });
  await expect(input).toBeFocused();
  const editing = (await input.boundingBox())!;
  for (const key of ["x", "y", "width", "height"] as const)
    expect(editing[key]).toBeCloseTo(bounds[key], 0);
  await expect(input).toHaveCSS("font-family", font);
  expect(
    await dialog.evaluate(
      (node) => getComputedStyle(node, "::backdrop").backgroundColor,
    ),
  ).not.toBe("rgba(0, 0, 0, 0)");
  await input.fill("pine");
  await input.press("Escape");
  await expect(dialog).toHaveCount(0);
  await expect(trigger).toBeFocused();
  await expect(trigger).toHaveText("oak");
  await trigger.click();
  await input.fill("oak-tree");
  await input.press("Enter");
  await expect(dialog).toHaveCount(0);
  await expect(trigger).toHaveText("oak-tree");
  await expect(page.getByRole("status")).toHaveText("Parent submissions: 0");
  await page.setViewportSize({ width: 390, height: 600 });
  await trigger.click();
  const mobile = (await input.boundingBox())!;
  expect(mobile.x).toBeGreaterThanOrEqual(0);
  expect(mobile.x + mobile.width).toBeLessThanOrEqual(390);
  const controls = (await dialog
    .locator(".editable-value-overlay")
    .boundingBox())!;
  expect(controls.x).toBeGreaterThanOrEqual(0);
  expect(controls.x + controls.width).toBeLessThanOrEqual(390);
  await page.screenshot({ path: "test-results/editable-value-mobile.png" });
  await page.mouse.click(380, 400);
  await expect(dialog).toHaveCount(0);
});

test("validation and rejected edits keep the draft available for correction and retry", async ({
  page,
}) => {
  await page.goto(
    "/iframe.html?id=components-editablevalue--failure&viewMode=story",
  );
  const trigger = page.getByRole("button", { name: "Edit Alias" });
  await trigger.click();
  const dialog = page.getByRole("dialog", { name: "Edit Alias" });
  const input = dialog.getByRole("textbox", { name: "Alias" });
  await input.fill("Bad_Alias");
  await input.press("Enter");
  await expect(dialog.getByRole("alert")).toContainText("Alias must be");
  await expect(input).toHaveValue("Bad_Alias");
  await input.fill("taken");
  await input.press("Enter");
  await expect(dialog.getByRole("alert")).toContainText("already in use");
  await expect(input).toHaveValue("taken");
  await expect(trigger).toHaveText("oak");
  await page.screenshot({ path: "test-results/editable-value-error.png" });
  await input.fill("pine");
  await dialog.getByRole("button", { name: "Confirm", exact: true }).click();
  await expect(dialog).toHaveCount(0);
  await expect(trigger).toHaveText("pine");
});

test("pending edits prevent dismissal and duplicate activation", async ({
  page,
}) => {
  await page.goto(
    "/iframe.html?id=components-editablevalue--pending&viewMode=story",
  );
  const trigger = page.getByRole("button", { name: "Edit Alias" });
  await trigger.click();
  const dialog = page.getByRole("dialog", { name: "Edit Alias" });
  const input = dialog.getByRole("textbox", { name: "Alias" });
  await input.fill("pine");
  await input.press("Enter");
  await expect(dialog).toHaveAttribute("aria-busy", "true");
  await expect(input).toHaveAttribute("readonly", "");
  await expect(
    dialog.getByRole("button", { name: "Cancel", exact: true }),
  ).toBeDisabled();
  await input.press("Escape");
  await input.press("Enter");
  await page.mouse.click(900, 400);
  await expect(dialog).toBeVisible();
  await expect(dialog).toHaveCount(0);
  await expect(trigger).toHaveText("pine");
  await expect(page.getByRole("status")).toHaveText("Parent submissions: 0");
});

test("Details and composer previews retain both title and alias across sequential edits", async ({
  page,
}) => {
  for (const story of [
    "components-sessiondetails--default",
    "conversation-composer--empty",
  ]) {
    await page.goto(`/iframe.html?id=${story}&viewMode=story`);
    if (story.startsWith("conversation")) {
      await page
        .getByRole("button", { name: "Session menu", exact: true })
        .click();
      await page
        .getByRole("menuitem", { name: "Details", exact: true })
        .click();
    }
    await page.getByRole("button", { name: "Edit Title", exact: true }).click();
    const title = page.getByRole("dialog", { name: "Edit Title", exact: true });
    await title
      .getByRole("textbox", { name: "Title", exact: true })
      .fill("Review session identity");
    await title.getByRole("button", { name: "Confirm", exact: true }).click();
    await expect(title).toHaveCount(0);
    await page.getByRole("button", { name: "Edit Alias", exact: true }).click();
    const alias = page.getByRole("dialog", { name: "Edit Alias", exact: true });
    await alias
      .getByRole("textbox", { name: "Alias", exact: true })
      .fill("oak-tree");
    await alias.getByRole("button", { name: "Confirm", exact: true }).click();
    await expect(alias).toHaveCount(0);
    await expect(
      page.getByRole("button", { name: "Edit Title", exact: true }),
    ).toHaveText("Review session identity");
    await expect(
      page.getByRole("button", { name: "Edit Alias", exact: true }),
    ).toHaveText("oak-tree");
    if (story.startsWith("conversation")) {
      await page
        .getByRole("button", { name: "Close details", exact: true })
        .click();
      await page
        .getByRole("button", { name: "Session menu", exact: true })
        .click();
      await page
        .getByRole("menuitem", { name: "Details", exact: true })
        .click();
      await expect(
        page.getByRole("button", { name: "Edit Title", exact: true }),
      ).toHaveText("Review session identity");
      await expect(
        page.getByRole("button", { name: "Edit Alias", exact: true }),
      ).toHaveText("oak-tree");
      await expect(
        page.getByRole("dialog", { name: "Session details", exact: true }),
      ).toHaveCSS("opacity", "1");
      await page.screenshot({
        path: "test-results/session-details-storybook.png",
      });
    }
  }
});
