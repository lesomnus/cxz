import { expect, test, type Page } from "@playwright/test";

async function open(page: Page, story: string) {
  await page.goto(
    `/iframe.html?id=accounts-workflows--${story}&viewMode=story`,
  );
}
const loginPanel = (page: Page) =>
  page.getByRole("region", { name: "Provider login", exact: true });
async function finishLogin(page: Page) {
  const panel = loginPanel(page);
  await expect(panel).toContainText("demo-code");
  await panel.getByLabel("Authentication code").fill("demo-code");
  await panel.getByRole("button", { name: "Submit code", exact: true }).click();
  await expect(panel).toHaveCount(0);
}

test("account creation uses the actual form and Reset clears its isolated state", async ({
  page,
}) => {
  await open(page, "add-account");
  await expect(
    page.getByText("No accounts yet.", { exact: true }),
  ).toBeVisible();
  await page.getByRole("button", { name: "Add account", exact: true }).click();
  const form = page.getByRole("dialog", { name: "Add account", exact: true });
  const aliasInput = form.getByRole("textbox", { name: "Alias", exact: true });
  await expect(
    form.getByRole("group", { name: "Account details", exact: true }),
  ).toBeVisible();
  await expect
    .poll(() =>
      aliasInput.evaluate((node) => {
        const css = getComputedStyle(node);
        const label = node.ownerDocument.querySelector(
          `label[for="${node.id}"]`,
        )!;
        return {
          padding: css.padding,
          radius: css.borderRadius,
          border: css.borderWidth,
          labelInset:
            label.getBoundingClientRect().left -
            node.getBoundingClientRect().left,
        };
      }),
    )
    .toEqual({
      padding: "12px",
      radius: "10px",
      border: "0px",
      labelInset: 10,
    });
  const agent = form.getByRole("combobox", { name: "Agent", exact: true });
  await expect(agent.locator(".agent-brand-codex svg")).toBeVisible();
  expect((await agent.boundingBox())!.height).toBe(
    (await aliasInput.boundingBox())!.height,
  );
  await agent.click();
  await expect(
    form
      .getByRole("option", { name: "Codex", exact: true })
      .locator(".agent-brand-codex svg"),
  ).toBeVisible();
  await expect(
    form
      .getByRole("option", { name: "Claude", exact: true })
      .locator(".agent-brand-claude img"),
  ).toBeVisible();
  await expect(form.locator(".setting-menu")).toHaveCSS(
    "border-top-left-radius",
    "13px",
  );
  await page.keyboard.press("Escape");
  await aliasInput.fill("INVALID_ALIAS");
  await expect(
    form.getByRole("button", { name: "Add", exact: true }),
  ).toBeDisabled();
  await form
    .getByRole("textbox", { name: "Alias", exact: true })
    .fill("design");
  await form
    .getByRole("textbox", { name: "Display name", exact: true })
    .fill("Design account");
  await form.getByRole("button", { name: "Add", exact: true }).click();
  const card = page.getByRole("article", {
    name: "Design account",
    exact: true,
  });
  await expect(card).toContainText("Login required");
  await card.getByRole("button", { name: "Log in", exact: true }).click();
  await finishLogin(page);
  await expect(card.getByText("Signed in", { exact: true })).toBeVisible();
  await page
    .getByRole("button", { name: "Reset preview", exact: true })
    .click();
  await expect(
    page.getByText("No accounts yet.", { exact: true }),
  ).toBeVisible();
  await expect(card).toHaveCount(0);
});

