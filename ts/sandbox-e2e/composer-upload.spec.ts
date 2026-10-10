import { expect, test, type Page } from "@playwright/test";
import { mkdtemp, mkdir, writeFile, rm } from "node:fs/promises";
import { tmpdir } from "node:os";
import { join } from "node:path";

test.use({
  viewport: { width: 1440, height: 1000 },
  isMobile: false,
  hasTouch: false,
});
async function open(page: Page) {
  const errors: string[] = [];
  page.on("pageerror", (error) => errors.push(error.message));
  await page.goto("/sandbox.html");
  await expect(
    page.getByRole("heading", { name: "Current status" }),
  ).toBeVisible({ timeout: 45000 });
  return {
    input: page.getByRole("textbox", { name: "Message", exact: true }),
    errors,
  };
}

test("toolbar folder picker preserves nested relative paths in one archive", async ({
  page,
}) => {
  const temp = await mkdtemp(join(tmpdir(), "cxz-browser-upload-"));
  try {
    const folder = join(temp, "assets");
    await mkdir(join(folder, "nested"), { recursive: true });
    await writeFile(join(folder, "nested", "한글.txt"), "source\r\n");
    const { input, errors } = await open(page);
    await page
      .getByRole("button", { name: "Session menu", exact: true })
      .click();
    const choosing = page.waitForEvent("filechooser");
    await page
      .getByRole("menuitem", { name: "Upload folder", exact: true })
      .click();
    await (await choosing).setFiles(folder);
    const chip = page.locator(".composer .paste-chip");
    await expect(chip).toHaveText(/assets/);
    await expect(chip).toHaveAttribute("data-upload-state", "ready");
    await input.press("Control+Enter");
    await expect(
      page.locator("article.input .message-body").last(),
    ).toContainText("assets.tar");
    expect(errors).toEqual([]);
  } finally {
    await rm(temp, { recursive: true, force: true });
  }
});

test("file drop preserves the caret and surrounding text, then sends an uploaded path", async ({
  page,
}) => {
  const { input, errors } = await open(page);
  await input.fill("Before  after");
  await input.evaluate((node: HTMLTextAreaElement) => {
    node.focus();
    node.setSelectionRange(7, 7);
  });
  await input.evaluate((node) => {
    const data = new DataTransfer();
    data.items.add(
      new File(["exact\r\n\0한글"], "한글 report.txt", { type: "text/plain" }),
    );
    node.dispatchEvent(
      new DragEvent("drop", {
        dataTransfer: data,
        bubbles: true,
        cancelable: true,
      }),
    );
  });
  await expect(input).toHaveValue(/^Before \[File .*한글 report.txt\] after$/);
  const chip = page.locator(".composer .paste-chip");
  await expect(chip).toHaveAttribute("data-upload-state", "ready");
  await chip.click();
  const card = page.getByRole("dialog", { name: "Attachment", exact: true });
  const path = await card.locator(".attachment-info code").textContent();
  expect(path).toMatch(
    /^\/cxz\/assets\/session-1\/upload-\d+\/한글 report.txt$/,
  );
  await page.keyboard.press("Escape");
  await input.press("Control+Enter");
  await expect(input).toHaveValue("");
  await expect(page.locator("article.input .message-body").last()).toHaveText(
    `Before [Attached file: ${path} — read this file for the full content] after`,
  );
  expect(errors).toEqual([]);
});

test("folder drop uploads one archive chip and toolbar can choose multiple files", async ({
  page,
}) => {
  const { input, errors } = await open(page);
  await input.evaluate((node) => {
    const data = new DataTransfer();
    const file = new File(["# Readme\n"], "README.md");
    data.items.add(file);
    const entry = {
      isDirectory: true,
      isFile: false,
      name: "sample-folder",
      createReader() {
        let page = 0;
        return {
          readEntries(resolve: (entries: unknown[]) => void) {
            resolve(
              page++ === 0
                ? [
                    {
                      isFile: true,
                      isDirectory: false,
                      name: file.name,
                      file: (resolve: (file: File) => void) => resolve(file),
                    },
                  ]
                : [],
            );
          },
        };
      },
    };
    const descriptor = Object.getOwnPropertyDescriptor(
      DataTransferItem.prototype,
      "webkitGetAsEntry",
    )!;
    Object.defineProperty(DataTransferItem.prototype, "webkitGetAsEntry", {
      configurable: true,
      value: () => entry,
    });
    node.dispatchEvent(
      new DragEvent("drop", {
        dataTransfer: data,
        bubbles: true,
        cancelable: true,
      }),
    );
    Object.defineProperty(
      DataTransferItem.prototype,
      "webkitGetAsEntry",
      descriptor,
    );
  });
  const chip = page.locator(".composer .paste-chip");
  await expect(chip).toHaveText(/sample-folder/);
  await expect(chip).toHaveAttribute("data-upload-state", "ready");
  await chip.click();
  await expect(
    page.getByRole("dialog", { name: "Attachment", exact: true }),
  ).toContainText("sample-folder.tar");
  await page.keyboard.press("Escape");
  await input.press("End");
  await page.getByRole("button", { name: "Session menu", exact: true }).click();
  const choosing = page.waitForEvent("filechooser");
  await page
    .getByRole("menuitem", { name: "Upload files", exact: true })
    .click();
  await (
    await choosing
  ).setFiles([
    { name: "one.txt", mimeType: "text/plain", buffer: Buffer.from("one") },
    { name: "two.txt", mimeType: "text/plain", buffer: Buffer.from("two") },
  ]);
  await expect(chip).toHaveCount(3);
  await expect(
    page.locator('.composer .paste-chip[data-upload-state="ready"]'),
  ).toHaveCount(3);
  await input.press("Control+Enter");
  await expect(
    page.locator("article.input .message-body").last(),
  ).toContainText("Attached directory archive:");
  await expect(
    page.locator("article.input .message-body").last(),
  ).toContainText("two.txt");
  expect(errors).toEqual([]);
});
