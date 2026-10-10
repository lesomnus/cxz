import { expect, test } from "@playwright/test";

test("confirmation dialog traps focus, cancels without acting and fits a small viewport", async ({
  page,
}) => {
  await page.goto(
    "/iframe.html?id=components-confirmationdialog--default&viewMode=story",
  );
  const launch = page.getByRole("button", {
    name: "Open confirmation",
    exact: true,
  });
  await launch.click();
  const dialog = page.getByRole("dialog", { name: "Stop session" });
  const cancel = dialog.getByRole("button", { name: "Cancel", exact: true });
  await expect(cancel).toBeFocused();
  await page.keyboard.press("Shift+Tab");
  await expect(
    dialog.getByRole("button", { name: "Stop", exact: true }),
  ).toBeFocused();
  await page.keyboard.press("Tab");
  await expect(cancel).toBeFocused();
  await page.keyboard.press("Escape");
  await expect(dialog).toHaveCount(0);
  await expect(launch).toBeFocused();
  await expect(page.getByRole("status")).toHaveCount(0);
  await page.setViewportSize({ width: 390, height: 600 });
  await launch.click();
  const bounds = (await dialog.boundingBox())!;
  expect(bounds.x).toBeGreaterThanOrEqual(0);
  expect(bounds.x + bounds.width).toBeLessThanOrEqual(390);
  expect(bounds.y).toBeGreaterThanOrEqual(0);
  expect(bounds.y + bounds.height).toBeLessThanOrEqual(600);
  await page.screenshot({
    path: "test-results/confirmation-dialog-mobile.png",
  });
  await cancel.click();
  await launch.click();
  await dialog.getByRole("button", { name: "Stop", exact: true }).click();
  await expect(dialog).toHaveCount(0);
  await expect(page.getByRole("status")).toHaveText("Action completed");
});

test("pending dialogs prevent dismissal and failed actions stay open for retry", async ({
  page,
}) => {
  await page.goto(
    "/iframe.html?id=components-confirmationdialog--pending&viewMode=story",
  );
  await page
    .getByRole("button", { name: "Open confirmation", exact: true })
    .click();
  let dialog = page.getByRole("dialog", { name: "Stop session" });
  await dialog.getByRole("button", { name: "Stop", exact: true }).click();
  await expect(dialog).toHaveAttribute("aria-busy", "true");
  await expect(
    dialog.getByRole("button", { name: "Cancel", exact: true }),
  ).toBeDisabled();
  await expect(
    dialog.getByRole("button", { name: "Working…", exact: true }),
  ).toBeDisabled();
  await page.keyboard.press("Escape");
  await expect(dialog).toBeVisible();
  await expect(dialog).toHaveCount(0);
  await expect(page.getByRole("status")).toHaveText("Action completed");
  await page.goto(
    "/iframe.html?id=components-confirmationdialog--failure&viewMode=story",
  );
  await page
    .getByRole("button", { name: "Open confirmation", exact: true })
    .click();
  dialog = page.getByRole("dialog", { name: "Stop session" });
  await dialog.getByRole("button", { name: "Stop", exact: true }).click();
  await expect(dialog.getByRole("alert")).toHaveText(
    "Action failed. Please try again.",
  );
  await expect(
    dialog.getByRole("button", { name: "Stop", exact: true }),
  ).toBeEnabled();
  await dialog.getByRole("button", { name: "Cancel", exact: true }).click();
  await expect(dialog).toHaveCount(0);
});

test("async free-text questions submit structured native keys and only explicit actions dismiss them", async ({
  page,
}) => {
  await page.goto(
    "/iframe.html?id=conversation-questioncard--free-text&viewMode=story",
  );
  const question = page.getByRole("region", { name: "Question", exact: true });
  await expect(question).toBeVisible();
  await expect(question).not.toContainText("not supported");
  await expect(question.getByRole("radio")).toHaveCount(0);
  const submit = question.getByRole("button", {
    name: "Submit answers",
    exact: true,
  });
  await expect(submit).toBeDisabled();
  const answer = question.getByRole("textbox", {
    name: "Answer: Describe the change you want to make.",
    exact: true,
  });
  await answer.fill("first line\nsecond line");
  await expect(question.locator(".editor-gutter > div > div")).toHaveText([
    "1",
    "2",
  ]);
  await page.keyboard.press("Escape");
  await expect(question).toBeVisible();
  await submit.click();
  await expect(question).toHaveCount(0);
  expect(JSON.parse((await page.getByRole("status").innerText())!)).toEqual({
    "0": { selected: [], other: "first line\nsecond line" },
  });
  await page.getByRole("button", { name: "Ask again", exact: true }).click();
  await question.getByRole("button", { name: "Deny", exact: true }).click();
  await expect(page.getByRole("status")).toHaveText("Denied");
});

