import { test, expect, type Page } from "@playwright/test";
const open = async (page: Page, story = "default") => {
  await page.clock.setFixedTime(new Date("2026-11-05T12:00:00Z"));
  await page.goto(`/iframe.html?id=pages-memory--${story}&viewMode=story`);
};
const grid = (page: Page) =>
  page.getByRole("region", { name: "Project memories", exact: true });
const doc = (page: Page) =>
  page.getByRole("region", { name: "Memory document", exact: true });
const card = (page: Page, name: string) =>
  grid(page).getByRole("link", { name, exact: true });
const back = (page: Page) =>
  page.getByRole("link", { name: "Memory", exact: true }).click();
async function rename(page: Page, name: string) {
  await doc(page)
    .getByRole("button", { name: "Edit Memory name", exact: true })
    .click();
  const editor = page.getByRole("dialog", {
    name: "Edit Memory name",
    exact: true,
  });
  await editor
    .getByRole("textbox", { name: "Memory name", exact: true })
    .fill(name);
  await editor.getByRole("button", { name: "Confirm", exact: true }).click();
  await expect(editor).toHaveCount(0);
}

test("the collection replaces old panels and controls with rectangular memory cards", async ({
  page,
}) => {
  await open(page);
  await expect(grid(page).getByRole("link")).toHaveCount(2);
  await expect(doc(page)).toHaveCount(0);
  await expect(page.getByRole("searchbox")).toHaveCount(0);
  await expect(
    page.getByRole("textbox", { name: "Search memory", exact: true }),
  ).toHaveCount(0);
  await expect(
    page.getByRole("radiogroup", { name: "Memory scope", exact: true }),
  ).toHaveCount(0);
  await expect(
    page.getByRole("button", { name: "New memory", exact: true }),
  ).toHaveCount(0);
  const item = card(page, "Architecture notes");
  await expect(item).toHaveAttribute(
    "href",
    /\/c\/_\/project\/web\/memory\/[\da-f-]{36}$/,
  );
  await expect(
    item.getByText("Design decisions", { exact: true }),
  ).toBeVisible();
  await expect(
    item.getByRole("img", { name: "Claude", exact: true }),
  ).toBeVisible();
  await expect(
    item.getByRole("img", { name: "Revision 1", exact: true }),
  ).toBeVisible();
  await expect(item.locator("time")).toHaveText("now");
  await expect(grid(page)).not.toContainText("topic=design");
});
for (const width of [390, 850, 1200])
  test(`cards preserve their hierarchy and grid at ${width}px`, async ({
    page,
  }) => {
    await page.setViewportSize({ width, height: 900 });
    await open(page, "card-grid");
    const cards = grid(page).getByRole("link");
    await expect(cards).toHaveCount(8);
    const boxes = await cards.evaluateAll((nodes) =>
      nodes.map((node) => {
        const r = node.getBoundingClientRect();
        return { x: r.x, y: r.y, width: r.width, height: r.height };
      }),
    );
    const columns = boxes.filter(
      (box) => Math.abs(box.y - boxes[0].y) < 1,
    ).length;
    expect(columns).toBe(width === 1200 ? 5 : 2);
    expect(
      boxes.every(
        (box) => box.width / box.height > 0.7 && box.width / box.height < 0.8,
      ),
    ).toBe(true);
    expect(
      await page.evaluate(() => document.documentElement.scrollWidth),
    ).toBe(width);
    const item = card(page, "Architecture notes");
    const owner = await item
      .getByText("Design decisions", { exact: true })
      .boundingBox();
    const title = item.getByText("Architecture notes", { exact: true });
    const titleBox = await title.boundingBox();
    const revision = await item
      .getByRole("img", { name: "Revision 1", exact: true })
      .locator("..")
      .boundingBox();
    const time = await item.locator("time").boundingBox();
    const box = await item.boundingBox();
    expect(owner!.y).toBeLessThan(titleBox!.y);
    expect(titleBox!.y).toBeLessThan(revision!.y);
    expect(revision!.x - box!.x).toBeCloseTo(12, 0);
    expect(box!.x + box!.width - time!.x - time!.width).toBeCloseTo(12, 0);
    expect(Math.abs(owner!.y - time!.y)).toBeLessThan(3);
    expect(time!.y).toBeLessThan(titleBox!.y);
    expect(box!.y + box!.height - revision!.y - revision!.height).toBeCloseTo(
      12,
      0,
    );
    await expect(item).toHaveCSS("border-top-width", "0px");
    await expect(title).toHaveCSS("font-size", "14px");
    await expect(
      card(page, "Choosing a storage model for project memories").locator(
        "time",
      ),
    ).toHaveText("1 hour ago");
  });
