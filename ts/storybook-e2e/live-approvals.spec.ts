import { expect, test } from "@playwright/test";

test("automatic approvals do not flash cards; unresolved approvals and questions stay actionable", async ({
  page,
}) => {
  const errors: string[] = [];
  page.on("pageerror", (error) => errors.push(error.message));
  await page.goto(
    "/iframe.html?id=conversation-questioncard--live-approvals&viewMode=story",
  );
  const group = page.getByRole("group", {
    name: "Pending approvals",
    exact: true,
  });
  await expect(group).toBeVisible();
  await expect(group).toHaveAttribute("data-pending-count", "0");
  await expect(group).toHaveAttribute("aria-busy", "false");
  await expect(
    group.getByRole("button", { name: "Deny", exact: true }),
  ).toBeDisabled();
  await expect(
    group.getByRole("button", { name: "Allow", exact: true }),
  ).toBeDisabled();
  const idleBounds = await group.boundingBox();
  await page.locator(".question-cards").waitFor({ state: "attached" });
  await page.evaluate(() => {
    const seen: string[] = [];
    (window as any).approvalFlashes = seen;
    const observer = new MutationObserver(() => {
      for (const card of document.querySelectorAll(
        ".question-cards .approval, [data-approval-stack], [role='group'][data-pending-count]:not([data-pending-count='0'])",
      ))
        seen.push(card.getAttribute("aria-label") ?? "approval");
    });
    observer.observe(document.querySelector(".conversation")!, {
      childList: true,
      subtree: true,
    });
  });
  await page
    .getByRole("button", { name: "Simulate automatic approvals" })
    .click();
  await page.waitForTimeout(1800);
  expect(await page.evaluate(() => (window as any).approvalFlashes)).toEqual(
    [],
  );
  await page
    .getByRole("button", { name: "Request approval", exact: true })
    .click();
  await expect(
    page.getByRole("region", {
      name: "item/commandExecution/requestApproval",
      exact: true,
    }),
  ).toBeVisible();
  expect(await group.boundingBox()).toEqual(idleBounds);
  await expect(
    page.getByRole("group", { name: "Pending approvals", exact: true }),
  ).toBeVisible();
  await page
    .getByRole("button", { name: "Resolve request", exact: true })
    .click();
  await expect(page.locator(".question-cards .approval")).toHaveCount(0);
  await expect(page.locator("[data-approval-stack]")).toHaveCount(0);
  await expect(
    group.getByRole("button", { name: "Deny", exact: true }),
  ).toBeDisabled();
  await expect(
    group.getByRole("button", { name: "Allow", exact: true }),
  ).toBeDisabled();
  await page.getByRole("button", { name: "Ask question", exact: true }).click();
  await expect(
    page.getByRole("region", { name: "Question", exact: true }),
  ).toBeVisible();
  await page
    .getByRole("button", { name: "Resolve request", exact: true })
    .click();
  await expect(page.locator(".question-cards .approval")).toHaveCount(0);
  expect(errors).toEqual([]);
});

