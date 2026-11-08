import { expect, test, type Locator } from "@playwright/test";

const story = (name = "persistent-answers", args = "") =>
  `/iframe.html?id=conversation-questioncard--${name}&viewMode=story${args ? `&args=${args}` : ""}`;
const card = (page: import("@playwright/test").Page) =>
  page.getByRole("region", { name: "Question", exact: true });
const tab = (question: Locator, index: number) =>
  question.getByRole("tab", { name: `Q${index}`, exact: true });

test("answers survive refresh and page navigation together with the selected question tab", async ({
  page,
}) => {
  await page.goto(story());
  const question = card(page);
  await question.getByRole("radio", { name: "Compact", exact: true }).check();
  await tab(question, 2).click();
  const notes = question.getByRole("textbox", {
    name: "Answer: Describe any additional changes.",
    exact: true,
  });
  await notes.fill("한글 답변\nKeep the selected layout.");
  await tab(question, 3).click();
  await question.getByRole("radio", { name: "Later", exact: true }).check();
  await page.reload();
  await expect(tab(question, 3)).toHaveAttribute("aria-selected", "true");
  await expect(
    question.getByRole("radio", { name: "Later", exact: true }),
  ).toBeChecked();
  await tab(question, 2).click();
  await expect(notes).toHaveValue("한글 답변\nKeep the selected layout.");
  await page.goto(
    "/iframe.html?id=conversation-composer--empty&viewMode=story",
  );
  await page.goto(story());
  await expect(tab(question, 2)).toHaveAttribute("aria-selected", "true");
  await expect(notes).toHaveValue("한글 답변\nKeep the selected layout.");
  await tab(question, 1).click();
  await expect(
    question.getByRole("radio", { name: "Compact", exact: true }),
  ).toBeChecked();
});

test("draft scopes isolate different sessions and Question request IDs", async ({
  page,
}) => {
  await page.goto(story());
  const question = card(page);
  const compact = question.getByRole("radio", { name: "Compact", exact: true });
  await compact.check();
  await page.goto(story("persistent-answers", "sessionId:other-session"));
  await expect(compact).not.toBeChecked();
  await question.getByRole("radio", { name: "Spacious", exact: true }).check();
  await page.goto(story());
  await expect(compact).toBeChecked();
  await page.goto(story("persistent-answers", "requestId:another-question"));
  await expect(compact).not.toBeChecked();
  await page.goto(story());
  await expect(compact).toBeChecked();
});

test("confirmed replies and cancellation clear only the saved Question draft", async ({
  page,
}) => {
  await page.goto(story());
  const question = card(page);
  const compact = question.getByRole("radio", { name: "Compact", exact: true });
  await compact.check();
  await tab(question, 2).click();
  await question.getByRole("textbox").fill("Ready to implement");
  await tab(question, 3).click();
  await question.getByRole("radio", { name: "Now", exact: true }).check();
  await question.getByRole("button", { name: "Submit", exact: true }).click();
  await expect(question).toHaveCount(0);
  await page.reload();
  await expect(tab(question, 1)).toHaveAttribute("aria-selected", "true");
  await expect(compact).not.toBeChecked();
  await compact.check();
  const cancel = question.locator(".question-footer button").first();
  await cancel.click();
  await expect(question).toBeVisible();
  await cancel.click();
  await expect(question).toHaveCount(0);
  await page.reload();
  await expect(compact).not.toBeChecked();
});

test("failed replies preserve editable answers across refresh", async ({
  page,
}) => {
  await page.goto(story("failed-submission"));
  const question = card(page),
    input = question.getByRole("textbox");
  await input.fill("Keep this answer until the server accepts it.");
  await question.getByRole("button", { name: "Submit", exact: true }).click();
  await expect(page.getByRole("alert")).toContainText(
    "Your answers are still available",
  );
  await expect(input).toBeEditable();
  await page.reload();
  await expect(input).toHaveValue(
    "Keep this answer until the server accepts it.",
  );
});

test("attachment drafts restore uploaded chips without uploading them again", async ({
  page,
}) => {
  await page.goto(story("persistent-attachments"));
  const question = card(page),
    input = question.getByRole("textbox");
  await input.evaluate((node) => {
    const clipboardData = new DataTransfer();
    clipboardData.setData("text/plain", "Attached answer line\n".repeat(100));
    node.dispatchEvent(
      new ClipboardEvent("paste", {
        clipboardData,
        bubbles: true,
        cancelable: true,
      }),
    );
  });
  const chip = question.locator(".paste-chip");
  await expect(chip).toHaveAttribute("data-upload-state", "ready");
  await expect(page.getByLabel("File uploads", { exact: true })).toHaveText(
    "1",
  );
  await page.reload();
  await expect(chip).toHaveAttribute("data-upload-state", "ready");
  await expect(page.getByLabel("File uploads", { exact: true })).toHaveText(
    "0",
  );
  await question.getByRole("button", { name: "Submit", exact: true }).click();
  await expect(
    page.getByRole("status").filter({ hasText: /"selected"/ }),
  ).toContainText("/cxz/assets/storybook/upload/");
});