test("cards handle both agent logos, long names and absent or retained owners", async ({
  page,
}) => {
  await open(page, "card-grid");
  await expect(
    grid(page).getByRole("img", { name: "Codex", exact: true }).first(),
  ).toBeVisible();
  await expect(
    grid(page).getByRole("img", { name: "Claude", exact: true }).first(),
  ).toBeVisible();
  await expect(
    card(page, "Notes without a session owner").getByText("Project memory", {
      exact: true,
    }),
  ).toBeVisible();
  await expect(
    card(page, "A retained session's architectural decisions"),
  ).toContainText("Session ");
  await expect(grid(page)).not.toContainText("legacy.");
  const long = card(
    page,
    "A long memory title that wraps across several lines without changing the card height",
  );
  expect(
    await long.evaluate((node) => node.scrollWidth <= node.clientWidth),
  ).toBe(true);
});
test("card selection opens a document without the former catalog sidebar", async ({
  page,
}) => {
  await open(page);
  await card(page, "Architecture notes").click();
  await expect(
    page.getByRole("heading", { name: "Architecture", exact: true }),
  ).toBeVisible();
  await expect(grid(page)).toHaveCount(0);
  await expect(
    page.getByRole("link", { name: "Back to memories", exact: true }),
  ).toHaveCount(0);
  await expect(
    page.getByRole("textbox", { name: "Search memory", exact: true }),
  ).toHaveCount(0);
  await back(page);
  await expect(card(page, "Handoff notes")).toBeVisible();
  await card(page, "Handoff notes").click();
  await expect(
    page.getByRole("heading", { name: "Handoff", exact: true }),
  ).toBeVisible();
});
test("project selection changes the collection scope", async ({ page }) => {
  await open(page);
  await expect(card(page, "Architecture notes")).toBeVisible();
  await page
    .getByRole("combobox", { name: "Select project", exact: true })
    .click();
  await page
    .getByRole("option", { name: "API workspace", exact: true })
    .click();
  await expect(grid(page).getByRole("link")).toHaveCount(1);
  await expect(card(page, "API notes")).toBeVisible();
  await expect(card(page, "Architecture notes")).toHaveCount(0);
});
for (const [story, message] of [
  ["empty", "No memories yet."],
  ["loading", "Loading memories…"],
  ["unavailable", "Memories could not be loaded."],
] as const)
  test(`the new collection has an explicit ${story} state`, async ({
    page,
  }) => {
    await open(page, story);
    await expect(page.getByText(message, { exact: false })).toBeVisible();
    await expect(doc(page)).toHaveCount(0);
    await expect(grid(page).getByRole("link")).toHaveCount(0);
  });
test("documents can still be read without a current owner session", async ({
  page,
}) => {
  await open(page, "no-sessions");
  await expect(card(page, "Architecture notes")).toContainText("Session ");
  await card(page, "Architecture notes").click();
  await expect(
    page.getByRole("heading", { name: "Architecture", exact: true }),
  ).toBeVisible();
});
test("the inline title refreshes the card without changing its body or revision", async ({
  page,
}) => {
  await open(page);
  await card(page, "Architecture notes").click();
  await rename(page, "Renamed notes");
  await expect(
    doc(page)
      .locator("dl > div")
      .filter({ has: page.locator("dt", { hasText: /^Revision$/ }) })
      .locator("dd"),
  ).toHaveText("1");
  await expect(doc(page)).toContainText("Keep UI decisions together.");
  await expect(doc(page)).toContainText("topic=design");
  await back(page);
  await expect(
    card(page, "Renamed notes").getByRole("img", {
      name: "Revision 1",
      exact: true,
    }),
  ).toBeVisible();
});

