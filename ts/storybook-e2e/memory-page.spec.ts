import { test, expect, type Page } from "@playwright/test";
const open = async (p: Page, story = "default", select = true) => {
  await p.goto(`/iframe.html?id=pages-memory--${story}&viewMode=story`);
  if (
    select &&
    !["empty", "unavailable", "loading", "document"].includes(story)
  ) {
    const entry = list(p)
      .getByRole("link", {
        name:
          story === "migrated-documents" ? /runtime.md/ : /Architecture notes/,
      })
      .first();
    await entry.click();
  }
};
const list = (p: Page) =>
  p.getByRole("region", { name: "Project memories", exact: true });
const doc = (p: Page) =>
  p.getByRole("region", { name: "Memory document", exact: true });
async function source(p: Page, text: string) {
  const edit = p.getByRole("button", { name: "Edit", exact: true });
  if (await edit.isVisible()) await edit.click();
  const button = p.getByRole("button", { name: "Source", exact: true });
  if (await button.isVisible()) await button.click();
  const editor = p.locator(".monaco-editor textarea");
  await editor.focus();
  await p.keyboard.press("ControlOrMeta+A");
  await p.keyboard.insertText(text);
}
test("flat memories open directly without an editing-session context", async ({
  page,
}) => {
  await open(page);
  await expect(
    page.getByRole("heading", { name: "Architecture", exact: true }),
  ).toBeVisible();
  await expect(
    page.getByRole("combobox", { name: "Editing session" }),
  ).toHaveCount(0);
  await expect(
    page.getByRole("textbox", { name: "Memory name", exact: true }),
  ).toHaveCount(0);
  await expect(
    page.getByRole("button", { name: "Edit", exact: true }),
  ).toBeVisible();
  await expect(
    list(page).getByRole("link", { name: /Architecture notes/ }),
  ).toHaveAttribute("aria-current", "page");
  await list(page)
    .getByRole("link", { name: /Handoff notes/ })
    .click();
  await expect(
    page.getByRole("heading", { name: "Handoff", exact: true }),
  ).toBeVisible();
});
test("project selection scopes documents", async ({ page }) => {
  await open(page);
  await expect(doc(page)).toBeVisible();
  await page
    .getByRole("combobox", { name: "Select project", exact: true })
    .click();
  await page
    .getByRole("option", { name: "API workspace", exact: true })
    .click();
  await list(page)
    .getByRole("link", { name: /API notes/ })
    .click();
  await expect(
    page.getByRole("heading", { name: "API", exact: true }),
  ).toBeVisible();
  await expect(
    list(page).getByRole("link", { name: /Architecture notes/ }),
  ).toHaveCount(0);
});
test("an empty project creates and saves its first Memory", async ({
  page,
}) => {
  await open(page, "empty");
  await expect(
    page.getByText("No memories yet. Create a document to start.", {
      exact: true,
    }),
  ).toBeVisible();
  await page.getByRole("button", { name: "New memory", exact: true }).click();
  await page
    .getByRole("textbox", { name: "Memory name", exact: true })
    .fill("First decision");
  await source(page, "# First decision");
  await page.getByRole("button", { name: "Save", exact: true }).click();
  await expect(
    list(page).getByRole("link", { name: /First decision/ }),
  ).toBeVisible();
  await expect(
    page.getByRole("heading", { name: "First decision", exact: true }),
  ).toBeVisible();
  await expect(page.getByRole("main").getByRole("alert")).toHaveCount(0);
});
test("edits are guarded on project changes and metadata saves preserve body revision", async ({
  page,
}) => {
  await open(page);
  await expect(
    page.getByRole("heading", { name: "Architecture", exact: true }),
  ).toBeVisible();
  await page.getByRole("button", { name: "Edit", exact: true }).click();
  await page
    .getByRole("textbox", { name: "Memory name", exact: true })
    .fill("Renamed notes");
  await page.getByRole("combobox", { name: "Select project" }).click();
  await page
    .getByRole("option", { name: "API workspace", exact: true })
    .click();
  await expect(page.getByRole("dialog")).toContainText(
    "Discard unsaved changes",
  );
  await page
    .getByRole("dialog")
    .getByRole("button", { name: "Cancel", exact: true })
    .click();
  await expect(page.getByRole("dialog")).not.toBeVisible();
  await page.getByRole("button", { name: "Save", exact: true }).click();
  await expect(
    doc(page).getByText("Revision 1", { exact: true }),
  ).toBeVisible();
  await expect(
    list(page).getByRole("link", { name: /Renamed notes/ }),
  ).toBeVisible();
});
test("body edits create versions, show diff and restore as another version", async ({
  page,
}) => {
  await open(page);
  await expect(
    page.getByRole("heading", { name: "Architecture", exact: true }),
  ).toBeVisible();
  await source(page, "# Updated decision");
  await page.getByRole("button", { name: "Save", exact: true }).click();
  await expect(
    doc(page).getByText("Revision 2", { exact: true }),
  ).toBeVisible();
  await page.getByText("Version history", { exact: true }).click();
  await page.getByRole("button", { name: "r1", exact: true }).click();
  await expect(doc(page).locator("pre")).toContainText("Updated decision");
  await page
    .getByRole("button", { name: "Use this version", exact: true })
    .click();
  await expect(
    page.getByRole("button", { name: "Save", exact: true }),
  ).toBeEnabled();
  await page.getByRole("button", { name: "Save", exact: true }).click();
  await expect(
    doc(page).getByText("Revision 3", { exact: true }),
  ).toBeVisible();
  await expect(
    page.getByRole("heading", { name: "Architecture", exact: true }),
  ).toBeVisible();
});
test("revision conflicts preserve the draft and display the actual failure", async ({
  page,
}) => {
  await open(page, "revision-conflict");
  await expect(
    page.getByRole("heading", { name: "Architecture", exact: true }),
  ).toBeVisible();
  await source(page, "# Unsaved draft");
  await page.getByRole("button", { name: "Save", exact: true }).click();
  await expect(page.getByRole("main").getByRole("alert")).toContainText(
    "Memory revision changed",
  );
  await page.getByRole("button", { name: "Preview", exact: true }).click();
  await expect(
    page.getByRole("heading", { name: "Unsaved draft", exact: true }),
  ).toBeVisible();
  await expect(
    doc(page).getByText("Architecture notes · Unsaved", { exact: true }),
  ).toBeVisible();
});
test("label filters, trash restoration and purge use the flat resource", async ({
  page,
}) => {
  await open(page);
  await expect(doc(page)).toBeVisible();
  await page
    .getByRole("button", { name: "Filter labels", exact: true })
    .click();
  await page
    .getByRole("textbox", { name: "Filter labels", exact: true })
    .fill("topic=none");
  await page.getByRole("button", { name: "Search", exact: true }).click();
  await expect(
    page.getByText("No memories match your filters.", { exact: true }),
  ).toBeVisible();
  await page
    .getByRole("textbox", { name: "Filter labels", exact: true })
    .fill("topic=design");
  await page.getByRole("button", { name: "Search", exact: true }).click();
  await list(page)
    .getByRole("link", { name: /Architecture notes/ })
    .click();
  await expect(
    page.getByRole("heading", { name: "Architecture", exact: true }),
  ).toBeVisible();
  await list(page)
    .getByRole("link", { name: /Architecture notes/ })
    .click();
  await page
    .getByRole("button", { name: "Memory actions", exact: true })
    .click();
  await page.getByRole("menuitem", { name: "Delete", exact: true }).click();
  await page.getByRole("button", { name: "Confirm", exact: true }).click();
  await expect(page.getByRole("dialog")).not.toBeVisible();
  await expect(
    list(page).getByRole("link", { name: /Architecture notes/ }),
  ).toHaveCount(0);
  await page.getByRole("radio", { name: "Trash", exact: true }).check();
  await list(page)
    .getByRole("link", { name: /Architecture notes/ })
    .click();
  await expect(
    doc(page).getByRole("button", { name: "Restore", exact: true }),
  ).toBeVisible();
  await page.getByRole("button", { name: "Restore", exact: true }).click();
  await expect(
    page.getByText("No memories match your filters.", { exact: true }),
  ).toBeVisible();
  await page.getByRole("radio", { name: "Memories", exact: true }).check();
  await expect(
    list(page).getByRole("link", { name: /Architecture notes/ }),
  ).toBeVisible();
  await list(page)
    .getByRole("link", { name: /Architecture notes/ })
    .click();
  await page
    .getByRole("button", { name: "Memory actions", exact: true })
    .click();
  await page.getByRole("menuitem", { name: "Delete", exact: true }).click();
  await page
    .getByRole("dialog")
    .getByRole("button", { name: "Confirm", exact: true })
    .click();
  await page.getByRole("radio", { name: "Trash", exact: true }).check();
  await list(page)
    .getByRole("link", { name: /Architecture notes/ })
    .click();
  await page.getByRole("button", { name: "Purge", exact: true }).click();
  await expect(page.getByRole("dialog")).toContainText("all its versions");
  await page
    .getByRole("dialog")
    .getByRole("button", { name: "Confirm", exact: true })
    .click();
  await expect(
    page.getByText("No memories match your filters.", { exact: true }),
  ).toBeVisible();
  await page.getByRole("radio", { name: "Memories", exact: true }).check();
  await expect(
    list(page).getByRole("link", { name: /Architecture notes/ }),
  ).toHaveCount(0);
  await expect(page.getByRole("main").getByRole("alert")).toHaveCount(0);
});
test("memory access does not require an existing project session", async ({
  page,
}) => {
  await open(page, "no-sessions");
  await expect(
    page.getByRole("heading", { name: "Architecture", exact: true }),
  ).toBeVisible();
  await page.getByRole("button", { name: "New memory", exact: true }).click();
  await expect(
    page.getByRole("textbox", { name: "Memory name", exact: true }),
  ).toBeVisible();
  await expect(
    page.getByRole("combobox", { name: "Owner session", exact: true }),
  ).toHaveCount(0);
});
for (const [story, message] of [
  ["unavailable", "Memories could not be loaded."],
  ["loading", "Loading memories…"],
] as const)
  test(`${story} is explicit`, async ({ page }) => {
    await open(page, story);
    await expect(page.getByText(message, { exact: false })).toBeVisible();
    await expect(
      page.getByText("No memories yet. Create a document to start.", {
        exact: true,
      }),
    ).toHaveCount(0);
  });
