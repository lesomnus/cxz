import { expect, test, devices, type Page } from "@playwright/test";

test.use({
  ...devices["Desktop Chrome"],
  viewport: { width: 1440, height: 1000 },
});

async function ready(page: Page) {
  await page.goto("/sandbox.html");
  await expect(
    page.getByRole("heading", { name: "Current status" }),
  ).toBeVisible({ timeout: 45000 });
}

// Freeze real animations at intermediate frames without adding app test hooks.
async function probe(page: Page) {
  await page.evaluate(() => {
    const state = { exit: undefined, arrival: undefined, entered: false } as {
      exit?: Animation;
      arrival?: Animation;
      entered: boolean;
    };
    (window as any).sendMotionProbe = state;
    const observer = new MutationObserver(() => {
      const exit = document
        .querySelector(".composer-send-ghost .editor-mirror")
        ?.getAnimations()[0];
      if (exit && !state.exit) {
        state.exit = exit;
        exit.pause();
        exit.currentTime =
          Number(exit.effect!.getComputedTiming().duration) / 2;
      }
      const arrival = document
        .querySelector(".input[data-send-arrival] .input-box")
        ?.getAnimations()[0];
      // Virtual rows can remount while the accepted input reaches Latest.
      if (arrival) state.arrival = arrival;
    });
    observer.observe(document.querySelector(".conversation")!, {
      childList: true,
      subtree: true,
      attributes: true,
      attributeFilter: ["data-send-arrival"],
    });
    function sample() {
      if (state.arrival?.playState === "running") {
        state.arrival.pause();
        state.arrival.currentTime =
          Number(state.arrival.effect!.getComputedTiming().duration) / 2;
        state.entered = true;
        observer.disconnect();
      } else requestAnimationFrame(sample);
    }
    requestAnimationFrame(sample);
  });
}

test("Send carries visible draft content upward and reveals one real message without changing measured row height", async ({
  page,
}) => {
  await ready(page);
  const input = page.getByRole("textbox", { name: "Message", exact: true });
  const text = "Send motion `code`\nSecond line";
  await input.fill(text);
  const initialHeight = (await input.boundingBox())!.height;
  const geometry = () =>
    page.locator(".composer-input").evaluate((el) => {
      const box = el.getBoundingClientRect();
      const transcript = document
        .querySelector(".transcript-area")!
        .getBoundingClientRect();
      return {
        top: box.top,
        height: box.height,
        transcriptHeight: transcript.height,
      };
    });
  const initialGeometry = await geometry();
  await probe(page);
  await page.getByRole("button", { name: "Send", exact: true }).click();
  await expect(page.locator(".composer-send-ghost")).toHaveCount(1);
  await expect(input).toHaveValue(text);
  const outgoing = await page
    .locator(".composer-send-ghost .editor-mirror")
    .evaluate((el) => {
      const style = getComputedStyle(el);
      const matrix = new DOMMatrix(style.transform);
      return { y: matrix.m42, scale: matrix.a, opacity: Number(style.opacity) };
    });
  expect(outgoing.y).toBeLessThan(0);
  expect(outgoing.scale).toBeLessThan(1);
  expect(outgoing.opacity).toBeGreaterThan(0);
  expect(outgoing.opacity).toBeLessThan(1);
  expect((await input.boundingBox())!.height).toBe(initialHeight);
  expect(await geometry()).toEqual(initialGeometry);
  await page.evaluate(() => (window as any).sendMotionProbe.exit.play());
  await page.waitForFunction(() => (window as any).sendMotionProbe.entered);
  await expect(input).toHaveValue("");
  expect(await geometry()).toEqual(initialGeometry);
  const row = page.locator("article.input").filter({ hasText: text });
  await expect(row).toHaveCount(1);
  const rowHeight = await row.evaluate((el: HTMLElement) => el.offsetHeight);
  const incoming = await row.locator(".input-box").evaluate((el) => {
    const style = getComputedStyle(el);
    const matrix = new DOMMatrix(style.transform);
    return { y: matrix.m42, scale: matrix.a, opacity: Number(style.opacity) };
  });
  expect(incoming.y).toBeGreaterThan(0);
  expect(incoming.scale).toBeLessThan(1);
  expect(incoming.opacity).toBeGreaterThan(0);
  expect(incoming.opacity).toBeLessThan(1);
  await page.evaluate(() => (window as any).sendMotionProbe.arrival.play());
  await expect(row).not.toHaveAttribute("data-send-arrival");
  expect(await row.evaluate((el: HTMLElement) => el.offsetHeight)).toBe(
    rowHeight,
  );
  await expect(page.locator(".composer-send-ghost")).toHaveCount(0);
  await expect(page.locator(".composer-input")).not.toHaveAttribute(
    "data-send-exiting",
  );
  await expect(row.locator(".message-body")).toHaveText(text);
});

