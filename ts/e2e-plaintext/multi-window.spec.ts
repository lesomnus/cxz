import { test, expect, devices } from "@playwright/test";

test.use({
  ...devices["Desktop Chrome"],
  viewport: { width: 1280, height: 900 },
});

test("HTTP/1.1 windows keep loading and sending while other windows subscribe", async ({
  page,
  context,
  baseURL,
}) => {
  await page.request.post("/auth/login", {
    headers: { Origin: new URL(baseURL!).origin },
    data: { token: "a".repeat(32) },
  });
  const windows = [page, await context.newPage(), await context.newPage()];
  try {
    for (const window of windows) {
      await window.goto("/sessions/session");
      await expect(window.locator(".transcript")).toHaveAttribute(
        "aria-busy",
        "false",
        { timeout: 5000 },
      );
    }
    const first = windows[0];
    const input = first.getByRole("textbox", { name: "Message", exact: true });
    await input.fill("Multiple windows remain connected");
    await first.getByRole("button", { name: "Send", exact: true }).click();
    await expect(input).toHaveValue("", { timeout: 5000 });
    for (const window of windows)
      await expect(
        window
          .locator(".transcript")
          .getByText("Multiple windows remain connected", { exact: true }),
      ).toBeVisible();
    await windows[1]
      .getByRole("link", { name: "Projects view", exact: true })
      .click();
    await windows[1]
      .getByRole("link", { name: "Sessions view", exact: true })
      .click();
    await expect(windows[1].locator(".transcript")).toHaveAttribute(
      "aria-busy",
      "false",
    );
  } finally {
    for (const window of windows.slice(1)) await window.close();
  }
});
