import { test, expect, type Page } from "@playwright/test";
const open = (p: Page, story = "default") =>
  p.goto(`/iframe.html?id=pages-memory--${story}&viewMode=story`);
const list = (p: Page) =>
  p.getByRole("region", { name: "Project memories", exact: true });
const doc = (p: Page) =>
  p.getByRole("region", { name: "Memory document", exact: true });
async function source(p: Page, text: string) {
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
    list(page).getByRole("button", { name: /Architecture notes/ }),
  ).toHaveAttribute("aria-pressed", "true");
  await list(page)
    .getByRole("button", { name: /Handoff notes/ })
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
  await expect(
    page.getByRole("heading", { name: "API", exact: true }),
  ).toBeVisible();
  await expect(
    list(page).getByRole("button", { name: /Architecture notes/ }),
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
    list(page).getByRole("button", { name: /First decision/ }),
  ).toBeVisible();
  await page.getByRole("button", { name: "Preview", exact: true }).click();
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
  await page.getByRole("button", { name: "Cancel", exact: true }).click();
  await expect(page.getByRole("dialog")).not.toBeVisible();
  await page.getByRole("button", { name: "Save", exact: true }).click();
  await expect(
    doc(page).getByText("Revision 1", { exact: true }),
  ).toBeVisible();
  await expect(
    list(page).getByRole("button", { name: /Renamed notes/ }),
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
  await page.getByRole("button", { name: "Preview", exact: true }).click();
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
  await expect(
    page.getByRole("heading", { name: "Architecture", exact: true }),
  ).toBeVisible();
  await page.getByRole("button", { name: "Delete", exact: true }).click();
  await page.getByRole("button", { name: "Confirm", exact: true }).click();
  await expect(page.getByRole("dialog")).not.toBeVisible();
  await expect(
    list(page).getByRole("button", { name: /Architecture notes/ }),
  ).toHaveCount(0);
  await page.getByRole("radio", { name: "Trash", exact: true }).check();
  await expect(
    doc(page).getByRole("button", { name: "Restore", exact: true }),
  ).toBeVisible();
  await page.getByRole("button", { name: "Restore", exact: true }).click();
  await expect(
    page.getByText("No memories match your filters.", { exact: true }),
  ).toBeVisible();
  await page.getByRole("radio", { name: "Memories", exact: true }).check();
  await expect(
    list(page).getByRole("button", { name: /Architecture notes/ }),
  ).toBeVisible();
  await page
    .getByRole("main")
    .getByRole("alert")
    .count()
    .then((n) => expect(n).toBe(0));
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