for (const width of [1280, 390]) {
  test(`toolbar approvals stack independent requests and resolve one at a time at ${width}px`, async ({
    page,
  }, testInfo) => {
    await page.setViewportSize({ width, height: 850 });
    const errors: string[] = [];
    page.on("pageerror", (error) => errors.push(error.message));
    await page.goto(
      "/iframe.html?id=conversation-questioncard--live-approvals&viewMode=story",
    );
    const request = page.getByRole("button", {
      name: "Request approval",
      exact: true,
    });
    await request.click();
    await request.click();
    await request.click();
    const group = page.getByRole("group", {
      name: "Pending approvals",
      exact: true,
    });
    const stack = page.locator("[data-approval-stack]");
    const current = page.getByRole("region", {
      name: "item/commandExecution/requestApproval",
      exact: true,
    });
    await expect(group).toHaveAttribute("data-pending-count", "3");
    await expect(stack).toHaveAttribute("data-positioned", "true");
    await expect(stack.locator(":scope > [aria-hidden='true']")).toHaveCount(2);
    await expect(current).toContainText("git status --short");
    await expect(current).toContainText("/workspace");
    await expect(current.locator(".card-heading, .card-footer")).toHaveCount(0);
    await expect(
      group.getByRole("button", { name: "Deny", exact: true }),
    ).toBeEnabled();
    await expect(
      group.getByRole("button", { name: "Allow", exact: true }),
    ).toBeEnabled();
    const buttons = (await group.boundingBox())!;
    const menu = (await page
      .getByRole("button", { name: "Session menu", exact: true })
      .boundingBox())!;
    const cards = (await stack.boundingBox())!;
    const conversation = (await page.locator(".conversation").boundingBox())!;
    expect(buttons.x + buttons.width).toBeLessThanOrEqual(menu.x);
    expect(cards.y + cards.height).toBeLessThan(buttons.y);
    expect(cards.width).toBeLessThanOrEqual(380);
    expect(cards.x).toBeGreaterThanOrEqual(conversation.x);
    expect(cards.x + cards.width).toBeLessThanOrEqual(
      conversation.x + conversation.width,
    );
    if (width === 1280)
      expect(cards.x + cards.width / 2).toBeCloseTo(
        buttons.x + buttons.width / 2,
        0,
      );
    await current.evaluate(async (node) => {
      await Promise.all(
        node
          .getAnimations({ subtree: true })
          .filter((a) => a.effect?.getTiming().iterations !== Infinity)
          .map((a) => a.finished.catch(() => {})),
      );
    });
    await page.screenshot({
      path: testInfo.outputPath("stacked-approvals.png"),
    });
    await current.locator("pre code").evaluate((node) => {
      node.textContent =
        "printf 'A long permission request stays inside the card'\n".repeat(80);
    });
    await expect
      .poll(async () => (await current.boundingBox())!.height)
      .toBeLessThanOrEqual(360);
    await current.hover();
    await expect(
      current.getByRole("scrollbar", { name: "Request details", exact: true }),
    ).toBeVisible();
    expect((await stack.boundingBox())!.y).toBeGreaterThanOrEqual(
      conversation.y,
    );
    await group.getByRole("button", { name: "Allow", exact: true }).click();
    await expect(group).toHaveAttribute("data-pending-count", "2");
    await expect(current).toContainText("npm run build");
    await group.getByRole("button", { name: "Deny", exact: true }).click();
    await expect(group).toHaveAttribute("data-pending-count", "1");
    await expect(current).toContainText("git diff --stat");
    await expect(stack.locator(":scope > [aria-hidden='true']")).toHaveCount(0);
    await group.getByRole("button", { name: "Allow", exact: true }).focus();
    await page.keyboard.press("Enter");
    await expect(group).toHaveAttribute("data-pending-count", "0");
    await expect(
      group.getByRole("button", { name: "Allow", exact: true }),
    ).toBeDisabled();
    await expect(
      group.getByRole("button", { name: "Deny", exact: true }),
    ).toBeDisabled();
    await expect(stack).toHaveCount(0);
    expect(
      await page.evaluate(
        () => document.documentElement.scrollWidth <= innerWidth,
      ),
    ).toBe(true);
    expect(errors).toEqual([]);
  });
}

test("permission decisions preserve a separate pending Question", async ({
  page,
}) => {
  await page.goto(
    "/iframe.html?id=conversation-questioncard--live-approvals&viewMode=story",
  );
  await page
    .getByRole("button", { name: "Request approval", exact: true })
    .click();
  const group = page.getByRole("group", {
    name: "Pending approvals",
    exact: true,
  });
  await expect(group).toBeVisible();
  await page.getByRole("button", { name: "Ask question", exact: true }).click();
  const question = page.getByRole("region", { name: "Question", exact: true });
  await expect(question).toBeVisible();
  await group.getByRole("button", { name: "Allow", exact: true }).click();
  await expect(group).toHaveAttribute("data-pending-count", "0");
  await expect(
    group.getByRole("button", { name: "Allow", exact: true }),
  ).toBeDisabled();
  await expect(question).toBeVisible();
  await question.getByRole("radio", { name: /^Conversation / }).check();
  await question.getByRole("button", { name: "Submit", exact: true }).click();
  await expect(question).toHaveCount(0);
});

test("a busy approval keeps the request visible and both decisions disabled", async ({
  page,
}) => {
  await page.goto(
    "/iframe.html?id=conversation-questioncard--live-approvals&viewMode=story&args=busy:true",
  );
  await page
    .getByRole("button", { name: "Request approval", exact: true })
    .click();
  const group = page.getByRole("group", {
    name: "Pending approvals",
    exact: true,
  });
  await expect(group).toHaveAttribute("aria-busy", "true");
  await expect(
    group.getByRole("button", { name: "Allow", exact: true }),
  ).toBeDisabled();
  await expect(
    group.getByRole("button", { name: "Deny", exact: true }),
  ).toBeDisabled();
  await expect(
    page.getByRole("region", {
      name: "item/commandExecution/requestApproval",
      exact: true,
    }),
  ).toBeVisible();
});
