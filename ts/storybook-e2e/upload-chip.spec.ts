import { expect, test } from "@playwright/test";

test("upload chips shimmer and fade into solid chips on completion", async ({
  page,
}) => {
  await page.goto(
    "/iframe.html?id=composer-upload-chips--upload-transition&viewMode=story",
  );
  const chips = page.locator(".paste-chip");
  await expect(chips).toHaveCount(2);
  // The shimmer must paint above the opaque inline chip background.
  expect(
    await chips
      .first()
      .evaluate((el) => getComputedStyle(el, "::before").zIndex),
  ).toBe("0");
  await expect(chips.first().locator(".paste-chip-label")).toHaveCSS(
    "z-index",
    "1",
  );
  await expect(chips.first()).toHaveAttribute("aria-busy", "true");
  const effect = () =>
    chips.first().evaluate((el) => {
      const style = getComputedStyle(el, "::before");
      return {
        opacity: style.opacity,
        playState: style.animationPlayState,
        duration: style.transitionDuration,
      };
    });
  await expect
    .poll(effect)
    .toEqual({ opacity: "1", playState: "running", duration: "0.45s" });
  await expect(chips.first()).toHaveAttribute("data-upload-state", "ready", {
    timeout: 8000,
  });
  await expect
    .poll(effect)
    .toEqual({ opacity: "0", playState: "paused", duration: "0.45s" });
  await expect(chips.first()).toHaveAttribute("aria-busy", "false");
  await page.getByRole("button", { name: "Replay upload" }).click();
  await expect(chips.first()).toHaveAttribute("data-upload-state", "uploading");
  await expect
    .poll(effect)
    .toEqual({ opacity: "1", playState: "running", duration: "0.45s" });
  await page.emulateMedia({ reducedMotion: "reduce" });
  expect(
    await chips
      .first()
      .evaluate((el) => getComputedStyle(el, "::before").animationName),
  ).toBe("none");
});
