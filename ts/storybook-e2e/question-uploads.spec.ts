import { expect, test, type Locator } from "@playwright/test";

const token = `ghp_${"aB12".repeat(9)}`;

async function paste(input: Locator, text: string) {
  await input.evaluate((node, text) => {
    const clipboardData = new DataTransfer();
    clipboardData.setData("text/plain", text);
    node.dispatchEvent(
      new ClipboardEvent("paste", {
        clipboardData,
        bubbles: true,
        cancelable: true,
      }),
    );
  }, text);
}

async function attach(input: Locator, type: "drop" | "paste", name: string) {
  await input.evaluate(
    (node, { type, name }) => {
      const data = new DataTransfer();
      data.items.add(new File(["attachment content"], name));
      node.dispatchEvent(
        type === "drop"
          ? new DragEvent(type, {
              dataTransfer: data,
              bubbles: true,
              cancelable: true,
            })
          : new ClipboardEvent(type, {
              clipboardData: data,
              bubbles: true,
              cancelable: true,
            }),
      );
    },
    { type, name },
  );
}

test.beforeEach(async ({ page }) => {
  await page.goto(
    "/iframe.html?id=conversation-questioncard--attachments&viewMode=story",
  );
});

test("large answer pastes upload and submit a path while the conversation draft stays independent", async ({
  page,
}) => {
  const question = page.getByRole("region", { name: "Question", exact: true });
  const input = question.getByRole("textbox");
  const composer = page.getByRole("textbox", { name: "Message", exact: true });
  const submit = question.getByRole("button", { name: "Submit", exact: true });
  await composer.fill("Keep the conversation draft");
  await input.fill("Before  after");
  await input.evaluate((node: HTMLTextAreaElement) => {
    node.focus();
    node.setSelectionRange(7, 7);
  });
  const text = "Long answer\n".repeat(100);
  await paste(input, text);
  const chip = question.locator(".paste-chip");
  await expect(chip).toHaveAttribute("data-send-as", "file");
  await expect(chip).toHaveAttribute("data-upload-state", "uploading");
  await expect(submit).toBeDisabled();
  await input.press("Control+Enter");
  await expect(submit).not.toBeFocused();
  await expect(question).toBeVisible();
  await expect(chip).toHaveAttribute("data-upload-state", "ready");
  await expect(submit).toBeEnabled();
  await input.press("Control+Enter");
  await expect(submit).toBeFocused();
  await expect(question).toBeVisible();
  await submit.click();
  const result = page.getByRole("status").filter({ hasText: /"selected"/ });
  const answer = JSON.parse((await result.textContent())!)["0"];
  expect(answer.selected).toEqual([]);
  expect(answer.other).toMatch(
    /^Before \[Attached file: \/cxz\/assets\/storybook\/upload\/paste-[0-9a-f]{8}\.txt — read this file for the full content\] after$/,
  );
  expect(answer.other).not.toContain(text);
  await expect(page.getByLabel("File uploads", { exact: true })).toHaveText(
    "1",
  );
  await expect(composer).toHaveValue("Keep the conversation draft");
});

test("medium Text chips retain their source and can switch to File like the composer", async ({
  page,
}) => {
  const question = page.getByRole("region", { name: "Question", exact: true });
  const input = question.getByRole("textbox");
  const text = "This answer has more than thirty-two characters.";
  await paste(input, text);
  const chip = question.locator(".paste-chip");
  await expect(chip).toHaveAttribute("data-send-as", "text");
  await expect(page.getByLabel("File uploads", { exact: true })).toHaveText(
    "0",
  );
  const submit = question.getByRole("button", { name: "Submit", exact: true });
  await expect(submit).toBeEnabled();
  await chip.getByRole("radio", { name: "File", exact: true }).check();
  await expect(submit).toBeDisabled();
  await expect(chip).toHaveAttribute("data-upload-state", "ready");
  await expect(submit).toBeEnabled();
  await chip.getByRole("radio", { name: "Text", exact: true }).check();
  await submit.click();
  const result = page.getByRole("status").filter({ hasText: /"selected"/ });
  expect(JSON.parse((await result.textContent())!)["0"].other).toBe(text);
});

for (const type of ["drop", "paste"] as const) {
  test(`answer file ${type} uses the shared upload and preview`, async ({
    page,
  }) => {
    const question = page.getByRole("region", {
      name: "Question",
      exact: true,
    });
    const input = question.getByRole("textbox");
    await attach(input, type, "report.txt");
    const chip = question.locator(".paste-chip");
    await expect(chip).toHaveAttribute("data-upload-state", "uploading");
    await expect(
      question.getByRole("button", { name: "Submit", exact: true }),
    ).toBeDisabled();
    await expect(chip).toHaveAttribute("data-upload-state", "ready");
    await chip.click();
    const preview = page.getByRole("dialog", {
      name: "Attachment",
      exact: true,
    });
    await expect(preview).toContainText(
      "/cxz/assets/storybook/upload/report.txt",
    );
    await preview
      .getByRole("button", { name: "Close paste preview", exact: true })
      .click();
    await question.getByRole("button", { name: "Submit", exact: true }).click();
    await expect(
      page.getByRole("status").filter({ hasText: /"selected"/ }),
    ).toContainText(
      "[Attached file: /cxz/assets/storybook/upload/report.txt — read this file for the full content]",
    );
    await expect(page.locator(".composer .paste-chip")).toHaveCount(0);
  });
}

