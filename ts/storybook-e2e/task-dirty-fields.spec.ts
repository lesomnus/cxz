import { expect, test, type Locator } from "@playwright/test";

const shadow = (input: Locator) =>
  input.evaluate((node) => getComputedStyle(node).boxShadow);

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
