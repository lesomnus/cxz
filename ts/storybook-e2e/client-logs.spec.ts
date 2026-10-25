import { expect, test } from "@playwright/test";

const story = "/iframe.html?id=settings-logs--persistent&viewMode=story";
test("OPFS logs survive reload and support filters, details, export and clear", async ({
  page,
}) => {
  await page.goto(story);
  await expect(page.getByRole("status")).toContainText("OPFS");
  await page.getByRole("button", { name: "Clear logs", exact: true }).click();
  await page
    .getByRole("button", { name: "Record disconnect", exact: true })
    .click();
  const summary = page
    .locator("summary")
    .filter({ hasText: "RPC stream disconnected" });
  await expect(summary).toHaveCount(1);
  await summary.click();
  await expect(page.locator("details[open] pre")).toContainText(
    '"closeCode": 1006',
  );
  await page.reload();
  await expect(
    page.locator("summary").filter({ hasText: "RPC stream disconnected" }),
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
    p.locator("summary").filter({ hasText: "RPC stream disconnected" });
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
    page.locator("summary").filter({ hasText: "Client rendering failed" }),
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
