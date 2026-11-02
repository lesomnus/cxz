import { test, expect, type Page } from "@playwright/test";
const open = (page: Page, story = "default") =>
  page.goto(`/iframe.html?id=pages-memory--${story}&viewMode=story`);
const memories = (page: Page) =>
  page.getByRole("region", { name: "Project memories", exact: true });
const documents = (page: Page) =>
  page.getByRole("region", { name: "Memory documents", exact: true });
async function edit(page: Page, text: string) {
  await page.getByRole("button", { name: "Source", exact: true }).click();
  const editor = page.locator(".monaco-editor textarea");
  await editor.focus();
  await page.keyboard.press("ControlOrMeta+A");
  await page.keyboard.insertText(text);
}

test("a new editing session shows populated project memories without reading a nonexistent default", async ({
  page,
}) => {
  await open(page);
  await expect(
    memories(page).getByRole("button", { name: /Architecture notes/ }),
  ).toHaveAttribute("aria-pressed", "true");
  await expect(
    memories(page).getByRole("button", { name: /New session/ }),
  ).toBeVisible();
  await expect(page.getByRole("main").getByRole("alert")).toHaveCount(0);
  await documents(page)
    .getByRole("button", { name: "notes.md", exact: true })
    .click();
  await expect(
    page.getByRole("heading", { name: "Architecture", exact: true }),
  ).toBeVisible();
  await expect(
    page.getByRole("button", { name: "Save", exact: true }),
  ).toBeDisabled();
  await memories(page)
    .getByRole("button", { name: /New session/ })
    .click();
  await expect(
    page.getByText("This memory has no documents.", { exact: true }),
  ).toBeVisible();
});

test("project selection scopes the memory list and editing context", async ({
  page,
}) => {
  await open(page);
  await expect(
    documents(page).getByRole("button", { name: "notes.md", exact: true }),
  ).toBeVisible();
  await page
    .getByRole("combobox", { name: "Select project", exact: true })
    .click();
  await page
    .getByRole("option", { name: "API workspace", exact: true })
    .click();
  await expect(
    memories(page).getByRole("button", { name: /API notes/ }),
  ).toBeVisible();
  await expect(
    memories(page).getByRole("button", { name: /Architecture notes/ }),
  ).toHaveCount(0);
  await documents(page)
    .getByRole("button", { name: "notes.md", exact: true })
    .click();
  await expect(
    page.getByRole("heading", { name: "API", exact: true }),
  ).toBeVisible();
  await expect(
    page.getByRole("combobox", { name: "Editing session" }),
  ).toContainText("pine");
});

test("empty project memory can create and save its first document", async ({
  page,
}) => {
  await open(page, "empty");
  await expect(page.getByText(/No shared memories yet/)).toBeVisible();
  await page
    .getByRole("button", { name: "This session's memory", exact: true })
    .click();
  await page
    .getByRole("textbox", { name: "New document name" })
    .fill("decisions.md");
  await page.getByRole("button", { name: "New document", exact: true }).click();
  const editor = page.locator(".monaco-editor textarea");
  await editor.focus();
  await page.keyboard.insertText("# First decision");
  await page.getByRole("button", { name: "Save", exact: true }).click();
  await expect(
    documents(page).getByRole("button", { name: "decisions.md", exact: true }),
  ).toBeVisible();
  await page.getByRole("button", { name: "Preview", exact: true }).click();
  await expect(
    page.getByRole("heading", { name: "First decision", exact: true }),
  ).toBeVisible();
  await expect(page.getByRole("main").getByRole("alert")).toHaveCount(0);
});

test("forked memory supports editing and protects drafts on project changes", async ({
  page,
}) => {
  await open(page);
  await documents(page)
    .getByRole("button", { name: "notes.md", exact: true })
    .waitFor();
  await page.getByText("Memory actions", { exact: true }).click();
  await page
    .getByRole("button", { name: "Fork into this session", exact: true })
    .click();
  await expect(
    documents(page).getByText("Forked notes", { exact: true }),
  ).toBeVisible();
  await documents(page)
    .getByRole("button", { name: "notes.md", exact: true })
    .click();
  await edit(page, "# Local draft");
  await expect(
    page.getByRole("button", { name: "Save", exact: true }),
  ).toBeEnabled();
  await page
    .getByRole("combobox", { name: "Select project", exact: true })
    .click();
  await page
    .getByRole("option", { name: "API workspace", exact: true })
    .click();
  await expect(page.getByRole("dialog")).toContainText(
    "Discard unsaved changes",
  );
  await page.getByRole("button", { name: "Cancel", exact: true }).click();
  await expect(page.getByRole("dialog")).not.toBeVisible();
  await page.getByRole("button", { name: "Save", exact: true }).click();
  await expect(
    page.getByRole("button", { name: "Save", exact: true }),
  ).toBeDisabled();
  await page.getByRole("button", { name: "Preview", exact: true }).click();
  await expect(
    page.getByRole("heading", { name: "Local draft", exact: true }),
  ).toBeVisible();
});

