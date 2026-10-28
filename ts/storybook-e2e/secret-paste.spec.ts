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
  await expect(input).toHaveValue(/^Before \[Secret [0-9a-f]{8}\]more  after$/);
  await expect(send).toBeEnabled();
  await expect(page.locator("body")).not.toContainText(token);
  await chip.click();
  const dialog = page.getByRole("dialog", { name: "Secret", exact: true });
  await expect(dialog).toContainText("GitHub token detected");
  await expect(dialog).toContainText("/cxz/secrets/storybook/value");
  await expect(
    dialog.getByRole("button", { name: "Expand source" }),
  ).toHaveCount(0);
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
  await page.getByRole("button", { name: "Delete", exact: true }).click();
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

test("embedded and incomplete tokens retain native paste handling; JWT header order is flexible", async ({
  page,
}) => {
  for (const text of [
    "ghp_",
    `TOKEN=${token}`,
    `use ${token} here`,
    `\`${token}\``,
  ]) {
    expect(await paste(page, text)).toBe(true);
  }
  await expect(page.getByLabel("Secret writes")).toHaveText("0");
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
    page.getByRole("dialog", { name: "Secret", exact: true }),
  ).toContainText("JWT detected");
});