test("whole-token answers use Secret files, concealed previews and path-only replies", async ({
  page,
}) => {
  const question = page.getByRole("region", { name: "Question", exact: true });
  const input = question.getByRole("textbox");
  await paste(input, token);
  const chip = question.getByRole("button", {
    name: "Secret file reference",
    exact: true,
  });
  await expect(chip).toHaveAttribute("data-upload-state", "uploading");
  await expect(
    question.getByRole("button", { name: "Submit", exact: true }),
  ).toBeDisabled();
  expect(await input.inputValue()).not.toContain(token);
  await expect(chip).toHaveAttribute("data-upload-state", "ready");
  await expect(page.locator("body")).not.toContainText(token);
  await chip.click();
  const preview = page.getByRole("dialog", { name: "Secret", exact: true });
  const body = preview.getByLabel("Secret value");
  await expect(body).toHaveText(token);
  await expect(body).toHaveCSS("filter", "blur(5px)");
  await preview
    .getByRole("button", { name: "Close paste preview", exact: true })
    .click();
  await question.getByRole("button", { name: "Submit", exact: true }).click();
  const result = page.getByRole("status").filter({ hasText: /"selected"/ });
  expect(JSON.parse((await result.textContent())!)["0"].other).toMatch(
    /^\(secret placed at \/cxz\/secrets\/[^/]+\/value\)$/,
  );
  await expect(page.getByLabel("Secret writes", { exact: true })).toHaveText(
    "1",
  );
  await expect(page.getByLabel("File uploads", { exact: true })).toHaveText(
    "0",
  );
  await expect(page.locator("body")).not.toContainText(token);
});

test("failed file uploads block Submit and retry from the same preview", async ({
  page,
}) => {
  await page.goto(
    "/iframe.html?id=conversation-questioncard--failed-upload&viewMode=story",
  );
  const question = page.getByRole("region", { name: "Question", exact: true });
  await attach(question.getByRole("textbox"), "drop", "retry.txt");
  const chip = question.locator(".paste-chip");
  await expect(chip).toHaveAttribute("data-upload-state", "error");
  const submit = question.getByRole("button", { name: "Submit", exact: true });
  await expect(submit).toBeDisabled();
  await chip.click();
  await page.getByRole("button", { name: "Retry upload", exact: true }).click();
  await expect(
    page.getByRole("dialog", { name: "Attachment", exact: true }),
  ).toContainText("/cxz/assets/storybook/upload/retry.txt");
  await page
    .getByRole("button", { name: "Close paste preview", exact: true })
    .click();
  await expect(chip).toHaveAttribute("data-upload-state", "ready");
  await expect(submit).toBeEnabled();
  await expect(page.getByLabel("File uploads", { exact: true })).toHaveText(
    "2",
  );
});

test("failed secrets never fall back to text uploads and removing the chip releases Submit", async ({
  page,
}) => {
  await page.goto(
    "/iframe.html?id=conversation-questioncard--failed-secret&viewMode=story",
  );
  const question = page.getByRole("region", { name: "Question", exact: true });
  const input = question.getByRole("textbox");
  await input.fill("Keep ");
  await input.press("End");
  await paste(input, token);
  const chip = question.locator(".paste-chip");
  await expect(chip).toHaveAttribute("data-upload-state", "error");
  await expect(
    question.getByRole("button", { name: "Submit", exact: true }),
  ).toBeDisabled();
  await expect(page.getByLabel("File uploads", { exact: true })).toHaveText(
    "0",
  );
  await expect(page.locator("body")).not.toContainText(token);
  await input.fill("Keep");
  await expect(chip).toHaveCount(0);
  await expect(
    question.getByRole("button", { name: "Submit", exact: true }),
  ).toBeEnabled();
});

test("uploads keep running across question tabs and gate the final submission", async ({
  page,
}) => {
  await page.goto(
    "/iframe.html?id=conversation-questioncard--attachments&viewMode=story&args=example:steps;uploadDelay:5000",
  );
  const question = page.getByRole("region", { name: "Question", exact: true });
  await question.getByRole("radio", { name: "Compact", exact: true }).check();
  await question.getByRole("tab", { name: /Q2/ }).click();
  const input = question.getByRole("textbox");
  await paste(input, "Additional answer context ".repeat(20));
  await question.getByRole("tab", { name: /Q3/ }).click();
  await question.getByRole("radio", { name: "Now", exact: true }).check();
  const submit = question.getByRole("button", { name: "Submit", exact: true });
  await expect(submit).toBeDisabled();
  await expect(submit).toBeEnabled();
  await submit.click();
  const result = page.getByRole("status").filter({ hasText: /"selected"/ });
  const answers = JSON.parse((await result.textContent())!);
  expect(answers.layout.selected).toEqual(["Compact"]);
  expect(answers.timing.selected).toEqual(["Now"]);
  expect(answers.notes.other).toContain(
    "[Attached file: /cxz/assets/storybook/upload/",
  );
});