test("revision conflicts keep the draft and explain the save failure", async ({
  page,
}) => {
  await open(page, "revision-conflict");
  await page.getByText("Memory actions", { exact: true }).click();
  await page
    .getByRole("button", { name: "Fork into this session", exact: true })
    .click();
  await expect(
    documents(page).getByText("Forked notes", { exact: true }),
  ).toBeVisible();
  await documents(page)
    .getByRole("button", { name: "notes.md", exact: true })
    .click();
  await edit(page, "# Unsaved draft");
  await page.getByRole("button", { name: "Save", exact: true }).click();
  await expect(page.getByRole("main").getByRole("alert")).toContainText(
    "Document revision changed",
  );
  await page.getByRole("button", { name: "Preview", exact: true }).click();
  await expect(
    page.getByRole("heading", { name: "Unsaved draft", exact: true }),
  ).toBeVisible();
  await expect(
    page.getByText("notes.md · Unsaved", { exact: true }),
  ).toBeVisible();
});

test("search, snapshots and trash distinguish empty results and retain restoration", async ({
  page,
}) => {
  await open(page);
  await page
    .getByRole("textbox", { name: "Search shared memory" })
    .fill("no-such-memory");
  await page.getByRole("button", { name: "Search", exact: true }).click();
  await expect(
    page.getByText("No memories match your search.", { exact: true }),
  ).toBeVisible();
  await page.getByRole("textbox", { name: "Search shared memory" }).fill("");
  await page.getByRole("button", { name: "Search", exact: true }).click();
  await memories(page)
    .getByRole("button", { name: /Saved checkpoint/ })
    .click();
  await documents(page)
    .getByRole("button", { name: "notes.md", exact: true })
    .click();
  await expect(
    page.getByRole("heading", { name: "Saved checkpoint", exact: true }),
  ).toBeVisible();
  await expect(
    page.getByRole("button", { name: "Save", exact: true }),
  ).toBeDisabled();
  await page.getByText("Memory actions", { exact: true }).click();
  await memories(page)
    .getByRole("button", { name: "Delete", exact: true })
    .click();
  await page.getByRole("button", { name: "Confirm", exact: true }).click();
  await expect(page.getByRole("dialog")).not.toBeVisible();
  await expect(
    memories(page).getByRole("button", { name: /Saved checkpoint/ }),
  ).toHaveCount(0);
  await page.getByRole("radio", { name: "Trash", exact: true }).check();
  await memories(page)
    .getByRole("button", { name: /Saved checkpoint/ })
    .click();
  await memories(page)
    .getByRole("button", { name: "Restore", exact: true })
    .click();
  await expect(
    memories(page).getByRole("button", { name: /Saved checkpoint/ }),
  ).toHaveCount(0);
  await expect(page.getByRole("main").getByRole("alert")).toHaveCount(0);
  await page.getByRole("radio", { name: "Library", exact: true }).check();
  await expect(
    memories(page).getByRole("button", { name: /Saved checkpoint/ }),
  ).toBeVisible();
});

for (const [story, message] of [
  ["unavailable", "Memories could not be loaded."],
  ["loading", "Loading memories…"],
  ["no-sessions", "No session available for this project"],
] as const) {
  test(`${story} has an explicit state`, async ({ page }) => {
    await open(page, story);
    await expect(page.getByText(message, { exact: false })).toBeVisible();
    await expect(page.getByText(/No shared memories yet/)).toHaveCount(0);
  });
}
for (const width of [390, 850, 1200]) {
  test(`memory panels fit ${width}px without page overflow`, async ({
    page,
  }) => {
    await page.setViewportSize({ width, height: 900 });
    await open(page);
    await expect(
      documents(page).getByRole("button", { name: "notes.md", exact: true }),
    ).toBeVisible();
    expect(
      await page.evaluate(
        () => document.documentElement.scrollWidth <= innerWidth,
      ),
    ).toBe(true);
    const left = (await memories(page).boundingBox())!;
    const right = (await documents(page).boundingBox())!;
    if (width === 1200) {
      expect(right.x - left.x - left.width).toBeCloseTo(10, 0);
      expect(left.y).toBeCloseTo(right.y, 0);
    } else if (width === 390)
      expect(right.y).toBeGreaterThan(left.y + left.height);
  });
}
