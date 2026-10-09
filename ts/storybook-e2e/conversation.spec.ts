import { expect, test, type Page } from "@playwright/test";

async function story(page: Page, id: string, globals = "") {
  const errors: string[] = [];
  page.on("pageerror", (error) => errors.push(error.message));
  await page.goto(
    `/iframe.html?id=${id}&viewMode=story${globals ? `&globals=${globals}` : ""}`,
  );
  await expect
    .poll(
      () =>
        page
          .locator("#storybook-root")
          .evaluate((node) => node.childNodes.length),
      { message: id },
    )
    .toBeGreaterThan(0);
  return errors;
}

test("send acknowledges, creates one input card and a final response, then resets", async ({
  page,
}) => {
  const errors = await story(
    page,
    "conversation-playground--slow-acknowledgement",
  );
  const editor = page.getByRole("textbox", { name: "Message", exact: true });
  const send = page.getByRole("button", { name: "Send", exact: true });
  await editor.fill("Check the shared Storybook conversation.");
  await send.click();
  await expect(editor).toHaveAttribute("readonly", "");
  await expect(page.locator(".composer-input")).toHaveAttribute(
    "data-sending",
    "true",
  );
  await expect(editor).not.toHaveAttribute("readonly", "");
  await expect(editor).toHaveValue("");
  const submitted = page
    .locator("article.input")
    .filter({ hasText: "Check the shared Storybook conversation." });
  await expect(submitted).toHaveCount(1);
  await expect(
    page
      .locator("article.response")
      .filter({ hasText: "Message received in the Storybook preview." }),
  ).toHaveCount(1);
  await expect(
    page.getByRole("button", { name: "Stop response", exact: true }),
  ).toBeDisabled();
  await expect(send).toBeDisabled();
  await editor.fill("A second message via keyboard.");
  await editor.press("Control+Enter");
  await expect(
    page
      .locator("article.input")
      .filter({ hasText: "A second message via keyboard." }),
  ).toHaveCount(1);
  // Reset during an active request also cancels its delayed response.
  await page.getByRole("button", { name: "Reset preview" }).click();
  await expect(editor).toHaveValue("");
  await expect(page.locator("article.input")).toHaveCount(1);
  await expect(page.locator("article.response")).toHaveCount(1);
  expect(errors).toEqual([]);
});

