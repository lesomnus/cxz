import { expect, test, devices } from "@playwright/test";

test.use({
  ...devices["Desktop Chrome"],
  viewport: { width: 1440, height: 1000 },
});
test("browser file drop reaches the authenticated HTTP upload bridge and sends its returned path", async ({
  page,
}) => {
  await page.request.post("/auth/login", {
    headers: { Origin: "https://127.0.0.1:18081" },
    data: { token: "a".repeat(32) },
  });
  await page.goto("/sessions/session");
  const input = page.getByRole("textbox", { name: "Message", exact: true });
  await expect(input).toBeVisible();
  const uploading = page.waitForResponse((response) =>
    response.url().includes("/attachments/session?"),
  );
  await input.evaluate((node) => {
    const data = new DataTransfer();
    data.items.add(new File([new Uint8Array([0, 255, 13, 10])], "report.bin"));
    node.dispatchEvent(
      new DragEvent("drop", {
        dataTransfer: data,
        bubbles: true,
        cancelable: true,
      }),
    );
  });
  const reply = await uploading;
  expect(reply.status()).toBe(200);
  // The raw Manager storage fixture validates binary bytes before acknowledgement.
  // Chromium's request inspector does not expose Blob bodies as postData.
  expect(reply.request().headers()["content-type"]).toBe(
    "application/octet-stream",
  );
  const path = (await reply.json()).path;
  expect(path).toMatch(/^\/cxz\/assets\/session\/[a-f0-9]+\/report\.bin$/);
  await expect(page.locator(".composer .paste-chip")).toHaveAttribute(
    "data-upload-state",
    "ready",
  );
  await input.press("Control+Enter");
  await expect(input).toHaveValue("");
  await expect(
    page.locator("article.input .message-body").last(),
  ).toContainText(
    `[Attached file: ${path} — read this file for the full content]`,
  );
});
