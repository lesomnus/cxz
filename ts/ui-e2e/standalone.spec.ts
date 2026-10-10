import { expect, test } from "@playwright/test";

test("standalone switch supports pointer and keyboard activation and a disabled state", async ({
  page,
}) => {
  await page.goto("/iframe.html?id=components-switch--default&viewMode=story");
  const toggle = page.getByRole("switch", { name: "Copy on selection" });
  await expect(toggle).toHaveAttribute("aria-checked", "true");
  await toggle.click();
  await expect(toggle).toHaveAttribute("aria-checked", "false");
  await toggle.press("Space");
  await expect(toggle).toHaveAttribute("aria-checked", "true");
  await toggle.press("Enter");
  await expect(toggle).toHaveAttribute("aria-checked", "false");
  await page.goto("/iframe.html?id=components-switch--disabled&viewMode=story");
  await expect(toggle).toBeDisabled();
  await expect(toggle).toHaveAttribute("aria-checked", "true");
});

test("package styling and tokens work without resetting host controls", async ({
  page,
}) => {
  await page.goto("/iframe.html?id=components-button--default&viewMode=story");
  const button = page.getByRole("button", { name: "New session", exact: true });
  await expect(button).toHaveCSS("border-top-width", "0px");
  await button.hover();
  await expect(button).toHaveCSS("background-color", "rgb(40, 40, 40)");
  await page.evaluate(() => {
    const plain = document.createElement("button");
    plain.id = "host-button";
    plain.textContent = "Host button";
    document.body.append(plain);
  });
  await expect(page.locator("#host-button")).not.toHaveCSS(
    "border-top-width",
    "0px",
  );
  expect(
    await page.evaluate(() =>
      getComputedStyle(document.documentElement).getPropertyValue("--space"),
    ),
  ).toBe("");
  await page.mouse.move(0, 0);
  await button.focus();
  await page.keyboard.down("Space");
  await expect
    .poll(() =>
      button
        .locator(".button-content")
        .evaluate((node) => getComputedStyle(node).transform),
    )
    .not.toBe("matrix(1, 0, 0, 1, 0, 0)");
  await page.keyboard.up("Space");
  await page.goto(
    "/iframe.html?id=components-button--default&viewMode=story&globals=theme:light",
  );
  await button.hover();
  await expect(button).toHaveCSS("background-color", "rgb(230, 230, 230)");
});

test("portaled dropdown aligns the current value and handles keyboard selection", async ({
  page,
}) => {
  await page.goto(
    "/iframe.html?id=components-valuemenu--default&viewMode=story",
  );
  const trigger = page.getByRole("combobox", { name: "Effort" });
  const bounds = (await trigger.locator(".meta-value").boundingBox())!;
  await trigger.click();
  const current = page
    .getByRole("option", { name: "medium", exact: true })
    .filter({ has: page.locator(".meta-value") });
  const aligned = (await current.locator(".meta-value").boundingBox())!;
  expect(aligned.x).toBeCloseTo(bounds.x, 0);
  expect(aligned.y).toBeCloseTo(bounds.y, 0);
  await page.getByRole("option", { name: "high", exact: true }).click();
  await expect(trigger).toHaveText("high");
  await trigger.press("ArrowDown");
  await expect(page.getByRole("listbox")).toBeVisible();
  await page.keyboard.press("Escape");
  await expect(trigger).toBeFocused();
});

test("inline editor and modal use independent defaults and injected translations", async ({
  page,
}) => {
  await page.goto(
    "/iframe.html?id=integration-standalone--translated&viewMode=story",
  );
  const trigger = page.getByRole("button", { name: "Edit Name" });
  const initial = (await trigger.boundingBox())!;
  await trigger.click();
  const input = page.getByRole("textbox", { name: "Name" });
  const bounds = (await input.boundingBox())!;
  for (const key of ["x", "y", "width", "height"] as const)
    expect(bounds[key]).toBeCloseTo(initial[key], 0);
  await expect(page.getByRole("button", { name: "Dismiss" })).toBeVisible();
  await input.fill("New workspace");
  await input.press("Enter");
  await expect(trigger).toHaveText("New workspace");
  await page.goto(
    "/iframe.html?id=components-confirmationdialog--default&viewMode=story",
  );
  await page.getByRole("button", { name: "Open confirmation" }).click();
  await expect(
    page.getByRole("button", { name: "Cancel", exact: true }),
  ).toBeFocused();
  await page.getByRole("button", { name: "Stop", exact: true }).click();
  await expect(page.getByRole("status")).toHaveText("Action completed");
});

test("optional editor entry loads Monaco workers without an app settings store", async ({
  page,
}) => {
  const errors: string[] = [];
  page.on("pageerror", (e) => errors.push(e.message));
  await page.goto(
    "/iframe.html?id=editor-sourceeditor--settings-json&viewMode=story",
  );
  await expect(page.locator(".monaco-editor")).toBeVisible();
  await page.locator(".monaco-editor").click({ position: { x: 130, y: 24 } });
  await expect(
    page.getByRole("textbox", { name: "File preview" }),
  ).toBeFocused();
  await page.keyboard.press("Control+A");
  // Clear the selection: Monaco otherwise wraps it in typed braces/quotes.
  await page.keyboard.press("Backspace");
  await page.keyboard.type('{"editor.tabSize": 6}');
  await expect(page.locator(".monaco-editor .view-lines")).toContainText(
    '"editor.tabSize": 6',
  );
  await expect(page.locator(".monaco-editor .view-line")).toHaveText(
    '{"editor.tabSize": 6}',
  );
  expect(errors).toEqual([]);
});
