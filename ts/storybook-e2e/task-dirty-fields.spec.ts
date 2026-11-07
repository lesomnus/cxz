import { expect, test, type Locator } from "@playwright/test";

const shadow = (input: Locator) =>
  input.evaluate((node) => getComputedStyle(node).boxShadow);

for (const width of [1280, 390]) {
  test(`Alias borders and focus rings fit inside horizontal clipping boundaries at ${width}px`, async ({
    page,
  }) => {
    await page.setViewportSize({ width, height: 900 });
    await page.goto(
      "/iframe.html?id=resources-tasks--task-details&viewMode=story",
    );
    const alias = page
      .getByRole("region", { name: "Task details", exact: true })
      .getByRole("textbox", { name: "Alias", exact: true });
    await expect(alias).toHaveValue("task-3");
    await alias.fill("changed-alias");
    await alias.scrollIntoViewIfNeeded();
    await alias.focus();
    const color = await alias.evaluate((input) => {
      const canvas = document.createElement("canvas");
      canvas.width = canvas.height = 1;
      const context = canvas.getContext("2d")!;
      context.fillStyle = getComputedStyle(input).getPropertyValue(
        "--ui-color-input-dirty-border",
      );
      context.fillRect(0, 0, 1, 1);
      return [...context.getImageData(0, 0, 1, 1).data];
    });
    expect(Math.max(...color.slice(0, 3))).toBeLessThan(130);
    expect(color[3]).toBe(255);
    const edges = await alias.evaluate((input) => {
      const rect = input.getBoundingClientRect();
      const style = getComputedStyle(input);
      const outset =
        parseFloat(style.outlineWidth) + parseFloat(style.outlineOffset);
      const clips = [];
      for (let node = input.parentElement; node; node = node.parentElement) {
        if (
          !["auto", "scroll", "hidden", "clip"].includes(
            getComputedStyle(node).overflowX,
          )
        )
          continue;
        const clip = node.getBoundingClientRect();
        clips.push({
          left: rect.left - outset - clip.left,
          right: clip.right - rect.right - outset,
        });
      }
      return { outset, clips };
    });
    expect(edges.outset).toBeGreaterThan(0);
    expect(edges.clips.length).toBeGreaterThan(0);
    for (const clip of edges.clips) {
      expect(clip.left).toBeGreaterThanOrEqual(-0.1);
      expect(clip.right).toBeGreaterThanOrEqual(-0.1);
    }
    expect(
      await page.evaluate(
        () => document.documentElement.scrollWidth <= innerWidth,
      ),
    ).toBe(true);
  });
}

test("unsaved fields gain a muted border independently and clear on revert, discard and save", async ({
  page,
}) => {
  await page.goto(
    "/iframe.html?id=resources-tasks--task-details&viewMode=story",
  );
  const detail = page.getByRole("region", {
    name: "Task details",
    exact: true,
  });
  const title = detail.getByRole("textbox", { name: "Title", exact: true });
  const alias = detail.getByRole("textbox", { name: "Alias", exact: true });
  const description = detail.getByRole("textbox", {
    name: "Description",
    exact: true,
  });
  const priority = detail.getByRole("combobox", {
    name: "Priority",
    exact: true,
  });
  await expect(title).toHaveValue("Connect the task table");
  const inputs = [title, alias, description, priority];
  const resting = await Promise.all(inputs.map(shadow));
  const original = await title.inputValue();
  const before = await title.boundingBox();

  await title.fill("Unsaved title");
  await expect.poll(() => shadow(title)).not.toBe(resting[0]);
  for (let i = 1; i < inputs.length; i++)
    expect(await shadow(inputs[i])).toBe(resting[i]);
  expect((await title.boundingBox())!.height).toBeCloseTo(before!.height, 2);
  expect((await title.boundingBox())!.width).toBeCloseTo(before!.width, 2);
  await title.fill(original);
  await expect.poll(() => shadow(title)).toBe(resting[0]);

  await alias.fill("unsaved-alias");
  await description.fill("Unsaved description");
  await priority.click();
  await page.getByRole("option", { name: "High", exact: true }).click();
  for (let i = 1; i < inputs.length; i++)
    await expect.poll(() => shadow(inputs[i])).not.toBe(resting[i]);
  await detail
    .getByRole("button", { name: "Discard changes", exact: true })
    .click();
  for (let i = 0; i < inputs.length; i++)
    await expect.poll(() => shadow(inputs[i])).toBe(resting[i]);
  await expect(alias).toHaveValue("task-3");
  await expect(priority).toHaveText("Normal");

  await description.fill("Saved description");
  const save = detail.getByRole("button", { name: "Save", exact: true });
  await save.click();
  await expect(save).toBeDisabled();
  await expect(description).toHaveValue("Saved description");
  for (let i = 0; i < inputs.length; i++)
    await expect.poll(() => shadow(inputs[i])).toBe(resting[i]);
});
