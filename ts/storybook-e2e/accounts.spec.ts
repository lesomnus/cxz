import { expect, test } from "@playwright/test";

test("account selection shows identity, scoped sessions, usage and actual login readiness", async ({
  page,
}) => {
  await page.goto("/iframe.html?id=resources-views--accounts&viewMode=story");
  const panel = page.getByRole("complementary", { name: "Account list" });
  await expect(
    panel.getByRole("img", { name: "Codex", exact: true }),
  ).toBeVisible();
  await expect(
    panel.getByRole("button", { name: /Work.*work/ }),
  ).toHaveAttribute("aria-pressed", "true");
  const work = panel.getByRole("button", { name: /Work.*work/ });
  const personal = panel.getByRole("button", { name: /Personal.*personal/ });
  const background = (element: HTMLElement) =>
    getComputedStyle(element).backgroundColor;
  const selectedBackground = await work.evaluate(background);
  expect(selectedBackground).not.toBe(await personal.evaluate(background));
  expect(selectedBackground).not.toBe("rgba(0, 0, 0, 0)");
  await work.hover();
  expect(await work.evaluate(background)).not.toBe(selectedBackground);
  const sessions = page.getByRole("region", {
    name: "Sessions using this account",
  });
  await expect(sessions).toContainText("Web account layout");
  await expect(sessions).not.toContainText("Claude project");
  await expect(
    page.getByRole("button", { name: "Log in", exact: true }),
  ).toBeDisabled();
  await expect(
    page.getByRole("button", { name: "Log out", exact: true }),
  ).toBeEnabled();
  await expect(
    page.getByText("Conversation usage", { exact: true }),
  ).toBeVisible();
  await panel.getByRole("button", { name: /Personal.*personal/ }).click();
  await page.getByRole("region", { name: "Account details" }).hover();
  expect(await personal.evaluate(background)).toBe(selectedBackground);
  expect(await work.evaluate(background)).not.toBe(selectedBackground);
  await expect(
    page.getByText("Session-scoped login", { exact: true }),
  ).toBeVisible();
  await expect(sessions).toContainText("Claude project");
  await expect(sessions).not.toContainText("Web account layout");
  await expect(
    page.getByRole("button", { name: "Log in", exact: true }),
  ).toHaveCount(0);
});

test("Add opens beside the panel and its language/provider dropdown works inside the modal", async ({
  page,
}) => {
  const errors: string[] = [];
  page.on("pageerror", (error) => errors.push(error.message));
  await page.goto("/iframe.html?id=resources-views--accounts&viewMode=story");
  const panel = page.getByRole("complementary", { name: "Account list" });
  const addBounds = (await panel
    .getByRole("button", { name: "Add account", exact: true })
    .boundingBox())!;
  await panel.getByRole("button", { name: "Add account", exact: true }).click();
  const dialog = page.getByRole("dialog", { name: "Add account", exact: true });
  await expect(dialog).toBeVisible();
  const foreground = dialog.locator('[data-dialog-trigger="true"]');
  const foregroundBounds = (await foreground.boundingBox())!;
  for (const key of ["x", "y", "width", "height"] as const)
    expect(foregroundBounds[key]).toBeCloseTo(addBounds[key], 1);
  await expect(foreground.locator("button")).toBeDisabled();
  await expect(foreground).toHaveAttribute("aria-hidden", "true");
  expect(
    await foreground.evaluate((node) => getComputedStyle(node).opacity),
  ).not.toBe("1");
  await page.mouse.click(
    addBounds.x + addBounds.width / 2,
    addBounds.y + addBounds.height / 2,
  );
  await expect(dialog).toBeVisible();
  const entrance = await dialog
    .locator("[data-anchored]")
    .evaluate((surface) => {
      const animation = surface.getAnimations()[0];
      return animation?.effect instanceof KeyframeEffect
        ? animation.effect.getKeyframes()
        : [];
    });
  expect(entrance[0]?.opacity).toBe("0");
  expect(entrance.at(-1)?.opacity).toBe("1");
  expect(entrance.at(-1)?.transform).toBe("none");
  // The top-layer dialog stays untransformed so fixed menus retain viewport coordinates.
  expect(
    await dialog.evaluate((node) => getComputedStyle(node).transform),
  ).toBe("none");
  const panelBox = (await panel.boundingBox())!,
    dialogBox = (await dialog.boundingBox())!;
  expect(dialogBox.x).toBeGreaterThanOrEqual(panelBox.x + panelBox.width - 1);
  await dialog.getByRole("combobox", { name: "Agent", exact: true }).click();
  await dialog.getByRole("option", { name: "Claude", exact: true }).click();
  await expect(
    dialog.getByRole("combobox", { name: "Agent", exact: true }),
  ).toContainText("Claude");
  await dialog
    .getByRole("textbox", { name: "Alias", exact: true })
    .fill("new-account");
  await dialog
    .getByRole("textbox", { name: "Display name", exact: true })
    .fill("New team account");
  await dialog.getByRole("button", { name: "Add", exact: true }).click();
  await expect(dialog).toHaveCount(0);
  await expect(
    panel.getByRole("button", { name: /New team account.*new-account/ }),
  ).toHaveAttribute("aria-pressed", "true");
  await expect(
    page.getByRole("region", { name: "Account details" }),
  ).toContainText("New team account");
  expect(errors).toEqual([]);
});

