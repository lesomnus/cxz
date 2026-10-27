import { expect, test } from "@playwright/test";

test("session notification badge uses the wrapper without changing its icon", async ({
  page,
}) => {
  await page.goto(
    "/iframe.html?id=navigation-resourceicon--unread&viewMode=story",
  );
  await expect(
    page.getByRole("img", { name: "1 unread sessions" }),
  ).toBeVisible();
  const icon = page.locator("[data-badged-icon]");
  await expect(icon.locator("mask ellipse")).toHaveAttribute("cx", "20.000");
  await expect(icon.locator("mask ellipse")).toHaveAttribute("cy", "2.000");
  await expect(icon.locator("mask ellipse")).toHaveAttribute(
    "stroke-width",
    "2.500",
  );
  await expect(
    icon.locator("[data-badged-icon-content] > svg"),
  ).toHaveAttribute("stroke-width", "1.5");
});

test("recording badge masks the microphone instead of painting a background border", async ({
  page,
}) => {
  await page.addInitScript(() => {
    class Recognition {
      onstart?: () => void;
      onend?: () => void;
      start() {
        this.onstart?.();
      }
      stop() {
        this.onend?.();
      }
      abort() {
        this.onend?.();
      }
    }
    Object.defineProperty(window, "SpeechRecognition", {
      value: Recognition,
      configurable: true,
    });
    if (navigator.mediaDevices)
      Object.defineProperty(navigator.mediaDevices, "getUserMedia", {
        value: async () => {
          throw new Error("Synthetic meter unavailable");
        },
        configurable: true,
      });
  });
  await page.goto(
    "/iframe.html?id=conversation-composer--voice-input&viewMode=story",
  );
  await page.getByRole("button", { name: "Voice input", exact: true }).click();
  const stop = page.getByRole("button", {
    name: "Stop voice input",
    exact: true,
  });
  await expect(stop.locator("[data-voice-recording]")).toBeVisible();
  await expect(stop.locator("mask ellipse")).toHaveAttribute(
    "stroke-width",
    "2.400",
  );
  await expect(stop.locator("[data-voice-recording]")).toHaveCSS(
    "border-top-width",
    "0px",
  );
  await expect(
    stop.locator("[data-badged-icon-content] > svg"),
  ).toHaveAttribute("stroke-width", "1.8");
  await stop.click();
  await expect(page.locator("[data-voice-recording]")).toHaveCount(0);
});
