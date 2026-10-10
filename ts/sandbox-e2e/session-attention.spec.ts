import { test, expect, devices, type Page } from "@playwright/test";

test.use({
  ...devices["Desktop Chrome"],
  viewport: { width: 1440, height: 1100 },
});
async function ready(page: Page) {
  await page.goto("/sandbox.html");
  await expect(
    page.getByRole("heading", { name: "Current status" }),
  ).toBeVisible({ timeout: 45000 });
}
async function audioProbe(page: Page) {
  await page.addInitScript(() => {
    const NativeContext = window.AudioContext;
    (window as any).soundStarts = [];
    (window as any).permissionRequests = 0;
    if (typeof Notification !== "undefined")
      Notification.requestPermission = async () => {
        (window as any).permissionRequests++;
        return "denied";
      };
    window.AudioContext = class extends NativeContext {
      override createBufferSource() {
        const source = super.createBufferSource();
        const start = source.start.bind(source);
        source.start = (...args) => {
          (window as any).soundStarts.push(source.buffer?.duration);
          start(...args);
        };
        return source;
      }
    };
  });
}

test("an offscreen completion marks its session, changes the tab and sounds once; reading Latest clears it", async ({
  page,
}) => {
  await audioProbe(page);
  await ready(page);
  const input = page.getByRole("textbox", { name: "Message", exact: true });
  await input.fill("Complete this while I read another session.");
  await input.press("Control+Enter");
  await expect(input).toBeEditable();
  await page.getByLabel("Scenario", { exact: true }).selectOption("session-2");
  const session = page.getByRole("link", { name: /Project checklist/ });
  await expect(
    session.getByRole("img", { name: "Unread response" }),
  ).toBeVisible();
  await expect.poll(() => page.title()).toMatch(/^\(\d+\)/);
  await expect
    .poll(() => page.evaluate(() => (window as any).soundStarts.length))
    .toBe(1);
  expect(await page.evaluate(() => (window as any).permissionRequests)).toBe(0);
  await page.getByLabel("Scenario", { exact: true }).selectOption("session-1");
  await expect(
    page.getByRole("heading", { name: "Current status" }).last(),
  ).toBeVisible();
  await expect(
    session.getByRole("img", { name: "Unread response" }),
  ).toHaveCount(0);
  expect(await page.evaluate(() => (window as any).soundStarts.length)).toBe(1);
});

test("new Questions receive tab attention and a sound; the settings switch can silence them", async ({
  page,
}) => {
  await audioProbe(page);
  await ready(page);
  await page.getByLabel("Scenario", { exact: true }).selectOption("session-4");
  const question = page.getByRole("region", { name: "Question", exact: true });
  await expect(question).toBeVisible();
  await question.getByRole("radio").first().check();
  await question.getByRole("button", { name: "Submit", exact: true }).click();
  await expect(question).toHaveCount(0);
  const input = page.getByRole("textbox", { name: "Message", exact: true });
  await expect(input).toBeEditable();
  await input.fill("Ask me again.");
  await input.press("Control+Enter");
  await page.getByLabel("Scenario", { exact: true }).selectOption("session-2");
  await expect(
    page
      .getByRole("link", { name: /Approval question/ })
      .getByRole("img", { name: "Awaiting answer" }),
  ).toBeVisible();
  await expect.poll(() => page.title()).toMatch(/^\(\d+\)/);
  await expect
    .poll(() => page.evaluate(() => (window as any).soundStarts.length))
    .toBeGreaterThan(0);
  await page.getByRole("link", { name: "Settings view" }).click();
  const sound = page.getByRole("switch", { name: "Notification sounds" });
  await expect(sound).toHaveAttribute("aria-checked", "true");
  await sound.click();
  expect(
    await page.evaluate(
      () =>
        JSON.parse(localStorage.getItem("settings")!)["notifications.sound"],
    ),
  ).toBe(false);
});

test("session drafts and paste source survive refresh and accepted sends remove the saved draft", async ({
  page,
}) => {
  await ready(page);
  const input = page.getByRole("textbox", { name: "Message", exact: true });
  await input.fill("한글 draft\n```js\nconst saved = true;\n```");
  await input.evaluate((el) => {
    const clipboardData = new DataTransfer();
    clipboardData.setData(
      "text/plain",
      "Saved paste source\nSecond line\nThird line\nFourth line",
    );
    el.dispatchEvent(
      new ClipboardEvent("paste", {
        clipboardData,
        bubbles: true,
        cancelable: true,
      }),
    );
  });
  await expect(page.locator(".paste-chip")).toHaveCount(1);
  await expect(page.locator(".paste-chip")).toHaveAttribute(
    "data-upload-state",
    "ready",
  );
  const text = await input.inputValue();
  await page.reload();
  await expect(input).toHaveValue(text, { timeout: 45000 });
  await expect(page.locator(".paste-chip")).toHaveCount(1);
  await page.locator(".paste-chip").click();
  await expect(
    page.getByRole("dialog", { name: "Paste source" }),
  ).toContainText("Saved paste source");
  await page
    .getByRole("dialog", { name: "Paste source" })
    .getByRole("button", { name: "Close" })
    .click();
  await input.fill("Accepted draft");
  await input.press("Control+Enter");
  await expect(input).toHaveValue("");
  await page.reload();
  await expect(input).toBeVisible({ timeout: 45000 });
  await expect(input).toHaveValue("");
});

test("an expanded composer contracts through intermediate heights after Send", async ({
  page,
}) => {
  await ready(page);
  const input = page.getByRole("textbox", { name: "Message", exact: true });
  const baseline = (await input.boundingBox())!.height;
  await input.fill(
    Array.from({ length: 30 }, (_, i) => `Line ${i}`).join("\n"),
  );
  await expect(input).toHaveAttribute("data-expanded", "true");
  await expect
    .poll(async () => (await input.boundingBox())!.height)
    .toBeGreaterThan(baseline * 2);
  await input.evaluate((node) => {
    const samples: number[] = [];
    (window as any).composerHeights = samples;
    const start = performance.now();
    const sample = () => {
      samples.push(node.getBoundingClientRect().height);
      if (performance.now() - start < 2000) requestAnimationFrame(sample);
    };
    requestAnimationFrame(sample);
  });
  await input.press("Control+Enter");
  await expect(input).toHaveValue("");
  await expect
    .poll(async () => (await input.boundingBox())!.height)
    .toBe(baseline);
  const heights = await page.evaluate(
    () => (window as any).composerHeights as number[],
  );
  expect(
    heights.filter(
      (height) => height > baseline + 2 && height < Math.max(...heights) - 2,
    ).length,
  ).toBeGreaterThan(2);
});