test("foreground trigger appears only while the account dialog fits beside the panel", async ({
  page,
}) => {
  await page.goto("/iframe.html?id=resources-views--accounts&viewMode=story");
  const panel = page.getByRole("complementary", { name: "Account list" });
  await panel.getByRole("button", { name: "Add account", exact: true }).click();
  const dialog = page.getByRole("dialog", { name: "Add account", exact: true });
  await expect(dialog.locator('[data-dialog-trigger="true"]')).toHaveCount(1);
  await page.setViewportSize({ width: 390, height: 700 });
  await expect(dialog).toBeVisible();
  await expect(dialog.locator('[data-dialog-trigger="true"]')).toHaveCount(0);
  await page.setViewportSize({ width: 1280, height: 720 });
  await expect(dialog.locator('[data-dialog-trigger="true"]')).toHaveCount(1);
  await dialog.getByRole("button", { name: "Cancel", exact: true }).click();
  await expect(dialog).toHaveCount(0);
});

test("dialogs respect reduced motion", async ({ page }) => {
  await page.emulateMedia({ reducedMotion: "reduce" });
  await page.goto("/iframe.html?id=resources-views--accounts&viewMode=story");
  await page.getByRole("button", { name: "Add account", exact: true }).click();
  const dialog = page.getByRole("dialog", { name: "Add account", exact: true });
  await expect(dialog).toBeVisible();
  expect(
    await dialog
      .locator("[data-anchored]")
      .evaluate((node) => getComputedStyle(node).animationName),
  ).toBe("none");
  await page.keyboard.press("Escape");
  await page.getByRole("button", { name: "Log out", exact: true }).click();
  const confirmation = page.getByRole("dialog", {
    name: "Log out · Work",
    exact: true,
  });
  expect(
    await confirmation
      .locator("[data-anchored]")
      .evaluate((node) => getComputedStyle(node).animationName),
  ).toBe("none");
});

for (const theme of ["dark", "light"]) {
  test(`account and confirmation dialogs share a translucent blurred surface (${theme})`, async ({
    page,
  }) => {
    await page.goto(
      `/iframe.html?id=resources-views--accounts&viewMode=story&globals=theme:${theme}`,
    );
    await page
      .getByRole("button", { name: "Add account", exact: true })
      .click();
    const addition = page.getByRole("dialog", {
      name: "Add account",
      exact: true,
    });
    const appearance = (node: HTMLElement) => {
      const surface = node.querySelector<HTMLElement>("[data-anchored]")!;
      const style = getComputedStyle(surface);
      const heading = getComputedStyle(node.querySelector(".card-heading")!);
      const body = getComputedStyle(node.querySelector(".card-body")!);
      const footer = getComputedStyle(node.querySelector(".card-footer")!);
      return {
        background: style.backgroundColor,
        blur: style.backdropFilter,
        radius: style.borderRadius,
        backdrop: getComputedStyle(node, "::backdrop").backgroundColor,
        headerPadding: heading.padding,
        bodyPadding: body.padding,
        bodyBackground: body.backgroundColor,
        bodyRadius: body.borderRadius,
        footerPadding: footer.padding,
      };
    };
    const initial = await addition.evaluate(appearance);
    expect(initial.blur).toMatch(/^blur\(/);
    expect(initial.bodyBackground).not.toBe("rgba(0, 0, 0, 0)");
    expect(initial.bodyRadius).not.toBe("0px");
    const alpha = await addition.locator("[data-anchored]").evaluate((node) => {
      const context = document.createElement("canvas").getContext("2d")!;
      context.fillStyle = getComputedStyle(node).backgroundColor;
      context.fillRect(0, 0, 1, 1);
      return context.getImageData(0, 0, 1, 1).data[3];
    });
    expect(alpha).toBeGreaterThan(0);
    expect(alpha).toBeLessThan(255);
    // Blur belongs to the animated surface itself, not a descendant inside its opacity layer.
    await expect(addition.locator(".floating-card")).toHaveCSS(
      "backdrop-filter",
      "none",
    );
    await page.keyboard.press("Escape");
    await page.getByRole("button", { name: "Log out", exact: true }).click();
    const confirmation = page.getByRole("dialog", {
      name: "Log out · Work",
      exact: true,
    });
    expect(await confirmation.evaluate(appearance)).toEqual(initial);
  });
}

test("logout needs confirmation and then enables login", async ({ page }) => {
  await page.goto("/iframe.html?id=resources-views--accounts&viewMode=story");
  await page.getByRole("button", { name: "Log out", exact: true }).click();
  const dialog = page.getByRole("dialog", {
    name: "Log out · Work",
    exact: true,
  });
  await dialog.getByRole("button", { name: "Cancel", exact: true }).click();
  await expect(page.getByText("Signed in", { exact: true })).toBeVisible();
  await page.getByRole("button", { name: "Log out", exact: true }).click();
  await dialog.getByRole("button", { name: "Log out", exact: true }).click();
  await expect(dialog).toHaveCount(0);
  await expect(
    page.getByRole("button", { name: "Log in", exact: true }),
  ).toBeEnabled();
  await expect(
    page.getByRole("button", { name: "Log out", exact: true }),
  ).toBeDisabled();
});