for (const viewport of [
  { width: 1200, height: 900, fractional: false },
  { width: 390, height: 720, fractional: false },
  { width: 1200, height: 900, fractional: true },
]) {
  test(`agent selection stays on its trigger after changing and reopening at width ${viewport.width}${viewport.fractional ? " with fractional height" : ""}`, async ({
    page,
  }, info) => {
    await page.setViewportSize(viewport);
    await open(page, "add-account");
    await page
      .getByRole("button", { name: "Add account", exact: true })
      .click();
    const trigger = page.getByRole("combobox", { name: "Agent", exact: true });
    for (const [current, next] of [
      ["Codex", "Claude"],
      ["Claude", "Codex"],
      ["Codex", "Claude"],
      ["Claude", "Claude"],
    ]) {
      await expect(trigger).toHaveText(current);
      if (viewport.fractional && current === "Claude") {
        await page
          .getByRole("group", { name: "Account details", exact: true })
          .evaluate((node) =>
            node.style.setProperty(
              "--ui-form-control-padding-block",
              "11.875px",
            ),
          );
      }
      const box = (await trigger.boundingBox())!;
      await page.mouse.move(box.x + box.width / 2, box.y + box.height / 2);
      await page.mouse.down();
      // Include a real held press, so the content has reached its pressed scale.
      await page.waitForTimeout(120);
      await page.mouse.up();
      const popup = page.locator(".setting-menu");
      const selected = popup.getByRole("option", {
        name: current,
        exact: true,
      });
      await expect(popup).toBeVisible();
      await expect(popup).toHaveAttribute("data-item-alignment", "selected");
      await expect(selected).toHaveAttribute("aria-selected", "true");
      await expect(selected).toBeFocused();
      await expect
        .poll(async () => {
          const a = (await trigger.boundingBox())!,
            b = (await selected.boundingBox())!;
          return Math.max(
            ...(["x", "y", "width", "height"] as const).map((key) =>
              Math.abs(a[key] - b[key]),
            ),
          );
        })
        .toBeLessThan(0.1);
      if (current === "Claude") {
        await page.screenshot({
          path: info.outputPath("claude-item-aligned.png"),
        });
      }
      await popup.getByRole("option", { name: next, exact: true }).click();
      await expect(popup).toHaveCount(0);
    }
  });
}

for (const zoom of [1, 1.25, 1.5]) {
  test(`Account Login preserves agent item alignment on repeated selections at CSS zoom ${zoom}`, async ({
    page,
  }, info) => {
    await page.setViewportSize({ width: 1600, height: 1000 });
    await open(page, "account-login");
    await page
      .getByRole("article", { name: "Work account", exact: true })
      .waitFor({ state: "attached" });
    await page.evaluate((zoom) => {
      document.body.style.zoom = String(zoom);
    }, zoom);
    await page
      .getByRole("button", { name: "Add account", exact: true })
      .click();
    const form = page.getByRole("dialog", { name: "Add account", exact: true });
    const trigger = form.getByRole("combobox", { name: "Agent", exact: true });
    for (const [current, next] of [
      ["Codex", "Claude"],
      ["Claude", "Codex"],
      ["Codex", "Claude"],
      ["Claude", "Claude"],
    ]) {
      await expect(trigger).toHaveText(current);
      await trigger.click();
      const popup = form.locator(".setting-menu");
      const selected = popup.getByRole("option", {
        name: current,
        exact: true,
      });
      await expect(popup).toHaveAttribute("data-item-alignment", "selected");
      await expect(selected).toBeFocused();
      await expect
        .poll(async () => {
          const a = (await trigger.boundingBox())!,
            b = (await selected.boundingBox())!;
          return Math.max(
            ...(["x", "y", "width", "height"] as const).map((key) =>
              Math.abs(a[key] - b[key]),
            ),
          );
        })
        .toBeLessThan(0.1);
      await expect(trigger.locator(".button-content")).toHaveCSS(
        "opacity",
        "0",
      );
      if (current === "Claude")
        await form.screenshot({
          path: info.outputPath("account-login-claude-aligned.png"),
        });
      await popup.getByRole("option", { name: next, exact: true }).click();
      await expect(popup).toHaveCount(0);
    }
  });
}

test("account login refreshes status and logout uses confirmation", async ({
  page,
}) => {
  await open(page, "account-login");
  const actionWidth = (await page
    .getByRole("button", { name: "Log in", exact: true })
    .boundingBox())!.width;
  await page.getByRole("button", { name: "Log in", exact: true }).click();
  const dialog = page.getByRole("dialog", {
    name: "Log in · work",
    exact: true,
  });
  await expect(dialog).toBeVisible();
  await expect(dialog.locator(".dialog-header")).toHaveText("Log in · work");
  await expect(
    dialog.locator(".dialog-body").getByLabel("Authentication code"),
  ).toBeFocused();
  await dialog.evaluate(async (node) => {
    await Promise.all(
      node
        .getAnimations({ subtree: true })
        .filter(
          (animation) => animation.effect?.getTiming().iterations !== Infinity,
        )
        .map((animation) => animation.finished.catch(() => {})),
    );
  });
  const fieldLayout = await dialog
    .getByLabel("Authentication code")
    .evaluate((input) => {
      const label = input.ownerDocument.querySelector(
        `label[for="${input.id}"]`,
      )!;
      const css = getComputedStyle(input);
      return {
        labelInset:
          label.getBoundingClientRect().left -
          input.getBoundingClientRect().left,
        radius: parseFloat(css.borderTopLeftRadius),
        insideScroll: !!input.closest('[data-scroll-area="viewport"]'),
      };
    });
  expect(fieldLayout.labelInset).toBe(fieldLayout.radius);
  expect(fieldLayout.insideScroll).toBe(false);
  await expect(dialog.locator(".dialog-footer").getByRole("button")).toHaveText(
    ["Submit code", "Cancel"],
  );
  await finishLogin(page);
  await expect(dialog).toHaveCount(0);
  await expect(page.getByText("Signed in", { exact: true })).toBeVisible();
  await expect(
    page.getByRole("button", { name: "Log in", exact: true }),
  ).toHaveCount(0);
  expect(
    (await page
      .getByRole("button", { name: "Log out", exact: true })
      .boundingBox())!.width,
  ).toBe(actionWidth);
  await page.getByRole("button", { name: "Log out", exact: true }).click();
  const confirmation = page.getByRole("dialog", {
    name: "Log out · Work account",
    exact: true,
  });
  await confirmation
    .getByRole("button", { name: "Cancel", exact: true })
    .click();
  await expect(page.getByText("Signed in", { exact: true })).toBeVisible();
  await page.getByRole("button", { name: "Log out", exact: true }).click();
  await confirmation
    .getByRole("button", { name: "Log out", exact: true })
    .click();
  await expect(page.getByText("Login required", { exact: true })).toBeVisible();
  await expect(
    page.getByRole("button", { name: "Log in", exact: true }),
  ).toBeEnabled();
});

