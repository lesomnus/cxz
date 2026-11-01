import { expect, test } from "@playwright/test";

test("session-search story opens, matches and jumps while keeping the overlay until Escape", async ({
  page,
}) => {
  await page.goto(
    "/iframe.html?id=conversation-sessionsearch--default&viewMode=story",
  );
  await page.getByRole("button", { name: "Search conversation" }).click();
  const search = page.getByRole("search", {
    name: "Search current conversation",
  });
  const input = search.getByRole("searchbox");
  const counter = search.getByRole("status");
  await expect(counter).toHaveAccessibleName("Result 0 of 0");
  await input.fill("network");
  await expect(counter).toHaveAccessibleName("Result 1 of 2");
  await expect(counter).toHaveText("0001/0002");
  await expect(search.locator("mark")).toHaveText(["network", "network"]);
  await input.press("ArrowDown");
  await expect(counter).toHaveAccessibleName("Result 2 of 2");
  await input.press("ArrowUp");
  await expect(counter).toHaveAccessibleName("Result 1 of 2");
  await expect(page.locator(".scroll-tick[data-search]")).toHaveCount(2);
  await search
    .getByRole("button", { name: /Investigate network latency/ })
    .click();
  await expect(page.getByText("Jumped to message 1")).toBeVisible();
  await expect(counter).toHaveAccessibleName("Result 2 of 2");
  await expect(search).toBeVisible();
  await page.mouse.click(5, 5);
  await expect(search).toBeVisible();
  await page.keyboard.press("Escape");
  await expect(search).toHaveCount(0);
  await expect(page.locator(".scroll-tick[data-search]")).toHaveCount(0);
});

test("Enter highlights each occurrence newest first, including inline Markdown, and wraps", async ({
  page,
}) => {
  await page.goto(
    "/iframe.html?id=conversation-sessionsearch--default&viewMode=story",
  );
  await page.getByRole("button", { name: "Search conversation" }).click();
  const search = page.getByRole("search", {
    name: "Search current conversation",
  });
  const input = search.getByRole("searchbox");
  await input.fill("network");
  const counter = search.getByRole("status");
  await expect(counter).toHaveAccessibleName("Result 1 of 2");
  await expect
    .poll(() =>
      page.evaluate(() => CSS.highlights.get("conversation-search")?.size),
    )
    .toBe(3);
  const active = () =>
    page.evaluate(() => {
      const range = [
        ...(CSS.highlights.get("conversation-search-current") ?? []),
      ][0] as Range | undefined;
      return range
        ? {
            seq: range.startContainer.parentElement?.closest<HTMLElement>(
              "[data-row]",
            )?.dataset.row,
            text: range.startContainer.parentElement?.textContent,
          }
        : undefined;
    });
  await input.press("Enter");
  await expect
    .poll(active)
    .toEqual({ seq: "2", text: "Second network match in this reply." });
  const bottomMatchScroll = await page
    .locator(".transcript")
    .evaluate((el) => el.scrollTop);
  await expect(input).toBeFocused();
  await input.press("Enter");
  await expect.poll(active).toEqual({ seq: "2", text: "network" });
  await expect(counter).toHaveAccessibleName("Result 1 of 2");
  expect(
    await page.locator(".transcript").evaluate((el) => el.scrollTop),
  ).toBeLessThan(bottomMatchScroll);
  await expect(input).toBeFocused();
  await input.press("Enter");
  await expect.poll(async () => (await active())?.seq).toBe("1");
  await expect(counter).toHaveAccessibleName("Result 2 of 2");
  await input.press("Enter");
  await expect
    .poll(active)
    .toEqual({ seq: "2", text: "Second network match in this reply." });
  await expect(counter).toHaveAccessibleName("Result 1 of 2");
  await input.fill("network transfer");
  await expect(counter).toHaveAccessibleName("Result 1 of 1");
  await expect(search.locator("mark")).toHaveText(["network transfer"]);
  await input.press("Enter");
  await expect
    .poll(() =>
      page.evaluate(() =>
        (
          [...(CSS.highlights.get("conversation-search-current") ?? [])][0] as
            | Range
            | undefined
        )?.toString(),
      ),
    )
    .toBe("network transfer");
  await input.fill("[network]");
  await expect(counter).toHaveAccessibleName("Result 0 of 0");
  await expect(counter).toHaveText("0000/0000");
  await expect(search.getByText("No matches", { exact: true })).toBeVisible();
  await expect(search.locator("mark")).toHaveCount(0);
  await expect
    .poll(() =>
      page.evaluate(() => CSS.highlights.get("conversation-search")?.size),
    )
    .toBe(0);
  await input.press("Escape");
  await expect
    .poll(() =>
      page.evaluate(() => CSS.highlights.has("conversation-search-current")),
    )
    .toBe(false);
});

test("local search updates when loaded responses arrive without changing the query", async ({
  page,
}) => {
  await page.goto(
    "/iframe.html?id=conversation-sessionsearch--default&viewMode=story",
  );
  await page.getByRole("button", { name: "Search conversation" }).click();
  const search = page.getByRole("search", {
      name: "Search current conversation",
    }),
    input = search.getByRole("searchbox");
  await input.fill("network");
  await expect(search.getByRole("status")).toHaveAccessibleName(
    "Result 1 of 2",
  );
  await page.getByRole("button", { name: "Append response" }).click();
  await expect(input).toHaveValue("network");
  await expect(search.getByRole("status")).toHaveAccessibleName(
    "Result 1 of 3",
  );
  await expect(search.locator("mark")).toHaveText([
    "network",
    "network",
    "network",
  ]);
  await search.locator('[data-search-index="0"]').click();
  await expect(page.getByText("Jumped to message 4")).toBeVisible();
  await expect(input).toBeFocused();
  await input.press("Escape");
  await expect(search).toHaveCount(0);
});

test("preview highlights preserve casing and match literal punctuation and repeated text", async ({
  page,
}) => {
  await page.goto(
    "/iframe.html?id=conversation-sessionsearch--with-results&viewMode=story",
  );
  const search = page.getByRole("search", {
    name: "Search current conversation",
  });
  const input = search.getByRole("searchbox");
  await expect(input).toHaveValue("network");
  const originalHeight = (
    await search.locator("[data-search-index]").first().boundingBox()
  )?.height;
  await input.fill("NETWORK");
  await expect(search.locator("mark")).toHaveText(["network", "network"]);
  await input.fill(".");
  const punctuation = search.locator("mark");
  await expect(punctuation.first()).toBeVisible();
  expect(
    (await punctuation.allTextContents()).every((text) => text === "."),
  ).toBe(true);
  await input.fill("Transfer diagnostics.");
  const repeated = search.locator("mark");
  await expect(repeated.first()).toBeVisible();
  expect(await repeated.count()).toBeGreaterThan(1);
  expect(
    (await repeated.allTextContents()).every((text) =>
      "Transfer diagnostics.".startsWith(text),
    ),
  ).toBe(true);
  const row = search.locator("[data-search-index]");
  await expect(row).toHaveCount(1);
  expect((await row.boundingBox())?.height).toBe(originalHeight);
  await row.click();
  await expect(page.getByText("Jumped to message 2")).toBeVisible();
  await input.press("Escape");
  await expect(search).toHaveCount(0);
});