test("paste chips preview and send their expanded contents", async ({
  page,
}) => {
  const errors = await story(page, "conversation-playground--interactive");
  const editor = page.getByRole("textbox", { name: "Message", exact: true });
  const text =
    "const one = 1;\nconst two = 2;\nconst three = 3;\nconsole.log(one + two + three);";
  await editor.focus();
  await editor.evaluate((node, body) => {
    const clipboardData = new DataTransfer();
    clipboardData.setData("text/plain", body);
    node.dispatchEvent(
      new ClipboardEvent("paste", {
        clipboardData,
        bubbles: true,
        cancelable: true,
      }),
    );
  }, text);
  await expect(editor).toHaveValue(/\[Paste .*4L/);
  await page.locator(".paste-chip").click();
  await expect(page.getByRole("dialog")).toContainText("const three = 3;");
  await page.getByRole("button", { name: "Close paste preview" }).click();
  await page.getByRole("button", { name: "Send", exact: true }).click();
  await expect(
    page.locator("article.input").filter({ hasText: "const three = 3;" }),
  ).toHaveCount(1);
  await expect(
    page.locator("article.input").filter({ hasText: "const three = 3;" }),
  ).not.toContainText("[Paste");
  expect(errors).toEqual([]);
});

test("tool detail tabs load Monaco and dismiss on transcript clicks", async ({
  page,
}) => {
  const errors = await story(page, "conversation-eventcards--completed-tool");
  const tool = page.locator(".tool-activity");
  await tool.click();
  await expect(page.getByRole("dialog")).toHaveCount(0);
  await tool.dblclick();
  await expect(page.getByRole("dialog")).toBeVisible();
  await expect(page.locator(".monaco-editor")).toBeVisible();
  await page.getByRole("tab", { name: "Output", exact: true }).click();
  await expect(page.getByRole("tabpanel")).toContainText("main");
  await expect(page.getByRole("tabpanel")).toHaveAttribute(
    "data-language",
    /.+/,
  );
  await expect(page.locator(".detail-copy")).toBeVisible();
  await page.locator(".transcript").click({ position: { x: 30, y: 250 } });
  await expect(page.getByRole("dialog")).toHaveCount(0);
  expect(errors).toEqual([]);
});

test("question remains until explicitly answered and reuses the multiline editor", async ({
  page,
}) => {
  const errors = await story(page, "conversation-questioncard--question");
  const question = page.getByRole("region", {
    name: "item/tool/requestUserInput",
    exact: true,
  });
  await expect(question).toBeVisible();
  await page.locator(".transcript").click({ position: { x: 30, y: 30 } });
  await expect(question).toBeVisible();
  const other = page.getByRole("textbox", {
    name: "Other answer: Which preview should we build?",
  });
  await other.fill("A combined preview\nwith multiple lines");
  await expect(question.locator(".editor-gutter")).toContainText("2");
  await page.getByRole("button", { name: "Submit answers" }).click();
  await expect(question).toHaveCount(0);
  await expect(page.getByRole("status")).toContainText("A combined preview");
  expect(errors).toEqual([]);
});

test("session navigation highlights rows and long-list scrolling stays inside the panel", async ({
  page,
}) => {
  const errors = await story(page, "sessions-panel--long-list");
  const selected = page.locator(".tree-session.active");
  await expect(selected).toHaveAttribute("href", "/sessions/session-0");
  await page.locator('.tree-session[href="/sessions/session-1"]').click();
  await expect(selected).toHaveAttribute("href", "/sessions/session-1");
  const scrollbar = page.getByRole("scrollbar", { name: "Panel scroll" });
  await scrollbar.focus();
  await scrollbar.press("End");
  await expect
    .poll(() =>
      page.locator(".panel-scroll-content").evaluate((el) => el.scrollTop),
    )
    .toBeGreaterThan(0);
  await expect(page.locator(".resource-panel h2")).toBeVisible();
  expect(errors).toEqual([]);
});

test("light toolbar global reaches code blocks", async ({ page }) => {
  const errors = await story(
    page,
    "conversation-composer--code-block",
    "theme:light",
  );
  await expect(page.locator("html")).toHaveAttribute("data-theme", "light");
  await expect(page.locator(".composer-editor")).toBeVisible();
  await expect(page.locator(".editor-code-header")).toBeVisible();
  expect(errors).toEqual([]);
});

test("slash commands and Markdown editing use the shared editor behavior", async ({
  page,
}) => {
  const errors = await story(page, "conversation-playground--interactive");
  const editor = page.getByRole("textbox", { name: "Message", exact: true });
  await editor.fill("/");
  await editor.press("End");
  await expect(page.locator(".command-suggestions")).toBeVisible();
  await editor.press("ArrowDown");
  await editor.press("ArrowRight");
  await expect(editor).toHaveValue(/^\/\w+/);
  await editor.fill("- First item");
  await editor.press("End");
  await editor.press("Enter");
  await expect(editor).toHaveValue("- First item\n- ");
  await editor.fill("");
  await editor.pressSequentially("```");
  await expect(editor).toHaveValue("```\n\n```");
  await editor.press("Tab");
  await expect(editor).toHaveValue("```\n  \n```");
  expect(errors).toEqual([]);
});

test("major component stories render without runtime errors", async ({
  page,
  request,
}) => {
  test.setTimeout(90000);
  const index = await (await request.get("/index.json")).json();
  const entries = Object.values(index.entries) as {
    id: string;
    type: string;
  }[];
  for (const entry of entries.filter((entry) => entry.type === "story")) {
    const errors = await story(page, entry.id);
    // Wait for component mount rather than just Storybook's loading shell.
    await expect(page.locator("#storybook-root"), entry.id).toBeVisible();
    if (entry.id.startsWith("editor-sourceeditor--")) {
      await expect(page.locator(".monaco-editor"), entry.id).toBeVisible();
      await expect
        .poll(
          () =>
            page
              .locator(".code-editor")
              .evaluate(
                (el) => el.clientHeight / el.parentElement!.clientHeight,
              ),
          { message: entry.id },
        )
        .toBeGreaterThan(0.95);
    }
    if (entry.id.startsWith("components-floatingcard--")) {
      await expect(page.locator(".floating-card"), entry.id).toHaveCSS(
        "opacity",
        "1",
      );
      await expect(page.locator(".floating-card"), entry.id).toBeInViewport();
    }
    await expect(page.locator(".sb-errordisplay"), entry.id).toBeHidden();
    expect(errors, entry.id).toEqual([]);
    page.removeAllListeners("pageerror");
  }
});

test("double Escape stops the simulated response without retaining the accepted draft", async ({
  page,
}) => {
  const errors = await story(page, "conversation-playground--interactive");
  const editor = page.getByRole("textbox", { name: "Message", exact: true });
  const stop = page.getByRole("button", { name: "Stop response", exact: true });
  await editor.fill("Stop this preview turn.");
  await page.getByRole("button", { name: "Send", exact: true }).click();
  await expect(stop).toBeEnabled();
  await page.keyboard.press("Escape");
  await page.keyboard.press("Escape");
  await expect(stop).toBeDisabled();
  await expect(editor).toHaveValue("");
  await expect(
    page
      .locator("article.input")
      .filter({ hasText: "Stop this preview turn." }),
  ).toHaveCount(1);
  await expect(
    page
      .locator("article.response")
      .filter({ hasText: "Message received in the Storybook preview." }),
  ).toHaveCount(0);
  expect(errors).toEqual([]);
});

test("long conversation uses virtual rows and the real transcript scrollbar", async ({
  page,
}) => {
  const errors = await story(
    page,
    "conversation-playground--long-conversation",
  );
  const pane = page.locator(".transcript");
  const scroll = page.getByRole("scrollbar", { name: "Conversation scroll" });
  await expect(scroll).toBeAttached();
  await expect
    .poll(() => page.locator(".transcript-row").count())
    .toBeGreaterThan(0);
  expect(await page.locator(".transcript-row").count()).toBeLessThan(180);
  await scroll.focus();
  await scroll.press("Home");
  await expect.poll(() => pane.evaluate((el) => el.scrollTop)).toBe(0);
  await expect(page.locator("article.input").first()).toContainText("UI · 1.");
  expect(errors).toEqual([]);
});

test("standalone floating cards are opaque, readable and interactive", async ({
  page,
}) => {
  const errors = await story(page, "components-floatingcard--paste-preview");
  const card = page.locator(".floating-card");
  await expect(card).toHaveCSS("opacity", "1");
  await expect(card).toBeInViewport();
  await expect(card.getByText("Paste preview", { exact: true })).toBeVisible();
  await expect(card.locator("pre")).toContainText("export const ready = true;");
  await expect(
    card.getByRole("button", { name: "Close paste preview" }),
  ).toBeEnabled();
  await page.goto(
    "/iframe.html?id=components-floatingcard--editor-tabs&viewMode=story",
  );
  await expect(page.locator(".floating-card")).toHaveCSS("opacity", "1");
  const viewport = page.locator(".detail-editor");
  await expect
    .poll(() => viewport.evaluate((el) => el.clientHeight))
    .toBeGreaterThan(100);
  await page.getByRole("tab", { name: "Output", exact: true }).click();
  await expect(page.getByRole("tabpanel")).toContainText("main");
  expect(errors).toEqual([]);
});

test("source editor fills its preview and accepts edits", async ({ page }) => {
  const errors = await story(page, "editor-sourceeditor--settings-json");
  await expect(page.locator(".monaco-editor")).toBeVisible();
  const editor = page.locator(".code-editor");
  await expect
    .poll(() =>
      editor.evaluate((el) => el.clientHeight / el.parentElement!.clientHeight),
    )
    .toBeGreaterThan(0.95);
  await expect(page.locator(".view-lines")).toContainText('"editor.tabSize"');
  // Monaco's visible text layer receives clicks above its native textarea.
  await page.locator(".monaco-editor").click({ position: { x: 130, y: 24 } });
  await expect(
    page.getByRole("textbox", { name: "Settings file", exact: true }),
  ).toBeFocused();
  await page.keyboard.press("Control+A");
  await page.keyboard.type('{"editor.tabSize": 6}');
  await expect(page.locator(".view-lines")).toContainText(
    '"editor.tabSize": 6',
  );
  expect(errors).toEqual([]);
});
