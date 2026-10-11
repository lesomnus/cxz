import { expect, test, devices } from "@playwright/test";
import { readFile } from "node:fs/promises";

test.use({
  ...devices["Desktop Chrome"],
  viewport: { width: 1200, height: 900 },
});
test.beforeEach(async ({ page }) => {
  await page.goto("/sandbox.html");
  await expect(
    page.getByRole("textbox", { name: "Message", exact: true }),
  ).toBeVisible();
  await expect(
    page.getByRole("combobox", { name: "Model", exact: true }),
  ).toBeEnabled();
});

test("inline container paths complete through directories without moving the caret while browsing", async ({
  page,
}) => {
  const input = page.getByRole("textbox", { name: "Message", exact: true });
  await input.fill("First line\nInspect `/work` and preserve the rest");
  const caret = (await input.inputValue()).indexOf("` and");
  await input.evaluate(
    (el: HTMLTextAreaElement, position) =>
      el.setSelectionRange(position, position),
    caret,
  );
  const list = page.getByRole("grid", {
    name: "Path suggestions",
    exact: true,
  });
  await expect(list.locator('[aria-selected="true"]')).toHaveAttribute(
    "aria-label",
    "/workspace/",
  );
  await expect(page.locator(".editor-mirror > .editor-line").first()).toHaveCSS(
    "visibility",
    "visible",
  );
  const before = await input.inputValue();
  await input.press("ArrowDown");
  expect(await input.inputValue()).toBe(before);
  expect(
    await input.evaluate((el: HTMLTextAreaElement) => el.selectionStart),
  ).toBe(caret);
  await input.press("Tab");
  await expect(input).toHaveValue(
    "First line\nInspect `/workspace/` and preserve the rest",
  );
  await expect(
    list.getByRole("gridcell", { name: /\/workspace\/src\// }),
  ).toBeVisible();
  await list.getByRole("gridcell", { name: /\/workspace\/src\// }).click();
  await expect(input).toHaveValue(
    "First line\nInspect `/workspace/src/` and preserve the rest",
  );
  await expect(
    list.getByRole("gridcell", { name: /\/workspace\/src\/app.ts/ }),
  ).toBeVisible();
  await list
    .getByRole("gridcell", { name: /\/workspace\/src\/app.ts/ })
    .click();
  await expect(input).toHaveValue(
    "First line\nInspect `/workspace/src/app.ts` and preserve the rest",
  );
  await expect(list).toHaveCount(0);
  await input.press("Control+z");
  await expect(input).toHaveValue(
    "First line\nInspect `/workspace/src/` and preserve the rest",
  );
});

test("home paths, missing directories, Escape, IME and fenced code use the right completion context", async ({
  page,
}) => {
  const input = page.getByRole("textbox", { name: "Message", exact: true });
  const list = page.getByRole("grid", {
    name: "Path suggestions",
    exact: true,
  });
  await input.fill("Look `~");
  await expect(
    list.getByRole("gridcell", { name: /~\/notes.txt/ }),
  ).toBeVisible();
  await input.press("Escape");
  await expect(list).toHaveCount(0);
  await input.pressSequentially("/");
  await expect(list).toBeVisible();
  await input.dispatchEvent("compositionstart", { data: "중" });
  await expect(list).toHaveCount(0);
  await input.dispatchEvent("compositionend", { data: "중" });
  await expect(list).toBeVisible();
  await input.fill("Look `/workspace/missing/");
  await expect(list.getByRole("status")).toContainText("Directory unavailable");
  await expect(list.getByRole("status")).toContainText("/workspace/missing/");
  await expect(list.getByRole("status")).toContainText(
    "sandbox directory not found",
  );
  await input.fill("```\n/workspace/src/\n```");
  await input.evaluate((el: HTMLTextAreaElement) =>
    el.setSelectionRange(19, 19),
  );
  await expect(list).toHaveCount(0);
  await input.fill("plain /workspace/src/");
  await expect(list).toHaveCount(0);
});

test("typing paths between automatically paired backticks matches download argument hints", async ({
  page,
}) => {
  const input = page.getByRole("textbox", { name: "Message", exact: true });
  const list = page.getByRole("grid", {
    name: "Path suggestions",
    exact: true,
  });
  for (const path of ["/workspace/", "~/"]) {
    await input.fill("");
    await input.pressSequentially(`Inspect \`${path}`, { delay: 25 });
    await expect(input).toHaveValue(`Inspect \`${path}\``);
    const selected = list.locator('[aria-selected="true"]');
    await expect(selected).toBeVisible();
    const inline = await selected.innerText();
    await input.fill("");
    await input.pressSequentially(`/download ${path}`, { delay: 25 });
    await expect(selected).toHaveText(inline);
  }
});

test("download completes its path and saves bytes without sending a prompt; failures retain the command", async ({
  page,
}) => {
  const input = page.getByRole("textbox", { name: "Message", exact: true });
  await input.fill("/download README");
  const list = page.getByRole("grid", {
    name: "Path suggestions",
    exact: true,
  });
  await expect(list.locator('[aria-selected="true"]')).toHaveAttribute(
    "aria-label",
    "/workspace/README.md",
  );
  await input.press("Tab");
  await expect(input).toHaveValue("/download /workspace/README.md");
  const downloadReady = page.waitForEvent("download");
  await input.press("Control+Enter");
  const download = await downloadReady;
  expect(download.suggestedFilename()).toBe("README.md");
  const content = await readFile((await download.path())!, "utf8");
  expect(content).toContain("This workspace is simulated by the WASM service.");
  await expect(input).toHaveValue("");
  await expect(
    page.locator("article.input").filter({ hasText: "/download" }),
  ).toHaveCount(0);
  await input.fill("/download ~/notes.txt");
  const homeReady = page.waitForEvent("download");
  await input.press("Control+Enter");
  expect((await homeReady).suggestedFilename()).toBe("notes.txt");
  await expect(input).toHaveValue("");
  await input.fill("/download /workspace/missing.txt");
  await input.press("Control+Enter");
  await expect(page.getByRole("alert")).toContainText("sandbox file not found");
  await expect(input).toHaveValue("/download /workspace/missing.txt");
  await expect(input).not.toHaveAttribute("readonly");
});