test("failed sends preserve the draft and reduced motion sends without decorative copies", async ({
  page,
}) => {
  await ready(page);
  const input = page.getByRole("textbox", { name: "Message", exact: true });
  await input.fill("/effort invalid");
  await input.press("Control+Enter");
  await expect(page.getByRole("alert")).toContainText("effort not supported");
  await expect(input).toHaveValue("/effort invalid");
  await expect(input).toBeEditable();
  await expect(page.locator(".composer-input")).toHaveAttribute(
    "aria-busy",
    "false",
  );
  await expect(
    page.locator(
      ".composer-send-ghost, [data-send-arrival], [data-send-exiting]",
    ),
  ).toHaveCount(0);
  await page.emulateMedia({ reducedMotion: "reduce" });
  await input.fill("Reduced motion message");
  await input.press("Control+Enter");
  await expect(input).toHaveValue("");
  await expect(page.locator("article.input .message-body").last()).toHaveText(
    "Reduced motion message",
  );
  await expect(
    page.locator(
      ".composer-send-ghost, [data-send-arrival], [data-send-exiting]",
    ),
  ).toHaveCount(0);
});

test("editing during departure preserves the next draft across session switches", async ({
  page,
}) => {
  await ready(page);
  const input = page.getByRole("textbox", { name: "Message", exact: true });
  await input.fill("Sent draft");
  await probe(page);
  await input.press("Control+Enter");
  await expect(page.locator(".composer-send-ghost")).toHaveCount(1);
  // Acceptance restores editing before the decorative departure or agent turn ends.
  await expect(input).toBeEditable();
  await input.fill("Next draft");
  await expect(page.locator(".composer-send-ghost")).toHaveCount(0);
  await expect(input).toHaveValue("Next draft");
  await expect(page.locator("article.input .message-body").last()).toHaveText(
    "Sent draft",
  );
  await page.getByLabel("Scenario", { exact: true }).selectOption("session-2");
  await expect(input).toHaveValue("");
  await page.getByLabel("Scenario", { exact: true }).selectOption("session-1");
  await expect(input).toHaveValue("Next draft");
  await expect(
    page.locator(
      ".composer-send-ghost, [data-send-arrival], [data-send-exiting]",
    ),
  ).toHaveCount(0);
});

test("pending sends lock native and custom edits until confirmation", async ({
  page,
}) => {
  await ready(page);
  const input = page.getByRole("textbox", { name: "Message", exact: true });
  const text = "- Pending draft\n```js\nconst n = 1;\n```";
  await input.fill(text);
  const surface = page.locator(".composer-input");
  const idleBackground = await surface.evaluate(
    (el) => getComputedStyle(el).backgroundColor,
  );
  // Hold outgoing binary RPC packets at the real WASM transport boundary.
  // The app has no special test transport or delay hooks.
  async function hold() {
    await page.evaluate(() => {
      const post = MessagePort.prototype.postMessage;
      const queued: (() => void)[] = [];
      MessagePort.prototype.postMessage = function (...args: any[]) {
        if (ArrayBuffer.isView(args[0]) || args[0] instanceof ArrayBuffer)
          queued.push(() => post.apply(this, args as any));
        else post.apply(this, args as any);
      };
      (window as any).releaseSend = () => {
        MessagePort.prototype.postMessage = post;
        queued.forEach((send) => send());
      };
    });
  }
  await hold();
  await page.getByRole("button", { name: "Send", exact: true }).click();
  await expect(input).not.toBeEditable();
  await expect(surface).toHaveAttribute("aria-busy", "true");
  await expect
    .poll(() => surface.evaluate((el) => getComputedStyle(el).backgroundColor))
    .not.toBe(idleBackground);
  await input.focus();
  await input.pressSequentially("cannot edit");
  await input.press("Tab");
  await input.press("Enter");
  await input.evaluate((el) => {
    const clipboardData = new DataTransfer();
    clipboardData.setData("text/plain", "one\ntwo\nthree\nfour");
    el.dispatchEvent(
      new ClipboardEvent("paste", {
        clipboardData,
        bubbles: true,
        cancelable: true,
      }),
    );
  });
  await expect(input).toHaveValue(text);
  await expect(
    page.getByLabel("Code syntax 1", { exact: true }),
  ).toBeDisabled();
  await page.evaluate(() => (window as any).releaseSend());
  await expect(input).toBeEditable();
  await expect(input).toHaveValue("");
  await expect(surface).toHaveAttribute("aria-busy", "false");
  await expect
    .poll(() => surface.evaluate((el) => getComputedStyle(el).backgroundColor))
    .toBe(idleBackground);
});

test("leaving during departure clears the accepted saved draft and cancels the decorative layers", async ({
  page,
}) => {
  await ready(page);
  const input = page.getByRole("textbox", { name: "Message", exact: true });
  await input.fill("Switch while sending");
  await probe(page);
  await input.press("Control+Enter");
  await expect(page.locator(".composer-send-ghost")).toHaveCount(1);
  await page.getByLabel("Scenario", { exact: true }).selectOption("session-2");
  await expect(input).toHaveValue("");
  await page.getByLabel("Scenario", { exact: true }).selectOption("session-1");
  await expect(input).toHaveValue("");
  await expect(page.locator("article.input .message-body").last()).toHaveText(
    "Switch while sending",
  );
  await expect(
    page.locator(
      ".composer-send-ghost, [data-send-arrival], [data-send-exiting]",
    ),
  ).toHaveCount(0);
});
