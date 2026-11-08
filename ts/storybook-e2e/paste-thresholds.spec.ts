import { expect, test } from "@playwright/test";

test("explicit expanded paste keeps text literal when automatic secret chips are enabled", async ({
  page,
}) => {
  await page.context().grantPermissions(["clipboard-read", "clipboard-write"]);
  await page.goto(
    "/iframe.html?id=conversation-composer--automatic-secret-paste&viewMode=story",
  );
  const input = page.getByRole("textbox", { name: "Message", exact: true });
  const text = `ghp_${"aB12".repeat(9)}`;
  await page.evaluate((value) => navigator.clipboard.writeText(value), text);
  await input.press("Control+Shift+v");
  await expect(input).toHaveValue(text);
  await expect(page.locator(".paste-chip")).toHaveCount(0);
  await expect(page.getByLabel("Secret writes")).toHaveText("0");
});

test("Ctrl+Shift+V inserts medium and large multiline text directly at the selection", async ({
  page,
}) => {
  await page.context().grantPermissions(["clipboard-read", "clipboard-write"]);
  await page.goto(
    "/iframe.html?id=composer-upload-chips--interactive&viewMode=story",
  );
  const input = page.getByRole("textbox", { name: "Message", exact: true });
  for (const body of [
    "a".repeat(33),
    "first\nsecond\nthird\nfourth\nfifth",
    "한글😀\t".repeat(100) + "\nlast line",
  ]) {
    await input.fill("before SELECT after");
    await input.evaluate((element: HTMLTextAreaElement) =>
      element.setSelectionRange(7, 13),
    );
    await page.evaluate((text) => navigator.clipboard.writeText(text), body);
    await input.press("Control+Shift+v");
    await expect(input).toHaveValue(`before ${body} after`);
    await expect(page.locator(".paste-chip")).toHaveCount(0);
    expect(
      await input.evaluate(
        (element: HTMLTextAreaElement) => element.selectionStart,
      ),
    ).toBe(7 + body.length);
    await expect(
      page.getByRole("button", { name: "Send", exact: true }),
    ).toBeEnabled();
  }
});

test("expanded paste can replace a chip, undo and redo, and does not change the next ordinary paste", async ({
  page,
}) => {
  await page.context().grantPermissions(["clipboard-read", "clipboard-write"]);
  await page.goto(
    "/iframe.html?id=composer-upload-chips--interactive&viewMode=story",
  );
  const input = page.getByRole("textbox", { name: "Message", exact: true });
  const chip = page.locator(".paste-chip");
  const original = "original chip content\n".repeat(3);
  await page.evaluate((text) => navigator.clipboard.writeText(text), original);
  await input.press("Control+v");
  await expect(chip).toHaveAttribute("data-send-as", "text");
  const expanded = "expanded content\n".repeat(25);
  await page.evaluate((text) => navigator.clipboard.writeText(text), expanded);
  await input.press("Control+a");
  await input.press("Control+Shift+v");
  await expect(input).toHaveValue(expanded);
  await expect(chip).toHaveCount(0);
  await input.press("Control+z");
  await expect(chip).toHaveCount(1);
  await input.press("Control+Shift+z");
  await expect(input).toHaveValue(expanded);
  await expect(chip).toHaveCount(0);
  await input.press("Control+a");
  await input.press("Control+v");
  await expect(chip).toHaveAttribute("data-send-as", "file");
  await expect(chip).toHaveAttribute("data-upload-state", "ready");
});