test("question tabs preserve choices, draft and undo and submit all steps together", async ({
  page,
}) => {
  await page.goto(
    "/iframe.html?id=conversation-questioncard--steps&viewMode=story",
  );
  const question = page.getByRole("region", { name: "Question", exact: true });
  const tabs = question.getByRole("tablist", { name: "Question steps" });
  const submit = question.getByRole("button", {
    name: "Submit answers",
    exact: true,
  });
  await expect(tabs.getByRole("tab")).toHaveText(["Layout", "Notes", "Timing"]);
  await question.getByRole("radio", { name: "Compact", exact: true }).check();
  await tabs.getByRole("tab", { name: "Notes", exact: true }).click();
  const answer = question.getByRole("textbox", {
    name: "Answer: Describe any additional changes.",
    exact: true,
  });
  await answer.fill("Keep this draft");
  await answer.press("End");
  await answer.press("!");
  await tabs.getByRole("tab", { name: "Timing", exact: true }).click();
  await expect(submit).toBeDisabled();
  await question.getByRole("radio", { name: "Now", exact: true }).check();
  await tabs.getByRole("tab", { name: "Notes", exact: true }).click();
  await expect(answer).toHaveValue("Keep this draft!");
  await answer.press("Control+z");
  await expect(answer).toHaveValue("Keep this draft");
  await tabs.getByRole("tab", { name: "Layout", exact: true }).focus();
  await page.keyboard.press("End");
  await expect(
    tabs.getByRole("tab", { name: "Timing", exact: true }),
  ).toBeFocused();
  await expect(
    question.getByRole("radio", { name: "Now", exact: true }),
  ).toBeChecked();
  await page.keyboard.press("Home");
  await expect(
    question.getByRole("radio", { name: "Compact", exact: true }),
  ).toBeChecked();
  await page.screenshot({ path: "test-results/question-tabs.png" });
  await submit.click();
  expect(JSON.parse(await page.getByRole("status").innerText())).toEqual({
    layout: { selected: ["Compact"], other: "" },
    notes: { selected: [], other: "Keep this draft" },
    timing: { selected: ["Now"], other: "" },
  });
});

test("async choice and free-text steps use positional keys and option previews render Markdown and code", async ({
  page,
}) => {
  await page.goto(
    "/iframe.html?id=conversation-questioncard--async-steps&viewMode=story",
  );
  let question = page.getByRole("region", { name: "Question", exact: true });
  await question.getByRole("radio", { name: "Dark", exact: true }).check();
  await question.getByRole("tab", { name: "Question 1", exact: true }).focus();
  await page.keyboard.press("ArrowRight");
  await expect(
    question.getByRole("tab", { name: "Question 2", exact: true }),
  ).toBeFocused();
  await question
    .getByRole("textbox", {
      name: "Answer: What else should we change?",
      exact: true,
    })
    .fill("More contrast");
  await question
    .getByRole("button", { name: "Submit answers", exact: true })
    .click();
  expect(JSON.parse(await page.getByRole("status").innerText())).toEqual({
    "0": { selected: ["Dark"], other: "" },
    "1": { selected: [], other: "More contrast" },
  });
  await page.goto(
    "/iframe.html?id=conversation-questioncard--option-previews&viewMode=story",
  );
  question = page.getByRole("region", { name: "Question", exact: true });
  await expect(
    question.getByRole("heading", { name: "Compact editor", exact: true }),
  ).toBeVisible();
  await expect(
    question.locator(".question-option-preview pre").first(),
  ).toContainText("max-width: 600px");
  const link = question.getByRole("link", { name: "Documentation" });
  await expect(link).toHaveAttribute("target", "_blank");
  await expect(link).toHaveAttribute("rel", "noopener noreferrer");
  await question.getByRole("radio", { name: /Compact/ }).check();
  await page.screenshot({ path: "test-results/question-option-previews.png" });
  await question
    .getByRole("button", { name: "Submit answers", exact: true })
    .click();
  expect(JSON.parse(await page.getByRole("status").innerText())).toEqual({
    "Which implementation should we use?": { selected: ["Compact"], other: "" },
  });
});