test("account login modal preserves page layout, traps focus and cancels from Escape or the backdrop on mobile", async ({
  page,
}, testInfo) => {
  await page.setViewportSize({ width: 390, height: 760 });
  await open(page, "account-login");
  const trigger = page.getByRole("button", { name: "Log in", exact: true });
  await expect(trigger).toBeEnabled();
  await trigger.scrollIntoViewIfNeeded();
  const card = page.locator('[data-account="work"]');
  const before = await card.boundingBox();
  await trigger.click();
  const dialog = page.getByRole("dialog", {
    name: "Log in · work",
    exact: true,
  });
  await expect(dialog).toBeVisible();
  expect(await card.boundingBox()).toEqual(before);
  const code = dialog.getByLabel("Authentication code");
  await expect(code).toBeFocused();
  await page.keyboard.press("Shift+Tab");
  await expect(
    dialog.getByRole("button", { name: "Cancel", exact: true }),
  ).toBeFocused();
  await page.keyboard.press("Tab");
  await expect(code).toBeFocused();
  await dialog.evaluate(async (node) => {
    await Promise.all(
      node
        .getAnimations({ subtree: true })
        .map((animation) => animation.finished.catch(() => {})),
    );
  });
  const bounds = (await dialog.boundingBox())!;
  expect(bounds.x).toBeGreaterThanOrEqual(0);
  expect(bounds.x + bounds.width).toBeLessThanOrEqual(390);
  expect(bounds.y).toBeGreaterThanOrEqual(0);
  expect(bounds.y + bounds.height).toBeLessThanOrEqual(760);
  await page.screenshot({
    path: testInfo.outputPath("account-login-modal-mobile.png"),
  });
  await page.keyboard.press("Escape");
  await expect(dialog).toHaveCount(0);
  await expect(page.getByText("Login required", { exact: true })).toBeVisible();
  await trigger.click();
  await expect(dialog).toBeVisible();
  await page.mouse.click(1, 1);
  await expect(dialog).toHaveCount(0);
  await expect(page.getByText("Login required", { exact: true })).toBeVisible();
  await trigger.click();
  await dialog.getByLabel("Authentication code").fill("demo-code");
  await expect(
    dialog.getByRole("button", { name: "Submit code", exact: true }),
  ).toBeEnabled();
  await dialog.getByLabel("Authentication code").press("Enter");
  await expect(dialog).toHaveCount(0);
  await expect(page.getByText("Signed in", { exact: true })).toBeVisible();
});

for (const story of ["login-failure-and-retry", "login-connection-closed"]) {
  test(`${story} starts a fresh successful login after retry`, async ({
    page,
  }) => {
    await open(page, story);
    await page.getByRole("button", { name: "Log in", exact: true }).click();
    const panel = loginPanel(page);
    await expect(panel.getByRole("alert")).toBeVisible();
    await expect(
      page.getByRole("button", {
        name: "Log in",
        exact: true,
        includeHidden: true,
      }),
    ).toBeDisabled();
    await panel
      .getByRole("button", { name: "Retry login", exact: true })
      .click();
    await finishLogin(page);
    await expect(page.getByText("Signed in", { exact: true })).toBeVisible();
  });
}