test("a canceled shortcut cannot change a later context-menu paste", async ({
  page,
}) => {
  await page.goto(
    "/iframe.html?id=composer-upload-chips--interactive&viewMode=story",
  );
  const input = page.getByRole("textbox", { name: "Message", exact: true });
  for (const reset of ["blur", "pointer", "key"]) {
    await input.fill("");
    await input.dispatchEvent("keydown", {
      key: "V",
      code: "KeyV",
      ctrlKey: true,
      shiftKey: true,
    });
    if (reset === "blur") {
      await input.evaluate((element: HTMLTextAreaElement) => element.blur());
      await input.focus();
    } else if (reset === "pointer") await input.click();
    else await input.press("ArrowRight");
    await input.evaluate((element) => {
      const data = new DataTransfer();
      data.setData("text/plain", "normal context-menu paste\n".repeat(3));
      element.dispatchEvent(
        new ClipboardEvent("paste", {
          clipboardData: data,
          bubbles: true,
          cancelable: true,
        }),
      );
    });
    await expect(page.locator(".paste-chip")).toHaveAttribute(
      "data-send-as",
      "text",
    );
  }
});

test("expanded paste retains the existing 1 MiB text limit", async ({
  page,
}) => {
  await page.context().grantPermissions(["clipboard-read", "clipboard-write"]);
  await page.goto(
    "/iframe.html?id=composer-upload-chips--interactive&viewMode=story",
  );
  const input = page.getByRole("textbox", { name: "Message", exact: true });
  await input.fill("keep this");
  await page.evaluate(() =>
    navigator.clipboard.writeText("a".repeat(1024 * 1024 + 1)),
  );
  await input.press("Control+Shift+v");
  await expect(input).toHaveValue("keep this");
  await expect(
    page
      .getByRole("status")
      .filter({ hasText: "Pastes are limited to 1 MiB." }),
  ).toBeVisible();
  await expect(page.locator(".paste-chip")).toHaveCount(0);
});

test("native paste uses inline, Text and File defaults at the character boundaries", async ({
  page,
}) => {
  await page.context().grantPermissions(["clipboard-read", "clipboard-write"]);
  await page.goto(
    "/iframe.html?id=composer-upload-chips--interactive&viewMode=story",
  );
  const input = page.getByRole("textbox", { name: "Message", exact: true });
  const chip = page.locator(".paste-chip");
  for (const [body, mode] of [
    ["a".repeat(32), "inline"],
    ["one\ntwo\nthree\nfour", "inline"],
    ["😀".repeat(32), "inline"],
    ["a".repeat(33), "text"],
    ["한".repeat(256), "text"],
    ["😀".repeat(257), "file"],
  ] as const) {
    await input.fill("");
    await page.evaluate((text) => navigator.clipboard.writeText(text), body);
    await input.press("Control+v");
    if (mode === "inline") {
      await expect(input).toHaveValue(body);
      await expect(chip).toHaveCount(0);
    } else {
      await expect(chip).toHaveAttribute("data-send-as", mode);
      if (mode === "file")
        await expect(chip).toHaveAttribute("data-upload-state", "ready");
      else {
        await expect(chip).not.toHaveAttribute("data-upload-state");
        await expect(
          page.getByRole("button", { name: "Send", exact: true }),
        ).toBeEnabled();
      }
      await expect(
        chip.getByRole("radio", {
          name: mode === "text" ? "Text" : "File",
          exact: true,
        }),
      ).toBeChecked();
    }
  }
});

test("a medium paste uploads only after selecting File", async ({ page }) => {
  await page.context().grantPermissions(["clipboard-read", "clipboard-write"]);
  await page.goto(
    "/iframe.html?id=composer-upload-chips--interactive&viewMode=story",
  );
  const input = page.getByRole("textbox", { name: "Message", exact: true });
  const body = "text first, optional attachment later";
  await page.evaluate((text) => navigator.clipboard.writeText(text), body);
  await input.press("Control+v");
  const chip = page.locator(".paste-chip");
  await expect(chip).toHaveAttribute("data-send-as", "text");
  await expect(chip).not.toHaveAttribute("data-upload-state");
  await chip.getByRole("radio", { name: "File", exact: true }).check();
  await expect(chip).toHaveAttribute("data-send-as", "file");
  await expect(chip).toHaveAttribute("data-upload-state", "ready");
  await chip.getByRole("radio", { name: "Text", exact: true }).check();
  await expect(chip).toHaveAttribute("data-send-as", "text");
  await expect(
    page.getByRole("button", { name: "Send", exact: true }),
  ).toBeEnabled();
});
