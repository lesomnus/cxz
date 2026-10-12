import { expect, test } from "@playwright/test";

test.use({
  viewport: { width: 1280, height: 900 },
  isMobile: false,
  hasTouch: false,
});

test("unread response uses the left indicator and Sessions icon count clears when read", async ({
  page,
}) => {
  await page.request.post("/auth/login", {
    headers: { Origin: "https://127.0.0.1:18081" },
    data: { token: "a".repeat(32) },
  });
  await page.addInitScript(() => {
    sessionStorage.setItem(
      `cxz.attention.v1:${encodeURIComponent(location.origin)}`,
      JSON.stringify({
        session: {
          run: "run",
          last: "3",
          done: "3",
          seen: "0",
          notified: "3",
          pending: "[]",
        },
      }),
    );
  });
  await page.goto("/sessions");
  const row = page.locator(".tree-session").first();
  const unread = row
    .locator(".session-indicator")
    .getByRole("img", { name: "Unread response", exact: true });
  await expect(unread).toBeVisible();
  await expect(unread.locator("circle")).toHaveCSS("fill", "rgb(217, 255, 48)");
  await expect(row.locator(".session-unread")).toHaveCount(0);
  const tab = page.getByRole("link", { name: "Sessions view", exact: true });
  const count = tab.getByRole("img", {
    name: "1 unread sessions",
    exact: true,
  });
  await expect(count).toBeVisible();
  await expect(count).toHaveText("1");
  await expect(count.locator("circle")).toHaveCSS("fill", "rgb(217, 255, 48)");
  const favicon = await page
    .locator("link[data-session-attention]")
    .getAttribute("href");
  expect(decodeURIComponent(favicon!)).toContain('fill="#d9ff30"');
  await row.click();
  await expect(page.locator(".transcript")).toHaveAttribute(
    "aria-busy",
    "false",
  );
  await expect(unread).toHaveCount(0);
  await expect(count).toHaveCount(0);
  // A pending question retains its own status indicator once unread clears.
  await expect(row.locator(".session-indicator")).toHaveAttribute(
    "aria-label",
    "Awaiting answer",
  );
});
