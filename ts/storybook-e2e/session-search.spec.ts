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
