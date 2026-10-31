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

test("account login refreshes status and logout uses confirmation", async ({
  page,
}) => {
  await open(page, "account-login");
  await page.getByRole("button", { name: "Log in", exact: true }).click();
  await finishLogin(page);
  await expect(page.getByText("Signed in", { exact: true })).toBeVisible();
  await expect(
    page.getByRole("button", { name: "Log in", exact: true }),
  ).toBeDisabled();
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

for (const story of ["login-failure-and-retry", "login-connection-closed"]) {
  test(`${story} starts a fresh successful login after retry`, async ({
    page,
  }) => {
    await open(page, story);
    await page.getByRole("button", { name: "Log in", exact: true }).click();
    const panel = loginPanel(page);
    await expect(panel.getByRole("alert")).toBeVisible();
    await expect(
      page.getByRole("button", { name: "Log out", exact: true }),
    ).toBeDisabled();
    await panel
      .getByRole("button", { name: "Retry login", exact: true })
      .click();
    await finishLogin(page);
    await expect(page.getByText("Signed in", { exact: true })).toBeVisible();
  });
}

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
  await page
    .getByRole("button", { name: "Refresh status", exact: true })
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
