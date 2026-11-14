import { expect, test } from "@playwright/test";

const story = "/iframe.html?id=pages-logs--persistent&viewMode=story";
test("OPFS logs survive reload and support filters, details, export and clear", async ({
  page,
}) => {
  await page.goto(story);
  await expect(page.getByRole("status")).toContainText("OPFS");
  await page.getByRole("button", { name: "Clear logs", exact: true }).click();
  await page
    .getByRole("button", { name: "Record disconnect", exact: true })
    .click();
  const record = page.getByRole("button", {
    name: "RPC stream disconnected",
    exact: true,
  });
  await expect(record).toHaveCount(1);
  await record.click();
  const dialog = page.getByRole("dialog", { name: "Log details" });
  await expect(dialog.locator("pre")).toContainText('"closeCode": 1006');
  await page.reload();
  await expect(
    page.getByRole("button", { name: "RPC stream disconnected", exact: true }),
  ).toHaveCount(1);
  await page
    .getByRole("searchbox", { name: "Search logs", exact: true })
    .fill("no-such-record");
  await expect(
    page.getByText("No matching logs", { exact: true }),
  ).toBeVisible();
  await page
    .getByRole("searchbox", { name: "Search logs", exact: true })
    .fill("RPC stream disconnected");
  await page
    .getByRole("combobox", { name: "Select session", exact: true })
    .click();
  await page.getByRole("option", { name: "Review", exact: true }).click();
  await expect(page.getByRole("status")).toContainText("1 matching records");
  const downloadPromise = page.waitForEvent("download");
  await page
    .getByRole("button", { name: "Download filtered logs", exact: true })
    .click();
  const download = await downloadPromise;
  const stream = await download.createReadStream();
  const chunks: Buffer[] = [];
  for await (const chunk of stream!) chunks.push(chunk);
  const records = Buffer.concat(chunks)
    .toString()
    .trim()
    .split("\n")
    .map((line) => JSON.parse(line));
  expect(records).toHaveLength(1);
  expect(records[0].details.closeCode).toBe(1006);
  await page.getByRole("button", { name: "Clear logs", exact: true }).click();
  await expect(
    page.getByText("No matching logs", { exact: true }),
  ).toBeVisible();
});

test("independent tabs share persistent logs and deletion without polling", async ({
  page,
  context,
}) => {
  await page.goto(story);
  await expect(page.getByRole("status")).toContainText("OPFS");
  await page.getByRole("button", { name: "Clear logs", exact: true }).click();
  const other = await context.newPage();
  await other.goto(story);
  await expect(other.getByRole("status")).toContainText("OPFS");
  await Promise.all([
    page
      .getByRole("button", { name: "Record disconnect", exact: true })
      .click(),
    other
      .getByRole("button", { name: "Record disconnect", exact: true })
      .click(),
  ]);
  const matching = (p: typeof page) =>
    p.getByRole("button", { name: "RPC stream disconnected", exact: true });
  await expect(matching(page)).toHaveCount(2);
  await expect(matching(other)).toHaveCount(2);
  await other.getByRole("button", { name: "Clear logs", exact: true }).click();
  await expect(matching(page)).toHaveCount(0);
  await expect(matching(other)).toHaveCount(0);
  await other.close();
});

test("unhandled client errors are recorded and memory fallback is explicit", async ({
  page,
}) => {
  await page.goto(story);
  await page
    .getByRole("button", { name: "Record client error", exact: true })
    .click();
  await expect(
    page.getByRole("button", { name: "Client rendering failed", exact: true }),
  ).toHaveCount(1);
  await page.addInitScript(() => {
    Object.defineProperty(window, "Worker", {
      value: class {
        constructor() {
          throw new Error("Workers blocked");
        }
      },
    });
  });
  await page.reload();
  await expect(page.getByRole("status")).toContainText("Memory only");
  await expect(page.getByText(/These logs are kept in this tab/)).toBeVisible();
});