for (const width of [390, 850, 1200])
  test(`flat Memory fits ${width}px`, async ({ page }) => {
    await page.setViewportSize({ width, height: 900 });
    await open(page);
    await expect(
      page.getByRole("heading", { name: "Architecture", exact: true }),
    ).toBeVisible();
    expect(
      await page.evaluate(
        () => document.documentElement.scrollWidth <= innerWidth,
      ),
    ).toBe(true);
    const left = (await list(page).boundingBox())!,
      right = (await doc(page).boundingBox())!;
    if (width === 1200) {
      expect(right.x - left.x - left.width).toBeCloseTo(10, 0);
      expect(right.y).toBeCloseTo(left.y, 0);
    } else if (width === 390)
      expect(right.y).toBeGreaterThan(left.y + left.height);
  });

test("reading and source inspection stay separate from editing, and Cancel preserves the document", async ({
  page,
}) => {
  await open(page);
  await expect(
    page.getByRole("heading", { name: "Architecture", exact: true }),
  ).toBeVisible();
  await expect(
    page.getByRole("textbox", { name: "Memory name", exact: true }),
  ).toHaveCount(0);
  await page.getByRole("button", { name: "Source", exact: true }).click();
  await expect(page.locator(".monaco-editor textarea")).toHaveJSProperty(
    "readOnly",
    true,
  );
  await page.getByRole("button", { name: "Edit", exact: true }).click();
  await expect(page.locator(".monaco-editor textarea")).toHaveJSProperty(
    "readOnly",
    false,
  );
  await page
    .getByRole("textbox", { name: "Memory name", exact: true })
    .fill("Discarded name");
  await page.getByRole("button", { name: "Cancel", exact: true }).click();
  await page
    .getByRole("dialog")
    .getByRole("button", { name: "Confirm", exact: true })
    .click();
  await expect(
    page.getByRole("heading", { name: "Architecture", exact: true }),
  ).toBeVisible();
  await expect(
    page.getByRole("textbox", { name: "Memory name", exact: true }),
  ).toHaveCount(0);
  await expect(
    list(page).getByRole("link", { name: /Discarded name/ }),
  ).toHaveCount(0);
});