test("returning to the grid guards an unsaved copied document", async ({
  page,
}) => {
  await open(page);
  await card(page, "Architecture notes").click();
  await doc(page).getByRole("button", { name: "Copy", exact: true }).click();
  await expect(page.locator(".monaco-editor textarea")).toBeVisible();
  await back(page);
  const dialog = page.getByRole("dialog", {
    name: "Discard unsaved changes",
    exact: true,
  });
  await dialog.getByRole("button", { name: "Cancel", exact: true }).click();
  await expect(
    doc(page).getByRole("button", { name: "Edit Memory name", exact: true }),
  ).toContainText("Architecture notes copy");
  await back(page);
  await dialog.getByRole("button", { name: "Confirm", exact: true }).click();
  await expect(card(page, "Architecture notes")).toBeVisible();
  await expect(card(page, "Architecture notes copy")).toHaveCount(0);
});

for (const width of [390, 1280])
  test(`document uses grouped title/body sections and panel actions at ${width}px`, async ({
    page,
  }) => {
    await page.setViewportSize({ width, height: 900 });
    await open(page, "document");
    await expect(
      doc(page).getByRole("button", { name: "Edit Memory name", exact: true }),
    ).toContainText("Architecture notes");
    for (const name of ["Edit", "Source", "Memory actions"])
      await expect(
        doc(page).getByRole("button", { name, exact: true }),
      ).toHaveCount(0);
    for (const text of ["Version history", "Document details"])
      await expect(doc(page).getByText(text, { exact: true })).toHaveCount(0);
    await expect(doc(page)).toHaveAttribute("data-section-layout", "group");
    const title = doc(page).getByRole("region", {
      name: "Memory information",
      exact: true,
    });
    await expect(title.locator("h2")).toHaveCSS("font-size", "16px");
    await expect(title.locator("h2 svg")).toHaveCount(0);
    for (const label of [
      "Title",
      "Owner session",
      "Revision",
      "Memory ID",
      "Created",
      "Updated",
      "Project",
      "Labels",
    ])
      await expect(
        title.locator("dt").filter({ hasText: new RegExp(`^${label}`) }),
      ).toBeVisible();
    await expect(
      title.getByRole("link", { name: /Design decisions/ }),
    ).toBeVisible();
    await expect(
      title.getByRole("link", { name: /Web workspace/ }),
    ).toBeVisible();
    if (width === 1280) {
      expect(
        await title
          .locator("dl")
          .evaluate(
            (node) =>
              getComputedStyle(node).gridTemplateColumns.split(" ").length,
          ),
      ).toBe(6);
      await expect(title.locator("dl > div").first()).toHaveCSS(
        "grid-column",
        "span 3",
      );
      const firstRow = await title.locator("dl > div").evaluateAll((nodes) => {
        const top = nodes[0].getBoundingClientRect().top;
        return nodes
          .filter(
            (node) => Math.abs(node.getBoundingClientRect().top - top) < 1,
          )
          .map((node) => node.querySelector("dt")?.textContent?.trim());
      });
      expect(firstRow).toEqual([
        "Title",
        "Owner session",
        "Revision",
        "Memory ID",
      ]);
    }
    const actions = doc(page).getByRole("group", {
      name: "Memory document actions",
      exact: true,
    });
    for (const name of ["Copy", "Reload", "Delete"])
      await expect(
        actions.getByRole("button", { name, exact: true }),
      ).toHaveAttribute("data-section-action", "button");
    const layout = await doc(page).evaluate((root) => {
      const title = root
        .querySelector('[aria-label="Memory information"]')!
        .getBoundingClientRect();
      const actions = root
        .querySelector('[aria-label="Memory document actions"]')!
        .getBoundingClientRect();
      const body = root
        .querySelector('[aria-label="Memory content"]')!
        .getBoundingClientRect();
      return {
        titleRight: title.right,
        titleBottom: title.bottom,
        actionsLeft: actions.left,
        actionsTop: actions.top,
        headerBottom: Math.max(title.bottom, actions.bottom),
        bodyTop: body.top,
      };
    });
    if (width === 1280)
      expect(layout.actionsLeft).toBeGreaterThan(layout.titleRight);
    else expect(layout.actionsTop).toBeGreaterThan(layout.titleBottom);
    expect(layout.bodyTop).toBeGreaterThan(layout.headerBottom);
    expect(
      await page.evaluate(
        () => document.documentElement.scrollWidth <= innerWidth,
      ),
    ).toBe(true);
  });