test("real OPFS segments rotate at 2 MiB and remain readable through a worker", async ({
  page,
}) => {
  await page.goto(story);
  await expect(page.getByRole("status")).toContainText("OPFS");
  await page.getByRole("button", { name: "Clear logs", exact: true }).click();
  const burst = page.getByRole("button", {
    name: "Generate rotation burst",
    exact: true,
  });
  await burst.click();
  await expect(burst).toBeEnabled({ timeout: 30000 });
  await page
    .getByRole("searchbox", { name: "Search logs", exact: true })
    .fill("Rotation record");
  await expect(page.getByRole("status")).toContainText("1200 matching records");
  const loaded = page.locator("[data-loaded-count]");
  await expect(loaded).toHaveAttribute("data-loaded-count", "100");
  expect(await loaded.locator("[data-log-row]").count()).toBeLessThan(100);
  const sizes = await page.evaluate(async () => {
    const root = await navigator.storage.getDirectory();
    const dir = await root.getDirectoryHandle("cxz-client-logs-v1");
    const sizes: number[] = [];
    const iterable = dir as FileSystemDirectoryHandle & {
      values(): AsyncIterableIterator<FileSystemFileHandle>;
    };
    for await (const handle of iterable.values())
      sizes.push((await handle.getFile()).size);
    return sizes;
  });
  expect(sizes.length).toBeGreaterThanOrEqual(2);
  expect(sizes.every((size) => size <= 2 * 1024 * 1024)).toBe(true);
  const viewport = loaded.locator('[data-scroll-area="viewport"]').first();
  await viewport.evaluate((node) => {
    node.scrollTop = node.scrollHeight;
  });
  await expect(loaded).toHaveAttribute("data-loaded-count", "200");
  await viewport.evaluate((node) => {
    node.scrollTop = node.scrollHeight;
  });
  await expect(loaded).toHaveAttribute("data-loaded-count", "300");
  expect(await loaded.locator("[data-log-row]").count()).toBeLessThan(100);
  await expect(
    page.getByRole("button", { name: "Next", exact: true }),
  ).toHaveCount(0);
  await page
    .getByRole("searchbox", { name: "Search logs", exact: true })
    .fill("");
  await expect(loaded).toHaveAttribute("data-loaded-count", "100");
  await viewport.evaluate((node) => {
    node.scrollTop = node.scrollHeight;
  });
  await expect(loaded).toHaveAttribute("data-loaded-count", "200");
  const historyTop = await viewport.evaluate((node) => node.scrollTop);
  const historyRow = await loaded.locator("[data-log-row]").first().innerText();
  await burst.click();
  await expect(burst).toBeEnabled({ timeout: 30000 });
  expect(await viewport.evaluate((node) => node.scrollTop)).toBe(historyTop);
  expect(await loaded.locator("[data-log-row]").first().innerText()).toBe(
    historyRow,
  );
  await viewport.evaluate((node) => {
    node.scrollTop = 0;
  });
  await expect(page.getByRole("status")).toContainText("2400 matching records");
});

