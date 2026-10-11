import { expect, test } from "@playwright/test";

test("composer path hints use the shared candidate grid in serverless previews", async ({
  page,
}) => {
  await page.goto(
    "/iframe.html?id=conversation-composer--path-hints&viewMode=story",
  );
  const input = page.getByRole("textbox", { name: "Message", exact: true });
  await input.fill("Inspect `/workspace/s");
  const hints = page.getByRole("grid", {
    name: "Path suggestions",
    exact: true,
  });
  await expect(hints.locator('[aria-selected="true"]')).toHaveAttribute(
    "aria-label",
    "/workspace/src/",
  );
  await input.press("Tab");
  await expect(input).toHaveValue("Inspect `/workspace/src/");
  await expect
    .poll(() => input.evaluate((el: HTMLTextAreaElement) => el.selectionStart))
    .toBe("Inspect `/workspace/src/".length);
  await expect(hints.getByRole("gridcell", { name: /app.ts/ })).toBeVisible();
  await hints.getByRole("gridcell", { name: /app.ts/ }).click();
  await expect(input).toHaveValue("Inspect `/workspace/src/app.ts");
  await expect(hints).toHaveCount(0);
});

test("path matrix navigates spatially, scrolls candidates and stays outside the typing line", async ({
  page,
}) => {
  await page.goto(
    "/iframe.html?id=conversation-composer--path-hints&viewMode=story",
  );
  const input = page.getByRole("textbox", { name: "Message", exact: true });
  const grid = page.getByRole("grid", {
    name: "Path suggestions",
    exact: true,
  });
  const selected = grid.locator('[aria-selected="true"]');
  await input.fill("First line\nInspect `/workspace/");
  await expect(grid.getByRole("gridcell")).toHaveCount(29);
  const caret = await input.evaluate(
    (el: HTMLTextAreaElement) => el.selectionStart,
  );
  const cols = Number(await grid.getAttribute("aria-colcount"));
  expect(cols).toBeGreaterThan(1);
  const names = await grid
    .getByRole("gridcell")
    .evaluateAll((els) => els.map((el) => el.getAttribute("aria-label")));
  await input.press("ArrowRight");
  await expect(selected).toHaveAttribute("aria-label", names[1]!);
  await input.press("ArrowDown");
  await expect(selected).toHaveAttribute("aria-label", names[1 + cols]!);
  await input.press("ArrowLeft");
  await expect(selected).toHaveAttribute("aria-label", names[cols]!);
  await input.press("ArrowUp");
  await expect(selected).toHaveAttribute("aria-label", names[0]!);
  expect(
    await input.evaluate((el: HTMLTextAreaElement) => el.selectionStart),
  ).toBe(caret);
  await expect(input).toHaveValue("First line\nInspect `/workspace/");
  const box = (await grid.boundingBox())!,
    text = (await input.boundingBox())!;
  const line = await input.evaluate((el) => {
    const css = getComputedStyle(el);
    return {
      padding: parseFloat(css.paddingTop),
      height: parseFloat(css.lineHeight),
    };
  });
  const lineTop = text.y + line.padding + line.height;
  expect(box.y + box.height <= lineTop || box.y >= lineTop + line.height).toBe(
    true,
  );
  const pipe = grid.getByRole("gridcell", {
    name: "/workspace/channel.pipe",
    exact: true,
  });
  const file = grid.getByRole("gridcell", {
    name: "/workspace/README.md",
    exact: true,
  });
  expect(await pipe.evaluate((el) => getComputedStyle(el).color)).not.toBe(
    await file.evaluate((el) => getComputedStyle(el).color),
  );
  await expect(pipe).toHaveAttribute("title", /Pipe/);
  await expect(
    grid.getByRole("gridcell", { name: "/workspace/run.sh", exact: true }),
  ).toHaveAttribute("title", /Executable/);
  await input.press("Escape");
  await expect(grid).toHaveCount(0);
  await expect(input).toBeFocused();
  await page.setViewportSize({ width: 390, height: 844 });
  await input.fill("Inspect `/workspace/");
  await expect(grid.getByRole("gridcell")).toHaveCount(29);
  for (let i = 0; i < 20; i++) await input.press("ArrowDown");
  expect(await grid.evaluate((el) => el.scrollTop)).toBeGreaterThan(0);
  await input.fill("Inspect `/workspace/r");
  await expect(selected).toHaveAttribute("aria-label", "/workspace/README.md");
  const mobile = (await grid.boundingBox())!;
  expect(mobile.x).toBeGreaterThanOrEqual(0);
  expect(mobile.x + mobile.width).toBeLessThanOrEqual(390);
  await input.press("Enter");
  await expect(input).toHaveValue("Inspect `/workspace/README.md");
  await expect(grid).toHaveCount(0);
});

test("reusable suggestion grid previews retain host focus", async ({
  page,
}) => {
  await page.goto(
    "/iframe.html?id=components-suggestiongrid--candidates&viewMode=story",
  );
  const input = page.getByRole("textbox", { name: "Query" });
  const grid = page.getByRole("grid", { name: "Candidates" });
  await expect(grid.getByRole("gridcell")).toHaveCount(24);
  await input.focus();
  await input.press("ArrowRight");
  await expect(grid.locator('[aria-selected="true"]')).toHaveAttribute(
    "aria-label",
    "candidate-2",
  );
  await grid
    .getByRole("gridcell", { name: "candidate-4", exact: true })
    .click();
  await expect(input).toBeFocused();
  await expect(grid.locator('[aria-selected="true"]')).toHaveAttribute(
    "aria-label",
    "candidate-4",
  );
});
