import { expect, test } from "@playwright/test";

test("portable waveform responds to host levels without requesting microphone access", async ({
  page,
}) => {
  const errors: string[] = [];
  page.on("pageerror", (error) => errors.push(error.message));
  await page.addInitScript(() => {
    if (navigator.mediaDevices)
      Object.defineProperty(navigator.mediaDevices, "getUserMedia", {
        value: () => {
          throw new Error(
            "A presentation component must not open a microphone",
          );
        },
      });
  });
  await page.goto(
    "/iframe.html?id=components-audiowaveform--listening&viewMode=story",
  );
  const wave = page.locator("canvas[data-active=true]");
  await expect(wave).toBeVisible();
  const volume = page.getByRole("slider", { name: "Volume" });
  await volume.focus();
  await volume.press("Home");
  const pixels = () =>
    wave.evaluate((el: HTMLCanvasElement) => {
      const data = el
        .getContext("2d")!
        .getImageData(0, 0, el.width, el.height).data;
      let top = el.height,
        bottom = 0;
      for (let y = 0; y < el.height; y++) {
        for (let x = 0; x < el.width; x++) {
          if (data[(y * el.width + x) * 4 + 3] > 12) {
            top = Math.min(top, y);
            bottom = Math.max(bottom, y);
            break;
          }
        }
      }
      return Math.max(0, bottom - top);
    });
  await expect.poll(pixels).toBeLessThan(4);
  await volume.press("End");
  await expect.poll(pixels).toBeGreaterThan(14);
  await page.goto(
    "/iframe.html?id=components-audiowaveform--upward&viewMode=story",
  );
  await expect(wave).toHaveAttribute("data-variant", "upward");
  await volume.focus();
  await volume.press("Home");
  await expect.poll(pixels).toBeLessThan(4);
  await expect
    .poll(() =>
      wave.evaluate((el: HTMLCanvasElement) => {
        const data = el
          .getContext("2d")!
          .getImageData(0, el.height - 1, el.width, 1).data;
        return data.some((alpha, index) => index % 4 === 3 && alpha > 12);
      }),
    )
    .toBe(true);
  await volume.press("End");
  await expect.poll(pixels).toBeGreaterThan(14);
  await expect
    .poll(() =>
      wave.evaluate((el: HTMLCanvasElement) => {
        const data = el
          .getContext("2d")!
          .getImageData(0, 0, el.width, el.height).data;
        let lime = 0,
          purple = 0;
        for (let i = 0; i < data.length; i += 4) {
          if (data[i + 3] <= 12) continue;
          const [r, g, b] = [data[i], data[i + 1], data[i + 2]];
          if (g > r && r > b) lime++;
          if (b > r && r > g) purple++;
        }
        return lime > 10 && purple > 10;
      }),
    )
    .toBe(true);
  expect(errors).toEqual([]);
});