for (const width of [1440, 390]) {
  test(`logs reuse the bounded resource layout and shared search at ${width}px`, async ({
    page,
  }, testInfo) => {
    await page.setViewportSize({ width, height: 900 });
    await page.goto("/iframe.html?id=pages-logs--populated&viewMode=story");
    const main = page.getByRole("main", { name: "Logs", exact: true });
    const table = main.getByRole("table", { name: "Client logs", exact: true });
    await expect(table.locator("tbody tr")).toHaveCount(25);
    await expect(table).toHaveCSS("border-top-left-radius", "0px");
    const timeWidth = await table
      .locator("[data-log-row]")
      .first()
      .locator("td")
      .first()
      .evaluate((cell) => {
        const css = getComputedStyle(cell);
        return {
          actual: cell.getBoundingClientRect().width,
          content:
            cell.querySelector("time")!.getBoundingClientRect().width +
            parseFloat(css.paddingLeft) +
            parseFloat(css.paddingRight),
        };
      });
    expect(Math.abs(timeWidth.actual - timeWidth.content)).toBeLessThan(1);
    await expect(
      table.getByRole("columnheader", { name: "Category", exact: true }),
    ).toBeVisible();
    await expect(
      table.getByRole("img", { name: "Client", exact: true }).first(),
    ).toBeVisible();
    const layout = main.locator("[data-resource-layout]");
    const box = (await layout.boundingBox())!;
    const mainBox = (await main.boundingBox())!;
    expect(box.width).toBeLessThanOrEqual(850);
    expect(
      Math.abs(box.x + box.width / 2 - mainBox.x - mainBox.width / 2),
    ).toBeLessThan(1);
    expect(box.x + box.width).toBeLessThanOrEqual(width);
    const toolbar = main.locator("[data-resource-toolbar-content]");
    await expect(
      toolbar.getByRole("combobox", { name: "Select project", exact: true }),
    ).toBeVisible();
    await expect(
      toolbar.getByRole("combobox", { name: "Select session", exact: true }),
    ).toBeVisible();
    const search = main.getByRole("search", { name: "Search logs" });
    await expect(search.locator("svg")).toBeVisible();
    const input = search.getByRole("searchbox", { name: "Search logs" });
    await expect(
      search.getByRole("button", { name: "Log level", exact: true }),
    ).toBeVisible();
    await expect(
      search.getByRole("button", { name: "Log category", exact: true }),
    ).toBeVisible();
    expect(await table.locator("tbody time").first().innerText()).not.toMatch(
      /[+-]\d\d:\d\d/,
    );
    await input.fill("Request 8 failed");
    await expect(table.locator("tbody tr")).toHaveCount(1);
    const row = table.locator("tbody tr").first();
    const before = (await row.boundingBox())!;
    expect(before.height).toBeLessThanOrEqual(28);
    const record = row.getByRole("button");
    await record.focus();
    await page.keyboard.press("Enter");
    const dialog = page.getByRole("dialog", { name: "Log details" });
    await expect(dialog.locator("pre")).toContainText('"attempt": 8');
    await expect
      .poll(() =>
        dialog.evaluate(
          (node) =>
            node
              .getAnimations({ subtree: true })
              .filter(
                (animation) =>
                  animation.playState === "running" || animation.pending,
              ).length,
        ),
      )
      .toBe(0);
    await expect(main.locator("details")).toHaveCount(0);
    expect((await row.boundingBox())!.height).toBe(before.height);
    await expect(dialog.locator("[data-scroll-area=viewport]")).toBeVisible();
    await page.screenshot({ path: testInfo.outputPath("log-details.png") });
    await page.keyboard.press("Escape");
    await expect(page.locator("dialog")).toHaveCount(0);
    await expect(record).toBeFocused();
    expect((await row.boundingBox())!.height).toBe(before.height);
    await input.fill("");
    await expect(table.locator("tbody tr")).toHaveCount(25);
    await table.locator("tbody tr").first().getByRole("cell").first().click();
    await expect(
      page.getByRole("dialog", { name: "Log details" }),
    ).toBeVisible();
    await page
      .getByRole("button", { name: "Close details", exact: true })
      .click();
    await expect(page.locator("dialog")).toHaveCount(0);
    expect(
      await page.evaluate(() => document.documentElement.scrollWidth),
    ).toBeLessThanOrEqual(width);
    await page.screenshot({ path: testInfo.outputPath("logs-page.png") });
    if (width === 1440) {
      await page.goto("/iframe.html?id=pages-logs--details&viewMode=story");
      await expect(
        page.getByRole("dialog", { name: "Log details" }).locator("pre"),
      ).toContainText('"disconnectedMs": 1200');
    }
  });
}

