import { expect, test, type Page } from "@playwright/test";

type Result = { transcript: string; final: boolean };
type ResultEvent = {
  resultIndex: number;
  results: { isFinal: boolean; 0: { transcript: string } }[];
};
type TestRecognition = {
  lang: string;
  continuous: boolean;
  interimResults: boolean;
  stops: number;
  aborts: number;
  onresult: ((event: ResultEvent) => void) | null;
  onerror: ((event: { error: string }) => void) | null;
  onstart: (() => void) | null;
  result(results: Result[]): void;
  end(): void;
};
declare global {
  interface Window {
    voiceTest: {
      instances: TestRecognition[];
      lateResult?: (event: ResultEvent) => void;
      deferStart?: boolean;
      startError?: string;
    };
  }
}

test.use({
  viewport: { width: 1440, height: 1000 },
  isMobile: false,
  hasTouch: false,
});

async function mockRecognition(page: Page, api: string) {
  await page.addInitScript((api) => {
    class MockRecognition {
      lang = "";
      continuous = false;
      interimResults = false;
      stops = 0;
      aborts = 0;
      onstart: (() => void) | null = null;
      onend: (() => void) | null = null;
      onresult: ((event: ResultEvent) => void) | null = null;
      onerror: ((event: { error: string }) => void) | null = null;
      constructor() {
        window.voiceTest.instances.push(this);
      }
      start() {
        if (window.voiceTest.startError)
          throw new DOMException(
            "Recognition service is unavailable",
            window.voiceTest.startError,
          );
        if (window.voiceTest.deferStart) return;
        this.onstart?.();
      }
      stop() {
        this.stops++;
      }
      abort() {
        this.aborts++;
      }
      end() {
        this.onend?.();
      }
      result(results: Result[]) {
        this.onresult?.({
          resultIndex: 0,
          results: results.map(({ transcript, final }) => ({
            isFinal: final,
            0: { transcript },
          })),
        });
      }
    }
    window.voiceTest = { instances: [] };
    Object.defineProperty(window, "SpeechRecognition", {
      configurable: true,
      value: undefined,
    });
    Object.defineProperty(window, "webkitSpeechRecognition", {
      configurable: true,
      value: undefined,
    });
    Object.defineProperty(window, api, {
      configurable: true,
      value: MockRecognition,
    });
  }, api);
}

async function openComposer(page: Page) {
  await page.goto("/sandbox.html");
  await expect(
    page.getByRole("heading", { name: "Current status" }),
  ).toBeVisible();
  return page.getByRole("textbox", { name: "Message", exact: true });
}

test("unsupported microphone is disabled but still shows its hover and keyboard hint", async ({
  page,
}) => {
  await page.addInitScript(() => {
    for (const key of ["SpeechRecognition", "webkitSpeechRecognition"])
      Object.defineProperty(window, key, {
        configurable: true,
        value: undefined,
      });
  });
  await openComposer(page);
  const microphone = page.getByRole("button", {
    name: "Voice input",
    exact: true,
  });
  await expect(microphone).toBeDisabled();
  const hint = page
    .getByRole("tooltip")
    .filter({ hasText: "Voice input is not supported" });
  await expect(hint).toBeHidden();
  await microphone.locator("..").hover();
  await expect(hint).toBeVisible();
  const toolbar = page.locator(".composer-toolbar");
  const terminalBox = await toolbar
    .getByRole("button", { name: "Terminal", exact: true })
    .boundingBox();
  const microphoneBox = await microphone.boundingBox();
  const sendBox = await toolbar
    .getByRole("button", { name: "Send", exact: true })
    .boundingBox();
  expect(terminalBox!.x).toBeLessThan(microphoneBox!.x);
  expect(microphoneBox!.x).toBeLessThan(sendBox!.x);
  await microphone.locator("..").focus();
  await expect(hint).toBeVisible();
});

test("reports the actual service error and waits for native start before showing Listening", async ({
  page,
}) => {
  await mockRecognition(page, "SpeechRecognition");
  await page.addInitScript(() => {
    Object.defineProperty(navigator, "language", {
      configurable: true,
      value: "ko",
    });
    Object.defineProperty(navigator, "languages", {
      configurable: true,
      value: ["ko", "ko-KR"],
    });
  });
  const input = await openComposer(page);
  await input.fill("keep this draft");
  await page.evaluate(() => {
    window.voiceTest.deferStart = true;
  });
  const microphone = page.getByRole("button", {
    name: "Voice input",
    exact: true,
  });
  await microphone.click();
  expect(await page.evaluate(() => window.voiceTest.instances[0].lang)).toBe(
    "ko-KR",
  );
  await expect(
    page.getByRole("tooltip").filter({ hasText: "Starting voice input…" }),
  ).toBeVisible();
  await expect(
    page.getByRole("tooltip").filter({ hasText: "Listening…" }),
  ).toHaveCount(0);
  await page.evaluate(() => window.voiceTest.instances[0].onstart?.());
  await expect(
    page.getByRole("tooltip").filter({ hasText: "Listening…" }),
  ).toBeVisible();
  await page.evaluate(() =>
    window.voiceTest.instances[0].onerror?.({ error: "network" }),
  );
  const hint = page.getByRole("tooltip").filter({
    hasText: "speech recognition service is unavailable or unreachable",
  });
  await expect(hint).toBeVisible();
  await expect(hint).toContainText("network · ko-KR");
  await expect(input).toHaveValue("keep this draft");
  await page.evaluate(() => {
    window.voiceTest.deferStart = false;
  });
  for (const [code, text] of [
    ["service-not-allowed", "does not allow its speech recognition service"],
    ["language-not-supported", "does not support the selected language"],
    ["aborted", "interrupted voice input"],
  ]) {
    await microphone.click();
    await page.evaluate(
      (error) => window.voiceTest.instances.at(-1)!.onerror?.({ error }),
      code,
    );
    await expect(
      page.getByRole("tooltip").filter({ hasText: text }),
    ).toBeVisible();
    await expect(microphone.locator("..")).toHaveAttribute("data-error", code);
  }
  await page.evaluate(() => {
    window.voiceTest.startError = "NotSupportedError";
  });
  await microphone.click();
  await expect(
    page
      .getByRole("tooltip")
      .filter({ hasText: "Recognition service is unavailable" }),
  ).toBeVisible();
  await expect(microphone.locator("..")).toHaveAttribute(
    "data-error",
    "service-not-allowed",
  );
  await expect(input).toHaveValue("keep this draft");
});