test("migrated documents remain compact and preserve provenance on metadata edits", async ({
  page,
}) => {
  await page.setViewportSize({ width: 1200, height: 900 });
  await open(page, "migrated-documents");
  await expect(
    list(page)
      .getByRole("link", { name: /runtime.md/ })
      .first(),
  ).toBeVisible();
  await expect(list(page)).not.toContainText("legacy.");
  await expect(
    list(page)
      .getByRole("navigation", { name: "Memories", exact: true })
      .getByRole("link"),
  ).toHaveCount(26);
  await page.getByText("Document details", { exact: true }).click();
  await expect(
    doc(page).getByText("legacy.collection", { exact: true }),
  ).toBeVisible();
  await page.getByRole("button", { name: "Edit", exact: true }).click();
  await expect(
    page.getByRole("textbox", { name: "Memory labels", exact: true }),
  ).not.toHaveValue(/legacy\./);
  await page
    .getByRole("textbox", { name: "Memory name", exact: true })
    .fill("renamed-runtime.md");
  await page
    .getByRole("textbox", { name: "Memory labels", exact: true })
    .fill("topic=review");
  await page.getByRole("button", { name: "Save", exact: true }).click();
  await expect(
    doc(page).getByText("Revision 1", { exact: true }),
  ).toBeVisible();
  await page.getByText("Document details", { exact: true }).click();
  await expect(
    doc(page).getByText("legacy.collection", { exact: true }),
  ).toBeVisible();
  await expect(
    doc(page).getByText("topic=review", { exact: true }),
  ).toBeVisible();
  expect(await page.evaluate(() => document.documentElement.scrollWidth)).toBe(
    1200,
  );
});