test("checkbox filters stay open for multiple selections and project/session selectors scope logs", async ({
  page,
}, testInfo) => {
  await page.goto("/iframe.html?id=pages-logs--populated&viewMode=story");
  const status = page.getByRole("status");
  await expect(status).toContainText("25 matching records");
  const level = page.getByRole("button", { name: "Log level", exact: true });
  await level.click();
  const menu = page.getByRole("menu", { name: "Log level", exact: true });
  const all = menu.getByRole("menuitemcheckbox", {
    name: "All levels",
    exact: true,
  });
  await expect(all).toHaveAttribute("aria-checked", "true");
  await all.click();
  await expect(status).toContainText("25 matching records");
  await expect(menu).toBeVisible();
  await menu
    .getByRole("menuitemcheckbox", { name: "warn", exact: true })
    .click();
  await expect(status).toContainText("25 matching records");
  await expect(all).toHaveAttribute("aria-checked", "mixed");
  await menu.screenshot({
    path: testInfo.outputPath("checkbox-filters.png"),
    animations: "disabled",
  });
  await level.click();
  await expect(menu).toHaveCount(0);
  await expect(status).toContainText("6 matching records");
  await level.click();
  await menu
    .getByRole("menuitemcheckbox", { name: "error", exact: true })
    .click();
  await expect(status).toContainText("6 matching records");
  await page.keyboard.press("Escape");
  await expect(status).toContainText("10 matching records");
  await level.click();
  await all.focus();
  await page.keyboard.press("ArrowDown");
  await page.keyboard.press("Space");
  await expect(status).toContainText("10 matching records");
  await expect(all).toHaveAttribute("aria-checked", "true");
  await page.keyboard.press("Escape");
  await expect(menu).toHaveCount(0);
  await expect(level).toBeFocused();
  await expect(status).toContainText("25 matching records");
  await page.getByRole("button", { name: "Log category", exact: true }).click();
  const categories = page.getByRole("menu", {
    name: "Log category",
    exact: true,
  });
  await categories
    .getByRole("menuitemcheckbox", { name: "All categories", exact: true })
    .click();
  await categories
    .getByRole("menuitemcheckbox", { name: "RPC", exact: true })
    .click();
  await expect(status).toContainText("25 matching records");
  await page.keyboard.press("Escape");
  await expect(status).toContainText("13 matching records");
  await page.getByRole("button", { name: "Log category", exact: true }).click();
  await categories
    .getByRole("menuitemcheckbox", { name: "Client", exact: true })
    .click();
  await expect(status).toContainText("13 matching records");
  await page.getByRole("columnheader", { name: "Time", exact: true }).click();
  await expect(categories).toHaveCount(0);
  await expect(status).toContainText("25 matching records");
  await page
    .getByRole("combobox", { name: "Select project", exact: true })
    .click();
  await page.getByRole("option", { name: "Archive", exact: true }).click();
  await expect(status).toContainText("0 matching records");
  await page
    .getByRole("combobox", { name: "Select project", exact: true })
    .click();
  await page.getByRole("option", { name: "Workspace", exact: true }).click();
  await expect(status).toContainText("25 matching records");
  await page
    .getByRole("combobox", { name: "Select session", exact: true })
    .click();
  await page.getByRole("option", { name: "Build", exact: true }).click();
  await expect(status).toContainText("12 matching records");
  await page
    .getByRole("combobox", { name: "Select session", exact: true })
    .click();
  await page.getByRole("option", { name: "Review", exact: true }).click();
  await expect(status).toContainText("13 matching records");
});

test("reusable checkbox menu preserves disabled choices when toggling all", async ({
  page,
}) => {
  await page.goto(
    "/iframe.html?id=components-checkboxmenu--disabled-option&viewMode=story",
  );
  await page.getByRole("button", { name: "Log level", exact: true }).click();
  const all = page.getByRole("menuitemcheckbox", {
    name: "All levels",
    exact: true,
  });
  await expect(all).toHaveAttribute("aria-checked", "mixed");
  await all.click();
  await all.click();
  await expect(page.getByLabel("Selected values")).toHaveText("info, error");
  await expect(page.getByLabel("Apply count")).toHaveText("0");
  await expect(
    page.getByRole("menuitemcheckbox", { name: "Errors", exact: true }),
  ).toBeDisabled();
  await page.keyboard.press("End");
  await expect(
    page.getByRole("menuitemcheckbox", { name: "Warnings", exact: true }),
  ).toBeFocused();
  await page.keyboard.press("Escape");
  await expect(page.getByLabel("Selected values")).toHaveText("error");
  await expect(page.getByLabel("Apply count")).toHaveText("1");
  await page.getByRole("button", { name: "Log level", exact: true }).click();
  await page.keyboard.press("Escape");
  await expect(page.getByLabel("Apply count")).toHaveText("1");
});