test("document loading keeps the grouped frame and disabled action panels", async ({
  page,
}) => {
  await open(page, "document-loading");
  await expect(doc(page)).toHaveAttribute("aria-busy", "true");
  await expect(doc(page)).toHaveAttribute("data-section-layout", "group");
  await expect(
    doc(page).getByRole("region", { name: "Memory information", exact: true }),
  ).toContainText("Memory");
  await expect(
    doc(page).getByText("Loading memory…", { exact: true }),
  ).toBeVisible();
  for (const name of ["Copy", "Reload", "Delete"])
    await expect(
      doc(page).getByRole("button", { name, exact: true }),
    ).toBeDisabled();
  await expect(
    doc(page).getByRole("button", { name: "Edit Memory name", exact: true }),
  ).toHaveCount(0);
});

test("the Reload panel refreshes the preview while showing pending feedback", async ({
  page,
}) => {
  await open(page, "cached-document");
  await card(page, "Architecture notes").click();
  await expect(doc(page)).toContainText("Keep UI decisions together.");
  await page
    .getByRole("button", { name: "Update document on server", exact: true })
    .click();
  const reload = doc(page).getByRole("button", { name: "Reload", exact: true });
  await reload.click();
  await expect(reload).toHaveAttribute("aria-busy", "true");
  await expect(reload).toBeDisabled();
  await expect(doc(page)).toContainText(
    "Another session changed this document.",
  );
  await expect(reload).toBeEnabled();
});

test("inline rename conflicts keep the attempted title in its editor", async ({
  page,
}) => {
  await open(page, "revision-conflict");
  await card(page, "Architecture notes").click();
  await doc(page)
    .getByRole("button", { name: "Edit Memory name", exact: true })
    .click();
  const editor = page.getByRole("dialog", {
    name: "Edit Memory name",
    exact: true,
  });
  const input = editor.getByRole("textbox", {
    name: "Memory name",
    exact: true,
  });
  await input.fill("Unsaved name");
  await editor.getByRole("button", { name: "Confirm", exact: true }).click();
  await expect(editor.getByRole("alert")).toContainText(
    "Memory revision changed",
  );
  await expect(input).toHaveValue("Unsaved name");
  await editor
    .getByRole("button", { name: "Dismiss error", exact: true })
    .click();
  await expect(input).toHaveValue("Unsaved name");
  await editor.getByRole("button", { name: "Cancel", exact: true }).click();
  await expect(doc(page)).toContainText("Architecture notes");
});

test("copy creation stays in the editor until saved, then appears in the grid", async ({
  page,
}) => {
  await open(page);
  await card(page, "Architecture notes").click();
  await doc(page).getByRole("button", { name: "Copy", exact: true }).click();
  await rename(page, "Copied architecture");
  const input = page.locator(".monaco-editor textarea");
  await input.focus();
  await page.keyboard.press("ControlOrMeta+A");
  await page.keyboard.insertText("# Copied body\n\nNew document content.");
  await doc(page).getByRole("button", { name: "Save", exact: true }).click();
  await expect(
    doc(page)
      .locator("dl > div")
      .filter({ has: page.locator("dt", { hasText: /^Revision$/ }) })
      .locator("dd"),
  ).toHaveText("1");
  await expect(doc(page)).toContainText("New document content.");
  await back(page);
  await expect(card(page, "Copied architecture")).toBeVisible();
});