test("version history closes on document selection instead of showing the previous resource history", async ({
  page,
}) => {
  await open(page);
  await expect(doc(page)).toBeVisible();
  await page.getByText("Version history", { exact: true }).click();
  await expect(
    page.getByRole("button", { name: "r1", exact: true }),
  ).toBeVisible();
  await list(page)
    .getByRole("link", { name: /Handoff notes/ })
    .click();
  const history = doc(page)
    .locator("details")
    .filter({ has: page.getByText("Version history", { exact: true }) });
  await expect(history).not.toHaveAttribute("open");
  await page.getByText("Version history", { exact: true }).click();
  await expect(
    page.getByRole("button", { name: "r1", exact: true }),
  ).toBeVisible();
});

test("restoring a version explicitly confirms replacement of a dirty draft", async ({
  page,
}) => {
  await open(page);
  await expect(
    page.getByRole("heading", { name: "Architecture", exact: true }),
  ).toBeVisible();
  await source(page, "# Version two");
  await page.getByRole("button", { name: "Save", exact: true }).click();
  await expect(
    doc(page).getByText("Revision 2", { exact: true }),
  ).toBeVisible();
  await source(page, "# Unsaved version three");
  await page.getByText("Version history", { exact: true }).click();
  await page.getByRole("button", { name: "r1", exact: true }).click();
  await page
    .getByRole("button", { name: "Use this version", exact: true })
    .click();
  await expect(page.getByRole("dialog")).toContainText(
    "Replace your unsaved changes",
  );
  await page
    .getByRole("dialog")
    .getByRole("button", { name: "Confirm", exact: true })
    .click();
  await expect(page.locator(".monaco-editor")).toContainText(
    "Shared memory belongs to the project.",
  );
  await expect(
    page.getByRole("button", { name: "Save", exact: true }),
  ).toBeEnabled();
});

test("the project collection shows links without opening a document", async ({
  page,
}) => {
  await open(page, "default", false);
  const entry = list(page).getByRole("link", { name: /Architecture notes/ });
  await expect(entry).toHaveAttribute(
    "href",
    /\/c\/_\/project\/web\/memory\/[\da-f-]{36}$/,
  );
  await expect(doc(page)).toHaveCount(0);
  await entry.click();
  await expect(
    page.getByRole("heading", { name: "Architecture", exact: true }),
  ).toBeVisible();
  await expect(entry).toHaveAttribute("aria-current", "page");
});
test("a document route opens its body directly", async ({ page }) => {
  await open(page, "document");
  await expect(
    page.getByRole("heading", { name: "Architecture", exact: true }),
  ).toBeVisible();
});
test("reselecting the open document preserves its draft", async ({ page }) => {
  await open(page);
  await page.getByRole("button", { name: "Edit", exact: true }).click();
  const name = page.getByRole("textbox", { name: "Memory name", exact: true });
  await name.fill("Keep this name");
  await list(page)
    .getByRole("link", { name: /Architecture notes/ })
    .click();
  await expect(name).toHaveValue("Keep this name");
  await expect(page.getByRole("dialog")).toHaveCount(0);
});
