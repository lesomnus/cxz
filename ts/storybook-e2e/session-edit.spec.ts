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
  const row = page.locator(".session-details > div").filter({ has: trigger });
  await expect(row.locator("dt")).toHaveCSS("font-family", font);
  await expect(row.locator("dd")).toHaveCSS("font-family", font);
  await expect(row).toHaveCSS("padding", "2px");
  await expect(row.locator("dt")).toHaveCSS("padding-left", "8px");
  await expect(row.locator("dt")).toHaveCSS("padding-right", "8px");
  const geometry = await row.evaluate((node) => {
    const style = getComputedStyle(node);
    const value = node.querySelector("button")!;
    const outer = node.getBoundingClientRect();
    const inner = value.getBoundingClientRect();
    return {
      inset: parseFloat(style.paddingRight),
      padding: [
        style.paddingTop,
        style.paddingRight,
        style.paddingBottom,
        style.paddingLeft,
      ],
      outerRadius: parseFloat(style.borderTopRightRadius),
      innerRadius: parseFloat(getComputedStyle(value).borderTopRightRadius),
      top: inner.top - outer.top,
      right: outer.right - inner.right,
      bottom: outer.bottom - inner.bottom,
    };
  });
  expect(new Set(geometry.padding).size).toBe(1);
  for (const inset of [geometry.top, geometry.right, geometry.bottom])
    expect(inset).toBeCloseTo(geometry.inset, 1);
  expect(geometry.innerRadius + geometry.inset).toBeCloseTo(
    geometry.outerRadius,
    1,
  );
  await expect(trigger).toHaveCSS("background-color", "rgba(0, 0, 0, 0)");
  await row.locator("dt").hover();
  await expect(row).not.toHaveCSS("background-color", "rgba(0, 0, 0, 0)");
  await expect(trigger).not.toHaveCSS("background-color", "rgba(0, 0, 0, 0)");
  expect(
    await row.evaluate((node) => getComputedStyle(node).backgroundColor),
  ).not.toBe(
    await trigger.evaluate((node) => getComputedStyle(node).backgroundColor),
  );
  await trigger.click();
  const dialog = page.getByRole("dialog", { name: "Edit Alias" });
  const input = dialog.getByRole("textbox", { name: "Alias", exact: true });
  await expect(input).toBeFocused();
  const editing = (await input.boundingBox())!;
  for (const key of ["x", "y", "width", "height"] as const)
    expect(editing[key]).toBeCloseTo(bounds[key], 0);
  await expect(input).toHaveCSS("font-family", font);
  await expect(input).toHaveCSS(
    "border-radius",
    await trigger.evaluate((node) => getComputedStyle(node).borderRadius),
  );
  await expect(input).toHaveCSS(
    "padding-left",
    await trigger.evaluate((node) => getComputedStyle(node).paddingLeft),
  );
  await expect(input).toHaveCSS(
    "padding-right",
    await trigger.evaluate((node) => getComputedStyle(node).paddingRight),
  );
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
      await page
        .locator(".session-details > div")
        .filter({
          has: page.getByRole("button", { name: "Edit Title", exact: true }),
        })
        .locator("dt")
        .hover();
      await page.screenshot({
        path: "test-results/session-details-storybook.png",
      });
    }
  }
});