test("deleting from its panel action returns to the refreshed collection", async ({
  page,
}) => {
  await open(page);
  await card(page, "Architecture notes").click();
  await doc(page).getByRole("button", { name: "Delete", exact: true }).click();
  await page
    .getByRole("dialog")
    .getByRole("button", { name: "Confirm", exact: true })
    .click();
  await expect(card(page, "Architecture notes")).toHaveCount(0);
  await expect(card(page, "Handoff notes")).toBeVisible();
});
test("migrated filenames remain distinct cards without provenance labels", async ({
  page,
}) => {
  await open(page, "migrated-documents");
  const links = grid(page).getByRole("link");
  await expect(links).toHaveCount(26);
  const hrefs = await links.evaluateAll((nodes) =>
    nodes.map((node) => (node as HTMLAnchorElement).href),
  );
  expect(new Set(hrefs).size).toBe(26);
  await expect(grid(page)).not.toContainText("legacy.");
});

async function chooseSession(page: Page, name: string) {
  await page
    .getByRole("combobox", { name: "Select session", exact: true })
    .click();
  await page.getByRole("option", { name, exact: true }).click();
}
test("session dropdown filters owner cards and All sessions includes unowned and retained memories", async ({
  page,
}) => {
  await open(page, "card-grid");
  await expect(grid(page).getByRole("link")).toHaveCount(8);
  await chooseSession(page, "New editing session");
  await expect(grid(page).getByRole("link")).toHaveCount(2);
  await expect(card(page, "Release checklist")).toBeVisible();
  await expect(card(page, "Architecture notes")).toHaveCount(0);
  await card(page, "Release checklist").click();
  await expect(
    page.getByRole("heading", { name: "Release checklist", exact: true }),
  ).toBeVisible();
  await back(page);
  await expect(grid(page).getByRole("link")).toHaveCount(2);
  await chooseSession(page, "Design decisions");
  await expect(grid(page).getByRole("link")).toHaveCount(4);
  await expect(card(page, "Architecture notes")).toBeVisible();
  await chooseSession(page, "All sessions");
  await expect(grid(page).getByRole("link")).toHaveCount(8);
  await expect(card(page, "Notes without a session owner")).toBeVisible();
});
test("project changes reset the session filter and offer only the current project's sessions", async ({
  page,
}) => {
  await open(page, "card-grid");
  await chooseSession(page, "New editing session");
  await expect(grid(page).getByRole("link")).toHaveCount(2);
  await page
    .getByRole("combobox", { name: "Select project", exact: true })
    .click();
  await page
    .getByRole("option", { name: "API workspace", exact: true })
    .click();
  const session = page.getByRole("combobox", {
    name: "Select session",
    exact: true,
  });
  await expect(session).toContainText("All sessions");
  await expect(card(page, "API notes")).toBeVisible();
  await session.click();
  await expect(
    page.getByRole("option", { name: "API implementation", exact: true }),
  ).toBeVisible();
  await expect(
    page.getByRole("option", { name: "Design decisions", exact: true }),
  ).toHaveCount(0);
  await page
    .getByRole("option", { name: "API implementation", exact: true })
    .click();
  await expect(card(page, "API notes")).toBeVisible();
});
test("changing session from a document guards drafts and returns to the filtered collection", async ({
  page,
}) => {
  await open(page, "document");
  await doc(page).getByRole("button", { name: "Copy", exact: true }).click();
  await expect(page.locator(".monaco-editor textarea")).toBeVisible();
  await chooseSession(page, "New editing session");
  const dialog = page.getByRole("dialog", {
    name: "Discard unsaved changes",
    exact: true,
  });
  await dialog.getByRole("button", { name: "Cancel", exact: true }).click();
  await expect(
    doc(page).getByRole("button", { name: "Edit Memory name", exact: true }),
  ).toContainText("Architecture notes copy");
  await expect(
    page.getByRole("combobox", { name: "Select session", exact: true }),
  ).toContainText("All sessions");
  await chooseSession(page, "New editing session");
  await dialog.getByRole("button", { name: "Confirm", exact: true }).click();
  await expect(doc(page)).toHaveCount(0);
  await expect(grid(page)).toContainText("No memories for this session.");
  await chooseSession(page, "All sessions");
  await expect(card(page, "Architecture notes")).toBeVisible();
});
