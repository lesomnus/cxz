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
  const cells = await grid.getByRole("gridcell").evaluateAll((els) =>
    els.map((el) => ({
      label: el.getAttribute("aria-label")!,
      rank: Number(el.getAttribute("data-rank")),
      row: Number(el.parentElement!.getAttribute("aria-rowindex")),
      col: Number(el.getAttribute("aria-colindex")),
    })),
  );
  const best = cells.find((cell) => cell.rank === 0)!;
  await expect(selected).toHaveAttribute("aria-label", best.label);
  expect(best.row).toBe(Number(await grid.getAttribute("aria-rowcount")));
  const horizontal = best.col < cols ? "ArrowRight" : "ArrowLeft";
  const neighbor = cells.find(
    (cell) =>
      cell.row === best.row &&
      cell.col === best.col + (horizontal === "ArrowRight" ? 1 : -1),
  )!;
  await input.press(horizontal);
  await expect(selected).toHaveAttribute("aria-label", neighbor.label);
  const upper = cells.find(
    (cell) => cell.row === neighbor.row - 1 && cell.col === neighbor.col,
  )!;
  await input.press("ArrowUp");
  await expect(selected).toHaveAttribute("aria-label", upper.label);
  await input.press("ArrowDown");
  await expect(selected).toHaveAttribute("aria-label", neighbor.label);
  await input.press(horizontal === "ArrowRight" ? "ArrowLeft" : "ArrowRight");
  await expect(selected).toHaveAttribute("aria-label", best.label);
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
  const caretX = await input.evaluate((el: HTMLTextAreaElement) => {
    const css = getComputedStyle(el),
      canvas = document.createElement("canvas"),
      context = canvas.getContext("2d")!;
    context.font = `${css.fontStyle} ${css.fontWeight} ${css.fontSize} ${css.fontFamily}`;
    context.fontKerning = css.fontKerning as CanvasFontKerning;
    const line = el.value.slice(0, el.selectionStart).split("\n").at(-1)!;
    return (
      el.getBoundingClientRect().left +
      parseFloat(css.paddingLeft) +
      context.measureText(line).width
    );
  });
  const distances = await grid.getByRole("gridcell").evaluateAll(
    (els, caret) =>
      els.map((el) => {
        const rect = el.getBoundingClientRect();
        return {
          rank: Number(el.getAttribute("data-rank")),
          distance:
            (rect.x + rect.width / 2 - caret.x) ** 2 +
            (rect.y + rect.height / 2 - caret.y) ** 2,
        };
      }),
    { x: caretX, y: lineTop + line.height / 2 },
  );
  const ranked = distances.sort((a, b) => a.rank - b.rank);
  for (let i = 1; i < ranked.length; i++)
    expect(ranked[i].distance + 1).toBeGreaterThanOrEqual(
      ranked[i - 1].distance,
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
  await expect
    .poll(() => grid.evaluate((el) => el.scrollTop))
    .toBeGreaterThan(0);
  const initialScroll = await grid.evaluate((el) => el.scrollTop);
  for (let i = 0; i < 20; i++) await input.press("ArrowUp");
  expect(await grid.evaluate((el) => el.scrollTop)).toBeLessThan(initialScroll);
  await input.fill("Inspect `/workspace/r");
  await expect(selected).toHaveAttribute("aria-label", "/workspace/README.md");
  const mobile = (await grid.boundingBox())!;
  expect(mobile.x).toBeGreaterThanOrEqual(0);
  expect(mobile.x + mobile.width).toBeLessThanOrEqual(390);
  await input.press("Enter");
  await expect(input).toHaveValue("Inspect `/workspace/README.md");
  await expect(grid).toHaveCount(0);
});

test("radial hints follow horizontal caret motion and reverse the anchor when opening below", async ({
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
  await input.fill("Inspect `/workspace/");
  await expect(grid.getByRole("gridcell")).toHaveCount(29);
  const initial = Number(await selected.getAttribute("aria-colindex"));
  await input.fill("word ".repeat(8) + "Inspect `/workspace/");
  await expect
    .poll(async () => Number(await selected.getAttribute("aria-colindex")))
    .toBeGreaterThan(initial);
  await expect(selected).toHaveAttribute("data-rank", "0");
  await page.addStyleTag({
    content:
      "form.composer { position: fixed !important; top: 0; bottom: auto; left: 0; right: 0; }",
  });
  await input.fill("Inspect `/workspace/");
  await expect
    .poll(() =>
      selected.evaluate((el) =>
        el.parentElement!.getAttribute("aria-rowindex"),
      ),
    )
    .toBe("1");
  const bounds = (await grid.boundingBox())!,
    text = (await input.boundingBox())!;
  expect(bounds.y).toBeGreaterThan(text.y);
  await input.press("ArrowDown");
  await expect(selected).not.toHaveAttribute("data-rank", "0");
  await input.press("ArrowUp");
  await expect(selected).toHaveAttribute("data-rank", "0");
  await input.press("Tab");
  await expect(input).toHaveValue("Inspect `/workspace/bin/");
});
