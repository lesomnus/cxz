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
  // The gRPC fixture validates exact binary bytes before acknowledging upload.
  // Chromium's request inspector does not expose Blob bodies as postData.
  expect(reply.request().headers()["content-type"]).toBe(
    "application/octet-stream",
  );
  expect((await reply.json()).path).toBe(
    "/cxz/assets/session/upload/report.bin",
  );
  await expect(page.locator(".composer .paste-chip")).toHaveAttribute(
    "data-upload-state",
    "ready",
  );
  await input.press("Control+Enter");
  await expect(input).toHaveValue("");
  await expect(
    page.locator("article.input .message-body").last(),
  ).toContainText(
    "[Attached file: /cxz/assets/session/upload/report.bin — read this file for the full content]",
  );
});
