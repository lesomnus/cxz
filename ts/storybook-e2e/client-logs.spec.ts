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
    .getByRole("textbox", { name: "Session UUID", exact: true })
    .fill("01949770-0000-8000-8008-000000000001");
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
  await expect(
    page
      .getByRole("table", { name: "Client logs", exact: true })
      .locator("tbody tr"),
  ).toHaveCount(100);
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
  await page.getByRole("button", { name: "Next", exact: true }).click();
  await expect(page.getByText("101–200 / 1200", { exact: true })).toBeVisible();
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
    const layout = main.locator("[data-resource-layout]");
    const box = (await layout.boundingBox())!;
    const mainBox = (await main.boundingBox())!;
    expect(box.width).toBeLessThanOrEqual(850);
    expect(
      Math.abs(box.x + box.width / 2 - mainBox.x - mainBox.width / 2),
    ).toBeLessThan(1);
    expect(box.x + box.width).toBeLessThanOrEqual(width);
    const fields = main
      .getByRole("group", { name: "Search logs", exact: true })
      .locator(":scope > div");
    const firstField = (await fields.nth(0).boundingBox())!;
    const thirdField = (await fields.nth(2).boundingBox())!;
    if (width < 600) {
      expect(thirdField.y).toBeGreaterThan(firstField.y + firstField.height);
    } else {
      expect(thirdField.y).toBe(firstField.y);
    }
    const search = main.getByRole("search", { name: "Search logs" });
    await expect(search.locator("svg")).toBeVisible();
    const input = search.getByRole("searchbox", { name: "Search logs" });
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
