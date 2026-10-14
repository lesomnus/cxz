import { expect, test, type Page } from "@playwright/test";

async function pixel(page: Page, image: Buffer, x: number, y: number) {
  return page.evaluate(
    async ({ data, x, y }) => {
      const image = new Image();
      image.src = `data:image/png;base64,${data}`;
      await image.decode();
      const canvas = document.createElement("canvas");
      canvas.width = image.width;
      canvas.height = image.height;
      const context = canvas.getContext("2d")!;
      context.drawImage(image, 0, 0);
      return [...context.getImageData(x, y, 1, 1).data];
    },
    { data: image.toString("base64"), x, y },
  );
}

test("clearance reveals the real patterned background and follows direct badge movement", async ({
  page,
}) => {
  await page.goto(
    "/iframe.html?id=components-badgedicon--transparent-clearance&viewMode=story",
  );
  const icon = page.locator("[data-badged-icon]");
  await expect(icon.locator("mask ellipse")).toHaveAttribute("cx", "36.000");
  const bounds = (await icon.boundingBox())!;
  const before = await page.screenshot({ clip: bounds });
  await icon.evaluate(
    (node) => ((node as HTMLElement).style.visibility = "hidden"),
  );
  const backdrop = await page.screenshot({ clip: bounds });
  await icon.evaluate((node) => ((node as HTMLElement).style.visibility = ""));
  expect(await pixel(page, before, 48, 20)).toEqual(
    await pixel(page, backdrop, 48, 20),
  );
  expect(await pixel(page, before, 5, 55)).toEqual([255, 255, 255, 255]);
  expect(await pixel(page, before, 36, 20)).toEqual([230, 92, 100, 255]);
  await icon.locator("[data-badged-icon-badge]").evaluate((node) => {
    Object.assign((node as HTMLElement).style, { left: "20px", top: "40px" });
  });
  await expect(icon.locator("mask ellipse")).toHaveAttribute("cx", "20.000");
  await expect(icon.locator("mask ellipse")).toHaveAttribute("cy", "40.000");
  const after = await page.screenshot({ clip: bounds });
  expect(await pixel(page, after, 32, 40)).toEqual(
    await pixel(page, backdrop, 32, 40),
  );
  expect(await pixel(page, after, 36, 20)).toEqual([255, 255, 255, 255]);
});

test("moving and hiding the badge preserves the icon node and follows its rendered stroke", async ({
  page,
}) => {
  await page.goto(
    "/iframe.html?id=components-badgedicon--moving-badge&viewMode=story",
  );
  const icon = page.getByTestId("moving-icon");
  await expect(icon.locator("mask ellipse")).toHaveAttribute(
    "stroke-width",
    "8.000",
  );
  await icon.locator("[data-badged-icon-content] > svg").evaluate((node) => {
    (window as any).originalBadgeIcon = { node, html: node.outerHTML };
  });
  await page.getByRole("button", { name: "Move badge", exact: true }).click();
  await expect(icon.locator("mask ellipse")).toHaveAttribute("cx", "16.000");
  await expect(icon.locator("mask ellipse")).toHaveAttribute("cy", "48.000");
  expect(
    await icon.locator("[data-badged-icon-content] > svg").evaluate((node) => {
      const original = (window as any).originalBadgeIcon;
      return node === original.node && node.outerHTML === original.html;
    }),
  ).toBe(true);
  await icon.locator("[data-badged-icon-content] > svg").evaluate((node) => {
    Object.assign((node as SVGElement).style, {
      width: "96px",
      height: "96px",
    });
  });
  await expect(icon.locator("mask ellipse")).toHaveAttribute(
    "stroke-width",
    "12.000",
  );
  await page.getByRole("button", { name: "Toggle badge", exact: true }).click();
  await expect(icon.locator("mask")).toHaveCount(0);
  await expect(icon.locator("[data-badged-icon-content]")).toHaveCSS(
    "mask-image",
    "none",
  );
});

test("rounded badges and CSS animation keep the mask aligned", async ({
  page,
}) => {
  const errors: string[] = [];
  page.on("pageerror", (error) => errors.push(error.message));
  await page.goto(
    "/iframe.html?id=components-badgedicon--rounded-rectangle&viewMode=story",
  );
  await expect(page.locator("mask rect[stroke]")).toHaveAttribute(
    "width",
    "32.000",
  );
  await expect(page.locator("mask rect[stroke]")).toHaveAttribute(
    "rx",
    "6.000",
  );
  await page.goto(
    "/iframe.html?id=components-badgedicon--css-motion&viewMode=story",
  );
  await expect(page.locator("mask ellipse")).toHaveAttribute(
    "stroke-width",
    "8.000",
  );
  const samples = await page.evaluate(async () => {
    const samples: {
      cx: number;
      cy: number;
      expected: number;
      expectedY: number;
    }[] = [];
    for (let index = 0; index < 20; index++) {
      // Sample after all frame callbacks (including the mask update), rather
      // than depending on this test's callback registration order.
      await new Promise<void>((resolve) =>
        requestAnimationFrame(() => setTimeout(resolve, 0)),
      );
      const icon = document
        .querySelector("[data-badged-icon-content]")!
        .getBoundingClientRect();
      const badge = document
        .querySelector("[data-badged-icon-badge]")!
        .getBoundingClientRect();
      const hole = document.querySelector("mask ellipse")!;
      samples.push({
        cx: Number(hole.getAttribute("cx")),
        cy: Number(hole.getAttribute("cy")),
        expected: badge.left + badge.width / 2 - icon.left,
        expectedY: badge.top + badge.height / 2 - icon.top,
      });
    }
    return samples;
  });
  expect(
    samples.every(
      (sample) =>
        Math.abs(sample.cx - sample.expected) < 0.2 &&
        Math.abs(sample.cy - sample.expectedY) < 0.2,
    ),
  ).toBe(true);
  expect(
    Math.hypot(
      samples[0].cx - samples.at(-1)!.cx,
      samples[0].cy - samples.at(-1)!.cy,
    ),
  ).toBeGreaterThan(1);
  await page.emulateMedia({ reducedMotion: "reduce" });
  await expect(page.locator("[data-badged-icon-badge]")).toHaveCSS(
    "animation-name",
    "none",
  );
  expect(errors).toEqual([]);
});

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