test("account login keeps its frame and controls fixed through loading, errors and body scrolling", async ({
  page,
}, testInfo) => {
  await page.emulateMedia({ reducedMotion: "reduce" });
  await page.clock.install();
  await open(page, "login-failure-and-retry");
  await page.getByRole("button", { name: "Log in", exact: true }).click();
  const dialog = page.getByRole("dialog", {
    name: "Log in · work",
    exact: true,
  });
  const geometry = () =>
    dialog.evaluate((node) => {
      const rect = (selector: string) =>
        node.querySelector(selector)!.getBoundingClientRect().toJSON();
      return {
        dialog: node.getBoundingClientRect().toJSON(),
        header: rect(".dialog-header"),
        body: rect(".dialog-body"),
        code: rect("input"),
        footer: rect(".dialog-footer"),
        submit: rect("button[type=submit]"),
      };
    });
  await expect(dialog).toBeVisible();
  const initial = await geometry();
  await page.clock.runFor(300);
  await expect(dialog).toContainText("demo-code");
  expect(await geometry()).toEqual(initial);
  await page.clock.runFor(400);
  await expect(dialog.getByRole("alert")).toBeVisible();
  await expect(
    dialog.getByRole("button", { name: "Retry login", exact: true }),
  ).toBeVisible();
  expect(await geometry()).toEqual(initial);
  await page.setViewportSize({ width: 390, height: 300 });
  await page.clock.runFor(100);
  const viewport = dialog.locator('[data-scroll-area="viewport"]');
  expect(
    await viewport.evaluate((node) => node.scrollHeight > node.clientHeight),
  ).toBe(true);
  const footer = (await dialog.locator(".dialog-footer").boundingBox())!;
  const code = dialog.getByLabel("Authentication code");
  await expect(code).toBeInViewport();
  const codeBeforeScroll = await code.boundingBox();
  await viewport.hover();
  await expect(
    dialog.getByRole("scrollbar", { name: "Login instructions", exact: true }),
  ).toBeVisible();
  await viewport.evaluate((node) => {
    node.scrollTop = node.scrollHeight;
  });
  await page.clock.runFor(50);
  expect(await dialog.locator(".dialog-footer").boundingBox()).toEqual(footer);
  expect(await code.boundingBox()).toEqual(codeBeforeScroll);
  const bounds = (await dialog.boundingBox())!;
  expect(bounds.y).toBeGreaterThanOrEqual(0);
  expect(bounds.y + bounds.height).toBeLessThanOrEqual(300);
  await page.screenshot({
    path: testInfo.outputPath("fixed-login-modal-short-viewport.png"),
  });
  await dialog.getByRole("button", { name: "Cancel", exact: true }).click();
});

for (const destination of ["conversation", "project"] as const) {
  test(`account session ${destination} buttons navigate to their own resource`, async ({
    page,
  }) => {
    const errors: string[] = [];
    page.on("pageerror", (error) => errors.push(error.message));
    await open(page, "signed-in-account");
    const table = page.getByRole("table", { name: "Sessions", exact: true });
    if (destination === "conversation") {
      const link = table.getByRole("link", {
        name: "Account login workflow seal",
        exact: true,
      });
      await expect(link).toHaveAttribute("href", "/c/_/session/seal");
      await link.click();
      await expect(
        page.getByRole("textbox", { name: "Message", exact: true }),
      ).toBeVisible();
    } else {
      const link = table.getByRole("link", {
        name: "Demo workspace workspace",
        exact: true,
      });
      await expect(link).toHaveAttribute("href", "/c/_/project/workspace");
      await link.focus();
      await page.keyboard.press("Enter");
      await expect(
        page.getByRole("main", { name: "Project workspace", exact: true }),
      ).toBeVisible();
      await expect(
        page.getByRole("combobox", { name: "Select project", exact: true }),
      ).toContainText("Demo workspace");
    }
    expect(errors).toEqual([]);
  });
}

test("account session row menus only contain lifecycle controls", async ({
  page,
}) => {
  await open(page, "signed-in-account");
  const table = page.getByRole("table", { name: "Sessions", exact: true });
  await table
    .getByRole("button", {
      name: "Controls for Account login workflow",
      exact: true,
    })
    .click();
  await expect(page.getByRole("menuitem")).toHaveText([
    "Stop",
    "Restart",
    "Purge",
  ]);
  await expect(
    page.getByRole("menuitem", { name: "Open conversation", exact: true }),
  ).toHaveCount(0);
  await page.keyboard.press("Escape");
  await expect(table).toBeVisible();
});

