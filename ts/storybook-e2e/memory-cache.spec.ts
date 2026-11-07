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

test("background revalidation preserves an unsaved draft and its original conflict revision", async ({
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
    .getByRole("button", { name: "Edit", exact: true })
    .click();
  const name = document(page).getByRole("textbox", {
    name: "Memory name",
    exact: true,
  });
  await name.fill("Keep my unsaved draft");
  // The revalidation is deliberately slower than entering the editor.
  await page.waitForTimeout(1200);
  await expect(name).toHaveValue("Keep my unsaved draft");
  await document(page)
    .getByRole("button", { name: "Save", exact: true })
    .click();
  await expect(
    page.getByText(/Memory (version|revision) changed/).first(),
  ).toBeVisible();
  await expect(name).toHaveValue("Keep my unsaved draft");
});

test("a confirmed save is retained for the next visit", async ({ page }) => {
  await page.goto(
    "/iframe.html?id=pages-memory--cached-document&viewMode=story",
  );
  await enter(page);
  await expect(document(page)).toContainText("Keep UI decisions together.");
  await document(page)
    .getByRole("button", { name: "Edit", exact: true })
    .click();
  await document(page)
    .getByRole("textbox", { name: "Memory name", exact: true })
    .fill("Cached saved document");
  const editor = page.locator(".monaco-editor textarea");
  await editor.focus();
  await page.keyboard.press("ControlOrMeta+A");
  await page.keyboard.insertText(
    "# Cached save\n\nNew body remains available.",
  );
  await document(page)
    .getByRole("button", { name: "Save", exact: true })
    .click();
  await expect(
    document(page).getByRole("button", { name: "Edit", exact: true }),
  ).toBeVisible();
  await expect(document(page)).toContainText("New body remains available.");
  await leave(page);
  await enter(page, "Cached saved document");
  await expect(document(page)).toContainText("New body remains available.", {
    timeout: 200,
  });
  await expect(
    document(page).getByText("Loading memory…", { exact: true }),
  ).toHaveCount(0);
});