for (const api of ["SpeechRecognition", "webkitSpeechRecognition"]) {
  test(`${api}: interim preview, final insertion at the caret, typing, stop and undo`, async ({
    page,
  }) => {
    await mockRecognition(page, api);
    const input = await openComposer(page);
    await input.fill("before after");
    await input.evaluate((el: HTMLTextAreaElement) =>
      el.setSelectionRange(7, 7),
    );
    await page
      .getByRole("button", { name: "Voice input", exact: true })
      .click();
    await expect(
      page.getByRole("button", { name: "Stop voice input" }),
    ).toHaveAttribute("aria-pressed", "true");
    expect(
      await page.evaluate(() => {
        const recognition = window.voiceTest.instances[0];
        return [
          recognition.lang,
          recognition.continuous,
          recognition.interimResults,
        ];
      }),
    ).toEqual([await page.evaluate(() => navigator.language), true, true]);
    await page.evaluate(() =>
      window.voiceTest.instances[0].result([
        { transcript: "안녕", final: false },
      ]),
    );
    await expect(
      page.getByRole("tooltip").filter({ hasText: "안녕" }),
    ).toBeVisible();
    await expect(input).toHaveValue("before after");
    await page.evaluate(() =>
      window.voiceTest.instances[0].result([
        { transcript: "안녕하세요", final: true },
      ]),
    );
    await expect(input).toHaveValue("before 안녕하세요 after");
    await input.press("Control+z");
    await expect(input).toHaveValue("before after");
    await input.press("End");
    await input.pressSequentially(" typed");
    // Final results stay in subsequent event lists and must not be inserted twice.
    await page.evaluate(() =>
      window.voiceTest.instances[0].result([
        { transcript: "안녕하세요", final: true },
        { transcript: "next", final: false },
      ]),
    );
    await expect(input).toHaveValue("before after typed");
    await page.getByRole("button", { name: "Stop voice input" }).click();
    expect(await page.evaluate(() => window.voiceTest.instances[0].stops)).toBe(
      1,
    );
    await page.evaluate(() =>
      window.voiceTest.instances[0].result([
        { transcript: "안녕하세요", final: true },
        { transcript: "next", final: true },
      ]),
    );
    await expect(input).toHaveValue("before after typed next");
    await page.evaluate(() => window.voiceTest.instances[0].end());
    await expect(
      page.getByRole("button", { name: "Voice input", exact: true }),
    ).toHaveAttribute("aria-pressed", "false");
  });
}

test("sending and switching sessions abort recognition and ignore late results", async ({
  page,
}) => {
  await mockRecognition(page, "SpeechRecognition");
  const input = await openComposer(page);
  await input.fill("keep this");
  await page.getByRole("button", { name: "Voice input", exact: true }).click();
  await page.evaluate(() => {
    window.voiceTest.lateResult = window.voiceTest.instances[0].onresult!;
  });
  await input.press("Control+Enter");
  await expect(input).toHaveValue("");
  expect(await page.evaluate(() => window.voiceTest.instances[0].aborts)).toBe(
    1,
  );
  await page.evaluate(() =>
    window.voiceTest.lateResult?.({
      resultIndex: 0,
      results: [{ isFinal: true, 0: { transcript: "late text" } }],
    }),
  );
  await expect(input).toHaveValue("");
  await page.getByRole("button", { name: "Voice input", exact: true }).click();
  await page.getByLabel("Scenario", { exact: true }).selectOption("session-4");
  expect(await page.evaluate(() => window.voiceTest.instances[1].aborts)).toBe(
    1,
  );
});

test("permission and recognition errors show a retryable hint without changing the draft", async ({
  page,
}) => {
  await mockRecognition(page, "SpeechRecognition");
  const input = await openComposer(page);
  await input.fill("preserved");
  await page.getByRole("button", { name: "Voice input", exact: true }).click();
  await page.evaluate(() =>
    window.voiceTest.instances[0].onerror?.({ error: "not-allowed" }),
  );
  await expect(
    page
      .getByRole("tooltip")
      .filter({ hasText: "Microphone access was denied" }),
  ).toBeVisible();
  await expect(input).toHaveValue("preserved");
  await expect(
    page.getByRole("button", { name: "Voice input", exact: true }),
  ).toBeEnabled();
  await page.getByRole("button", { name: "Voice input", exact: true }).click();
  await expect(
    page.getByRole("button", { name: "Stop voice input" }),
  ).toHaveAttribute("aria-pressed", "true");
});
