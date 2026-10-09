import { expect, test, devices } from "@playwright/test";

test.use({
  ...devices["Desktop Chrome"],
  viewport: { width: 1440, height: 1000 },
});

test.beforeEach(async ({ page }) => {
  await page.goto("/sandbox.html");
  await expect(
    page.getByRole("textbox", { name: "Message", exact: true }),
  ).toBeVisible({ timeout: 45000 });
  await expect(
    page.getByRole("combobox", { name: "Model", exact: true }),
  ).toBeEnabled();
});

test("suggestions cover only the text area, arrows preserve the caret and Right commits with Undo", async ({
  page,
}) => {
  const input = page.getByRole("textbox", { name: "Message", exact: true });
  await input.pressSequentially("/");
  const list = page.getByRole("listbox", {
    name: "Command suggestions",
    exact: true,
  });
  const selected = list.locator('[aria-selected="true"] .command-name');
  await expect(list.getByRole("option")).toHaveCount(7);
  await expect(selected).toHaveText("/model");
  const geometry = await input.evaluate((el: HTMLTextAreaElement) => {
    const overlay = document.querySelector<HTMLElement>(
      ".command-suggestions",
    )!;
    const current = overlay.querySelector<HTMLElement>(
      '[data-current="true"]',
    )!;
    const text = el.getBoundingClientRect(),
      root = overlay.getBoundingClientRect(),
      row = current.getBoundingClientRect();
    const style = getComputedStyle(el),
      height = parseFloat(style.lineHeight);
    const overlayStyle = getComputedStyle(overlay);
    return {
      textX: text.x,
      rootX: root.x,
      rootWidth: root.width,
      textWidth: el.clientWidth,
      topGap: row.y - root.y,
      rootHeight: root.height,
      rowHeight: height,
      overlayTopPadding: parseFloat(overlayStyle.paddingTop),
      overlayBottomPadding: parseFloat(overlayStyle.paddingBottom),
      baseline: row.y - text.y,
      padding: parseFloat(style.paddingTop),
      caret: el.selectionStart,
    };
  });
  expect(geometry.rootX).toBe(geometry.textX);
  expect(geometry.rootWidth).toBe(geometry.textWidth);
  expect(geometry.topGap).toBe(
    geometry.rowHeight * 3 + geometry.overlayTopPadding,
  );
  expect(geometry.rootHeight).toBe(
    geometry.rowHeight * 7 +
      geometry.overlayTopPadding +
      geometry.overlayBottomPadding,
  );
  expect(geometry.baseline).toBe(geometry.padding);
  await input.press("ArrowDown");
  await expect(selected).toHaveText("/effort");
  await expect(input).toHaveValue("/");
  expect(
    await input.evaluate((el: HTMLTextAreaElement) => el.selectionStart),
  ).toBe(geometry.caret);
  await input.press("ArrowUp");
  await expect(selected).toHaveText("/model");
  await page.screenshot({
    path: "test-results/command-suggestions-desktop.png",
  });
  await input.press("ArrowRight");
  await expect(input).toHaveValue("/model");
  await expect(list).toHaveCount(0);
  await expect(input).toBeFocused();
  await input.press("Control+z");
  await expect(input).toHaveValue("/");
  // Native Undo restores its replacement selection; collapse it before browsing.
  await input.press("End");
  await expect(list).toBeVisible();
});

test("fuzzy model hints use reported choices and submitting them applies the setting", async ({
  page,
}) => {
  const input = page.getByRole("textbox", { name: "Message", exact: true });
  await input.fill("/mdl");
  const list = page.getByRole("listbox", {
    name: "Command suggestions",
    exact: true,
  });
  const selected = list.locator('[aria-selected="true"] .command-name');
  await expect(selected).toHaveText("/model");
  await expect(
    list.getByRole("option", { name: /compact the current context/i }),
  ).toHaveCount(0);
  await input.press("ArrowRight");
  await input.pressSequentially(" sandbox-claude-co");
  await expect(selected).toHaveText("/model sandbox-claude-compact");
  await input.press("ArrowRight");
  await input.press("Control+Enter");
  await expect(input).toHaveValue("");
  await expect(page.locator(".model-field .meta-value")).toHaveText(
    "sandbox-claude-compact",
  );
  await expect(
    page
      .locator("article.input")
      .filter({ hasText: "/model sandbox-claude-compact" }),
  ).toHaveCount(0);
  await input.fill("/effort");
  await input.press("Control+Enter");
  await expect(
    page.getByRole("listbox", { name: "Effort choices", exact: true }),
  ).toBeVisible();
});

test("mobile suggestions preserve multiline text, dismiss on Escape and leave Other editors alone", async ({
  page,
}) => {
  await page.setViewportSize({ width: 390, height: 844 });
  const input = page.getByRole("textbox", { name: "Message", exact: true });
  const list = page.getByRole("listbox", {
    name: "Command suggestions",
    exact: true,
  });
  await input.fill("/mdl\nKeep this body");
  await input.evaluate((el: HTMLTextAreaElement) => el.setSelectionRange(4, 4));
  await expect(list).toBeVisible();
  await input.press("ArrowRight");
  await expect(input).toHaveValue("/model\nKeep this body");
  await page.getByLabel("Scenario", { exact: true }).selectOption("session-4");
  await expect(
    page.getByText("Which environment?", { exact: true }),
  ).toBeVisible();
  await input.fill("/");
  await expect(list).toBeVisible();
  const bounds = (await list.boundingBox())!;
  expect(bounds.x + bounds.width).toBeLessThanOrEqual(390);
  await page.screenshot({
    path: "test-results/command-suggestions-mobile.png",
  });
  await input.press("Escape");
  await expect(list).toHaveCount(0);
  await expect(page.locator(".stop")).toHaveAttribute("data-armed", "false");
  await input.press("Escape");
  await expect(page.locator(".stop")).toHaveAttribute("data-armed", "true");
  const other = page.getByRole("textbox", {
    name: "Other answer: Which environment?",
    exact: true,
  });
  await other.fill("/");
  await expect(list).toHaveCount(0);
});

test("unaccepted hints never replace the submitted prompt or its send animation", async ({
  page,
}) => {
  const input = page.getByRole("textbox", { name: "Message", exact: true });
  await input.fill("/co");
  await expect(
    page.locator('.command-suggestion[aria-selected="true"] .command-name'),
  ).toHaveText("/compact");
  await page.evaluate(() => {
    const observer = new MutationObserver(() => {
      const ghost = document.querySelector<HTMLElement>(".composer-send-ghost");
      if (!ghost) return;
      (window as typeof window & { commandGhost?: unknown }).commandGhost = {
        text: ghost.querySelector(".editor-line")?.textContent,
        hints: ghost.querySelectorAll(".command-suggestions").length,
        commandOpen: ghost.hasAttribute("data-command-open"),
      };
      observer.disconnect();
    });
    observer.observe(document.querySelector(".composer-input")!, {
      childList: true,
    });
  });
  await input.press("Control+Enter");
  await expect
    .poll(() =>
      page.evaluate(
        () =>
          (window as typeof window & { commandGhost?: unknown }).commandGhost,
      ),
    )
    .toEqual({ text: "/co", hints: 0, commandOpen: false });
  await expect(
    page.locator("article.input .message-body").filter({ hasText: /^\/co$/ }),
  ).toBeVisible();
  await expect(input).toHaveValue("");
});
