import { expect, test } from "@playwright/test";

test("portable elapsed timer advances without response metrics", async ({
  page,
}) => {
  await page.goto(
    "/iframe.html?id=components-elapsedtime--running&viewMode=story",
  );
  const timer = page.getByRole("timer", {
    name: "Response elapsed time",
    exact: true,
  });
  await expect(timer).toBeVisible();
  const initial = await timer.textContent();
  await expect
    .poll(() => timer.textContent(), { timeout: 3000 })
    .not.toBe(initial);
});
