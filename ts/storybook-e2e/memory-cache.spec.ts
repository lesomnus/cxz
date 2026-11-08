import { expect, test, type Page } from "@playwright/test";
const document = (page: Page) =>
  page.getByRole("region", { name: "Memory document", exact: true });
const collection = (page: Page) =>
  page.getByRole("region", { name: "Project memories", exact: true });
const enter = (page: Page, name = "Architecture notes") =>
  collection(page)
    .getByRole("link", { name, exact: true })
    .evaluate((node) => (node as HTMLAnchorElement).click());
async function leave(page: Page) {
  await page.getByRole("link", { name: "Memory", exact: true }).click();
  await expect(document(page)).toHaveCount(0);
}

test("reopening a Memory document draws the retained body before its delayed background Read", async ({
  page,
}) => {
  await page.goto(
    "/iframe.html?id=pages-memory--cached-document&viewMode=story",
  );
  await enter(page);
  await expect(
    document(page).getByText("Loading memory…", { exact: true }),
  ).toBeVisible();
  await expect(document(page)).toContainText("Keep UI decisions together.");
  await leave(page);
  await enter(page);
  await expect(document(page)).toContainText("Keep UI decisions together.", {
    timeout: 200,
  });
  await expect(
    document(page).getByText("Loading memory…", { exact: true }),
  ).toHaveCount(0);
});

test("background revalidation preserves an inline title draft and metadata saves keep the newer body", async ({
  page,
}) => {
  await page.goto(
    "/iframe.html?id=pages-memory--cached-document&viewMode=story",
  );
  await enter(page);
  await expect(document(page)).toContainText("Keep UI decisions together.");
  await leave(page);
  await page
    .getByRole("button", { name: "Update document on server", exact: true })
    .click();
  await enter(page);
  await document(page)
    .getByRole("button", { name: "Edit Memory name", exact: true })
    .click();
  const editor = page.getByRole("dialog", {
    name: "Edit Memory name",
    exact: true,
  });
  const name = editor.getByRole("textbox", {
    name: "Memory name",
    exact: true,
  });
  await name.fill("Keep my unsaved title");
  await expect(document(page)).toContainText(
    "Another session changed this document.",
  );
  await expect(name).toHaveValue("Keep my unsaved title");
  await editor.getByRole("button", { name: "Confirm", exact: true }).click();
  await expect(editor).toHaveCount(0);
  await expect(document(page)).toContainText("Keep my unsaved title");
  await expect(document(page)).toContainText(
    "Another session changed this document.",
  );
  await expect(
    document(page)
      .locator("dl > div")
      .filter({ has: page.locator("dt", { hasText: /^Revision$/ }) })
      .locator("dd"),
  ).toHaveText("2");
});

test("a confirmed title save is retained for the next visit", async ({
  page,
}) => {
  await page.goto(
    "/iframe.html?id=pages-memory--cached-document&viewMode=story",
  );
  await enter(page);
  await expect(document(page)).toContainText("Keep UI decisions together.");
  await document(page)
    .getByRole("button", { name: "Edit Memory name", exact: true })
    .click();
  const editor = page.getByRole("dialog", {
    name: "Edit Memory name",
    exact: true,
  });
  await editor
    .getByRole("textbox", { name: "Memory name", exact: true })
    .fill("Cached saved title");
  await editor.getByRole("button", { name: "Confirm", exact: true }).click();
  await expect(editor).toHaveCount(0);
  await expect(document(page)).toContainText("Cached saved title");
  await leave(page);
  await enter(page, "Cached saved title");
  await expect(document(page)).toContainText("Keep UI decisions together.", {
    timeout: 200,
  });
  await expect(
    document(page).getByText("Loading memory…", { exact: true }),
  ).toHaveCount(0);
});
