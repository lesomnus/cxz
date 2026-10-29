import { expect, test, type Page } from "@playwright/test";

const token = `ghp_${"aB12".repeat(9)}`;
async function paste(page: Page, text: string) {
  return page
    .getByRole("textbox", { name: "Message", exact: true })
    .evaluate((node, text) => {
      const clipboardData = new DataTransfer();
      clipboardData.setData("text/plain", text);
      return node.dispatchEvent(
        new ClipboardEvent("paste", {
          clipboardData,
          bubbles: true,
          cancelable: true,
        }),
      );
    }, text);
}

test.beforeEach(async ({ page }) => {
  await page.goto(
    "/iframe.html?id=conversation-composer--automatic-secret-paste&viewMode=story",
  );
});

test("short whole tokens immediately become secret chips and remain editable around preparation", async ({
  page,
}) => {
  const input = page.getByRole("textbox", { name: "Message", exact: true });
  const send = page.getByRole("button", { name: "Send", exact: true });
  await input.fill("Before  after");
  await input.evaluate((node: HTMLTextAreaElement) => {
    node.focus();
    node.setSelectionRange(7, 7);
  });
  expect(await paste(page, token)).toBe(false);
  const chip = page.getByRole("button", {
    name: "Secret file reference",
    exact: true,
  });
  await expect(chip).toHaveAttribute("data-upload-state", "uploading");
  await expect(send).toBeDisabled();
  expect(await input.inputValue()).not.toContain(token);
  await input.pressSequentially("more ");
  await expect(chip).toHaveAttribute("data-upload-state", "ready");
  // The current editor reserves the measured chip width with invisible text.
  // Its source token belongs to the chip, not the textarea's displayed value.
  await expect(input).toHaveValue(/^Before [\s\S]+more  after$/);
  await expect(chip.locator(".paste-chip-label")).toHaveText("Secret");
  expect(await input.inputValue()).not.toContain(token);
  await expect(send).toBeEnabled();
  await expect(page.locator("body")).not.toContainText(token);
  await chip.click();
  const dialog = page.getByRole("dialog", { name: "Secret", exact: true });
  await expect(dialog.getByLabel("Secret value")).toHaveText(token);
  await expect(
    dialog.getByRole("button", { name: "Expand", exact: true }),
  ).toBeEnabled();
  await expect(page.getByLabel("Secret writes")).toHaveText("1");
});

test("a failed secret write blocks Send without leaking the raw value or falling back", async ({
  page,
}) => {
  await page.getByRole("button", { name: "Fail secret writes: off" }).click();
  const input = page.getByRole("textbox", { name: "Message", exact: true });
  await paste(page, token);
  const chip = page.getByRole("button", {
    name: "Secret file reference",
    exact: true,
  });
  await expect(chip).toHaveAttribute("data-upload-state", "error");
  await expect(
    page.getByRole("button", { name: "Send", exact: true }),
  ).toBeDisabled();
  expect(await input.inputValue()).not.toContain(token);
  await chip.click();
  await expect(page.getByRole("alert")).toContainText(
    "Secret preparation failed",
  );
  await page
    .getByRole("button", { name: "Close paste preview", exact: true })
    .click();
  await input.focus();
  await input.press("ControlOrMeta+a");
  await input.press("Backspace");
  await expect(input).toHaveValue("");
  await expect(chip).toHaveCount(0);
  await expect(page.getByLabel("Secret writes")).toHaveText("1");
});

test("deleting a pending secret cancels it and cleans up a late file acknowledgement", async ({
  page,
}) => {
  const input = page.getByRole("textbox", { name: "Message", exact: true });
  await paste(page, token);
  await expect(page.getByLabel("Secret writes")).toHaveText("1");
  await input.fill("");
  await expect(page.getByLabel("Deleted secret files")).toHaveText("1");
  await expect(
    page.getByRole("button", { name: "Secret file reference", exact: true }),
  ).toHaveCount(0);
  await expect(input).toHaveValue("");
});

test("embedded and incomplete tokens retain ordinary paste handling; JWT header order is flexible", async ({
  page,
}) => {
  const input = page.getByRole("textbox", { name: "Message", exact: true });
  expect(await paste(page, "ghp_")).toBe(true);
  for (const text of [`TOKEN=${token}`, `use ${token} here`, `\`${token}\``]) {
    await input.fill("");
    await paste(page, text);
    await expect(page.locator(".paste-chip")).toHaveAttribute(
      "data-send-as",
      "text",
    );
    await expect(
      page.getByRole("button", { name: "Secret file reference", exact: true }),
    ).toHaveCount(0);
  }
  await expect(page.getByLabel("Secret writes")).toHaveText("0");
  await input.fill("");
  const encode = (value: unknown) =>
    Buffer.from(JSON.stringify(value)).toString("base64url");
  const jwt = `${encode({ kid: "fixture", alg: "RS256" })}.${encode({ sub: "system:serviceaccount:default:fixture" })}.c2lnbmF0dXJl`;
  expect(await paste(page, jwt)).toBe(false);
  const chip = page.getByRole("button", {
    name: "Secret file reference",
    exact: true,
  });
  await expect(chip).toHaveAttribute("data-upload-state", "ready");
  await chip.click();
  await expect(
    page
      .getByRole("dialog", { name: "Secret", exact: true })
      .getByLabel("Secret value"),
  ).toHaveText(jwt);
});