test("canceling a submitted login cannot authenticate later", async ({
  page,
}) => {
  await page.clock.install();
  await open(page, "account-login");
  await page.getByRole("button", { name: "Log in", exact: true }).click();
  const panel = loginPanel(page);
  await panel.getByLabel("Authentication code").fill("demo-code");
  await panel.getByRole("button", { name: "Submit code", exact: true }).click();
  await panel.getByRole("button", { name: "Cancel", exact: true }).click();
  await expect(panel).toHaveCount(0);
  await page.clock.fastForward(1000);
  await page.getByRole("button", { name: "Log in", exact: true }).click();
  await loginPanel(page)
    .getByRole("button", { name: "Cancel", exact: true })
    .click();
  await expect(page.getByText("Login required", { exact: true })).toBeVisible();
  await page.getByRole("button", { name: "Log in", exact: true }).click();
  await finishLogin(page);
  await expect(page.getByText("Signed in", { exact: true })).toBeVisible();
});

test("session creation performs its scoped login and reuses the creation key", async ({
  page,
}) => {
  await open(page, "session-creation-requires-login");
  await page.getByRole("button", { name: "Add session", exact: true }).click();
  const form = page.getByRole("dialog", { name: "Add session", exact: true });
  await form.getByRole("combobox", { name: "Account", exact: true }).click();
  await form
    .getByRole("option", { name: "Claude work account · claude", exact: true })
    .click();
  await form
    .getByRole("textbox", { name: "Title", exact: true })
    .fill("Authenticated preview");
  await form
    .getByRole("button", { name: "Create session", exact: true })
    .click();
  await expect(loginPanel(page)).toBeVisible();
  await expect(
    form.getByRole("textbox", { name: "Title", exact: true }),
  ).toBeDisabled();
  await expect(
    form.getByRole("group", { name: "Session details", exact: true }),
  ).toHaveJSProperty("disabled", true);
  await expect(
    loginPanel(page).getByLabel("Authentication code"),
  ).toBeEnabled();
  await finishLogin(page);
  await expect(form).toHaveCount(0);
  await expect(
    page.getByRole("textbox", { name: "Message", exact: true }),
  ).toBeVisible();
  await expect(
    page.getByRole("button", { name: "Log in again", exact: true }),
  ).toHaveCount(0);
});

test("sending discovers expired OAuth and login resumes without replaying the input", async ({
  page,
}) => {
  const errors: string[] = [];
  page.on("pageerror", (error) => errors.push(error.message));
  await open(page, "conversation-requires-login");
  const input = page.getByRole("textbox", { name: "Message", exact: true });
  await expect(input).toHaveValue(
    "Review the workspace and suggest the next step.",
  );
  await page.getByRole("button", { name: "Send", exact: true }).click();
  const login = page.getByRole("button", { name: "Log in again", exact: true });
  await login.click();
  await finishLogin(page);
  await expect(login).toHaveCount(0);
  await expect(page.locator("article.input")).toHaveCount(1);
  await input.fill("Continue after login.");
  await page.getByRole("button", { name: "Send", exact: true }).click();
  await expect(
    page.getByText(/Login is complete\. This response/),
  ).toBeVisible();
  expect(errors).toEqual([]);
});

test("an expired session keeps its recovery notice when login is canceled on mobile", async ({
  page,
}, testInfo) => {
  await page.setViewportSize({ width: 390, height: 800 });
  await open(page, "expired-session");
  const login = page.getByRole("button", { name: "Log in again", exact: true });
  await login.click();
  const panel = loginPanel(page);
  await expect(panel).toBeVisible();
  const card = page.getByRole("dialog", {
    name: "Claude account login",
    exact: true,
  });
  await expect(card).toHaveAttribute("data-entered", "true");
  await card.evaluate(async (node) => {
    await Promise.all(
      node
        .getAnimations({ subtree: true })
        .filter(
          (animation) => animation.effect?.getTiming().iterations !== Infinity,
        )
        .map((animation) => animation.finished.catch(() => {})),
    );
  });
  const bounds = (await card.boundingBox())!;
  const composer = (await page.locator(".composer-wrapper").boundingBox())!;
  const gap = await card.evaluate((node) =>
    parseFloat(getComputedStyle(node).bottom),
  );
  expect(bounds.x).toBeCloseTo(composer.x, 0);
  expect(bounds.width).toBeCloseTo(composer.width, 0);
  expect(bounds.y + bounds.height).toBeCloseTo(composer.y - gap, 0);
  await page.screenshot({
    path: testInfo.outputPath("mobile-session-login.png"),
  });
  await panel.getByRole("button", { name: "Cancel", exact: true }).click();
  await expect(panel).toHaveCount(0);
  await expect(login).toBeVisible();
  expect(
    await page.evaluate(
      () => document.documentElement.scrollWidth <= innerWidth,
    ),
  ).toBe(true);
});
