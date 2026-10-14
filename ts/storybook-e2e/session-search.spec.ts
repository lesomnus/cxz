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
  await search.getByRole("searchbox").fill("network");
  await expect(search.getByRole("status")).toHaveText("2 matches");
  await search
    .getByRole("button", { name: /Investigate network latency/ })
    .click();
  await expect(page.getByText("Jumped to message 1")).toBeVisible();
  await expect(search).toBeVisible();
  await page.mouse.click(5, 5);
  await expect(search).toBeVisible();
  await page.keyboard.press("Escape");
  await expect(search).toHaveCount(0);
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
  await expect(search.getByRole("status")).toHaveText("2 matches");
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
  expect(
    await page.locator(".transcript").evaluate((el) => el.scrollTop),
  ).toBeLessThan(bottomMatchScroll);
  await expect(input).toBeFocused();
  await input.press("Enter");
  await expect.poll(async () => (await active())?.seq).toBe("1");
  await input.press("Enter");
  await expect
    .poll(active)
    .toEqual({ seq: "2", text: "Second network match in this reply." });
  await input.fill("[network]");
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