test("Secret upload shows measured percentages until acknowledgement, then only its name", async ({
  page,
}) => {
  await page.evaluate(() => {
    const recorded: { label: string; width: number; input: string }[] = [];
    const observer = new MutationObserver(() => {
      const chip = document.querySelector(".paste-chip");
      const label = chip?.querySelector(".paste-chip-label")?.textContent;
      const input = document.querySelector<HTMLTextAreaElement>(
        'textarea[aria-label="Message"]',
      );
      if (chip && label && recorded.at(-1)?.label !== label)
        recorded.push({
          label,
          width: chip.getBoundingClientRect().width,
          input: input?.value ?? "",
        });
    });
    observer.observe(document.body, {
      subtree: true,
      childList: true,
      characterData: true,
    });
    Reflect.set(window, "secretProgressRecorder", { recorded, observer });
  });
  await paste(page, token);
  const chip = page.getByRole("button", {
    name: "Secret file reference",
    exact: true,
  });
  await expect(chip).toHaveAttribute("data-upload-state", "ready");
  await expect(chip.locator(".paste-chip-label")).toHaveText("Secret");
  const recorded = await page.evaluate(() => {
    const { recorded, observer } = Reflect.get(
      window,
      "secretProgressRecorder",
    ) as {
      recorded: { label: string; width: number; input: string }[];
      observer: MutationObserver;
    };
    observer.disconnect();
    return recorded;
  });
  const labels = recorded.map((entry) => entry.label);
  expect(labels).toEqual(
    expect.arrayContaining(["  000%", "  099%", "Secret"]),
  );
  expect(labels.some((label) => /^  0(?:12|25|50|75)%$/.test(label))).toBe(
    true,
  );
  expect(labels).not.toContain("  100%");
  expect(new Set(recorded.map((entry) => entry.width)).size).toBe(1);
  expect(recorded[0].width).toBeGreaterThan(0);
  expect(new Set(recorded.map((entry) => entry.input)).size).toBe(1);
  await expect(chip.locator('[data-mode="integer"]')).toHaveCount(0);
});

test("the value-only Secret body reveals during selection and Expand requires confirmation", async ({
  page,
}) => {
  const input = page.getByRole("textbox", { name: "Message", exact: true });
  await paste(page, token);
  const chip = page.getByRole("button", {
    name: "Secret file reference",
    exact: true,
  });
  await expect(chip).toHaveAttribute("data-upload-state", "ready");
  await chip.click();
  const dialog = page.getByRole("dialog", { name: "Secret", exact: true });
  await expect(dialog).toHaveCSS("transform", "matrix(1, 0, 0, 1, 0, 0)");
  const value = dialog.getByLabel("Secret value");
  await expect(value).toHaveText(token);
  expect(await dialog.locator(".card-body").textContent()).toBe(token);
  await expect(value).toHaveCSS("filter", "blur(5px)");
  const bounds = (await value.boundingBox())!;
  await page.mouse.move(bounds.x + 2, bounds.y + bounds.height / 2);
  await page.mouse.down();
  await page.mouse.move(
    bounds.x + Math.min(150, bounds.width - 2),
    bounds.y + bounds.height / 2,
    { steps: 8 },
  );
  await expect(value).toHaveCSS("filter", "blur(0px)");
  await page.mouse.up();
  await page.evaluate(() => window.getSelection()?.removeAllRanges());
  await expect(value).toHaveCSS("filter", "blur(5px)");
  const expand = dialog.getByRole("button", { name: "Expand", exact: true });
  const close = dialog.getByRole("button", {
    name: "Close paste preview",
    exact: true,
  });
  expect((await expand.boundingBox())!.x).toBeLessThan(
    (await close.boundingBox())!.x,
  );
  await expand.click();
  await expect(dialog.locator(".confirm-button")).toHaveAttribute(
    "data-confirming",
    "true",
  );
  expect(await input.inputValue()).not.toContain(token);
  await expect(chip).toHaveCount(1);
  await dialog
    .getByRole("button", { name: "Expand secret as plain text", exact: true })
    .click();
  await expect(dialog).toHaveCount(0);
  await expect(chip).toHaveCount(0);
  await expect(input).toHaveValue(token);
});

test("confirmed expansion cannot overwrite an edited draft", async ({
  page,
}) => {
  const input = page.getByRole("textbox", { name: "Message", exact: true });
  await paste(page, token);
  const chip = page.getByRole("button", {
    name: "Secret file reference",
    exact: true,
  });
  await expect(chip).toHaveAttribute("data-upload-state", "ready");
  await chip.click();
  const dialog = page.getByRole("dialog", { name: "Secret", exact: true });
  await expect(dialog.getByLabel("Secret value")).toHaveText(token);
  await input.fill("new draft");
  await dialog.getByRole("button", { name: "Expand", exact: true }).click();
  await dialog
    .getByRole("button", { name: "Expand secret as plain text", exact: true })
    .click();
  await expect(input).toHaveValue("new draft");
  await expect(dialog.getByRole("status")).toContainText("input has changed");
});
