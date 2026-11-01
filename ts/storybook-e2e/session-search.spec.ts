import { expect, test } from "@playwright/test";

test("session-search story opens, matches and jumps while keeping the overlay until Escape", async ({
  page,
}) => {
  await page.goto(
    "/iframe.html?id=conversation-sessionsearch--default&viewMode=story",
  );
  await page.getByRole("button", { name: "Search conversation" }).click();
  const search = page.getByRole("search", {
    name: "Search current conversation",
  });
  const input = search.getByRole("searchbox");
  const counter = search.getByRole("status");
  await expect(counter).toHaveAccessibleName("Result 0 of 0");
  await input.fill("network");
  await expect(counter).toHaveAccessibleName("Result 1 of 2");
  await expect(counter).toHaveText("0001/0002");
  await expect(search.locator("mark")).toHaveText(["network", "network"]);
  await input.press("ArrowDown");
  await expect(counter).toHaveAccessibleName("Result 2 of 2");
  await input.press("ArrowUp");
  await expect(counter).toHaveAccessibleName("Result 1 of 2");
  await expect(page.locator(".scroll-tick[data-search]")).toHaveCount(2);
  await search
    .getByRole("button", { name: /Investigate network latency/ })
    .click();
  await expect(page.getByText("Jumped to message 1")).toBeVisible();
  await expect(counter).toHaveAccessibleName("Result 2 of 2");
  await expect(search).toBeVisible();
  await page.mouse.click(5, 5);
  await expect(search).toBeVisible();
  await page.keyboard.press("Escape");
  await expect(search).toHaveCount(0);
  await expect(page.locator(".scroll-tick[data-search]")).toHaveCount(0);
});

test("Enter highlights each occurrence newest first, including inline Markdown, and wraps", async ({
  page,
}) => {
  await page.goto(
    "/iframe.html?id=conversation-sessionsearch--default&viewMode=story",
  );
  await page.getByRole("button", { name: "Search conversation" }).click();
  const search = page.getByRole("search", {
    name: "Search current conversation",
  });
  const input = search.getByRole("searchbox");
  await input.fill("network");
  const counter = search.getByRole("status");
  await expect(counter).toHaveAccessibleName("Result 1 of 2");
  await expect
    .poll(() =>
      page.evaluate(() => CSS.highlights.get("conversation-search")?.size),
    )
    .toBe(3);
  const active = () =>
    page.evaluate(() => {
      const range = [
        ...(CSS.highlights.get("conversation-search-current") ?? []),
      ][0] as Range | undefined;
      return range
        ? {
            seq: range.startContainer.parentElement?.closest<HTMLElement>(
              "[data-row]",
            )?.dataset.row,
            text: range.startContainer.parentElement?.textContent,
          }
        : undefined;
    });
  await input.press("Enter");
  await expect
    .poll(active)
    .toEqual({ seq: "2", text: "Second network match in this reply." });
  const bottomMatchScroll = await page
    .locator(".transcript")
    .evaluate((el) => el.scrollTop);
  await expect(input).toBeFocused();
  await input.press("Enter");
  await expect.poll(active).toEqual({ seq: "2", text: "network" });
  await expect(counter).toHaveAccessibleName("Result 1 of 2");
  expect(
    await page.locator(".transcript").evaluate((el) => el.scrollTop),
  ).toBeLessThan(bottomMatchScroll);
  await expect(input).toBeFocused();
  await input.press("Enter");
  await expect.poll(async () => (await active())?.seq).toBe("1");
  await expect(counter).toHaveAccessibleName("Result 2 of 2");
  await input.press("Enter");
  await expect
    .poll(active)
    .toEqual({ seq: "2", text: "Second network match in this reply." });
  await expect(counter).toHaveAccessibleName("Result 1 of 2");
  await input.fill("network transfer");
  await expect(counter).toHaveAccessibleName("Result 1 of 1");
  await expect(search.locator("mark")).toHaveText(["network transfer"]);
  await input.press("Enter");
  await expect
    .poll(() =>
      page.evaluate(() =>
        (
          [...(CSS.highlights.get("conversation-search-current") ?? [])][0] as
            | Range
            | undefined
        )?.toString(),
      ),
    )
    .toBe("network transfer");
  await input.fill("[network]");
  await expect(counter).toHaveAccessibleName("Result 0 of 0");
  await expect(counter).toHaveText("0000/0000");
  await expect(search.getByText("No matches", { exact: true })).toBeVisible();
  await expect(search.locator("mark")).toHaveCount(0);
  await expect
    .poll(() =>
      page.evaluate(() => CSS.highlights.get("conversation-search")?.size),
    )
    .toBe(0);
  await input.press("Escape");
  await expect
    .poll(() =>
      page.evaluate(() => CSS.highlights.has("conversation-search-current")),
    )
    .toBe(false);
});

test("local search updates when loaded responses arrive without changing the query", async ({
  page,
}) => {
  await page.goto(
    "/iframe.html?id=conversation-sessionsearch--default&viewMode=story",
  );
  await page.getByRole("button", { name: "Search conversation" }).click();
  const search = page.getByRole("search", {
      name: "Search current conversation",
    }),
    input = search.getByRole("searchbox");
  await input.fill("network");
  await expect(search.getByRole("status")).toHaveAccessibleName(
    "Result 1 of 2",
  );
  await page.getByRole("button", { name: "Append response" }).click();
  await expect(input).toHaveValue("network");
  await expect(search.getByRole("status")).toHaveAccessibleName(
    "Result 1 of 3",
  );
  await expect(search.locator("mark")).toHaveText([
    "network",
    "network",
    "network",
  ]);
  await search.locator('[data-search-index="0"]').click();
  await expect(page.getByText("Jumped to message 4")).toBeVisible();
  await expect(input).toBeFocused();
  await input.press("Escape");
  await expect(search).toHaveCount(0);
});

test("preview highlights preserve casing and match literal punctuation and repeated text", async ({
  page,
}) => {
  await page.goto(
    "/iframe.html?id=conversation-sessionsearch--with-results&viewMode=story",
  );
  const search = page.getByRole("search", {
    name: "Search current conversation",
  });
  const input = search.getByRole("searchbox");
  await expect(input).toHaveValue("network");
  const originalHeight = (
    await search.locator("[data-search-index]").first().boundingBox()
  )?.height;
  await input.fill("NETWORK");
  await expect(search.locator("mark")).toHaveText(["network", "network"]);
  await input.fill(".");
  const punctuation = search.locator("mark");
  await expect(punctuation.first()).toBeVisible();
  expect(
    (await punctuation.allTextContents()).every((text) => text === "."),
  ).toBe(true);
  await input.fill("Transfer diagnostics.");
  const repeated = search.locator("mark");
  await expect(repeated.first()).toBeVisible();
  expect(await repeated.count()).toBeGreaterThan(1);
  expect(
    (await repeated.allTextContents()).every((text) =>
      "Transfer diagnostics.".startsWith(text),
    ),
  ).toBe(true);
  const row = search.locator("[data-search-index]");
  await expect(row).toHaveCount(1);
  expect((await row.boundingBox())?.height).toBe(originalHeight);
  await row.click();
  await expect(page.getByText("Jumped to message 2")).toBeVisible();
  await input.press("Escape");
  await expect(search).toHaveCount(0);
});

test("search mode toggles share matches with previews, transcript highlighting and navigation", async ({
  page,
}) => {
  await page.goto(
    "/iframe.html?id=conversation-sessionsearch--with-results&viewMode=story",
  );
  const search = page.getByRole("search", {
    name: "Search current conversation",
  });
  const input = search.getByRole("searchbox");
  const counter = search.getByRole("status");
  const matchCase = search.getByRole("button", {
    name: "Match case",
    exact: true,
  });
  const wholeWord = search.getByRole("button", {
    name: "Match whole word",
    exact: true,
  });
  const regex = search.getByRole("button", {
    name: "Use regular expression",
    exact: true,
  });
  const highlightCount = () =>
    page.evaluate(() => CSS.highlights.get("conversation-search")?.size ?? 0);
  await expect(input).toHaveValue("network");
  for (const button of [matchCase, wholeWord, regex])
    await expect(button).toHaveAttribute("aria-pressed", "false");
  await input.fill("NETWORK");
  await expect(counter).toHaveAccessibleName("Result 1 of 2");
  await matchCase.click();
  await expect(matchCase).toHaveAttribute("aria-pressed", "true");
  await expect(input).toBeFocused();
  await expect(input).toHaveValue("NETWORK");
  await expect(counter).toHaveAccessibleName("Result 0 of 0");
  await expect(search.locator("mark")).toHaveCount(0);
  await expect.poll(highlightCount).toBe(0);
  await input.fill("net");
  await expect(counter).toHaveAccessibleName("Result 1 of 2");
  await wholeWord.click();
  await expect(wholeWord).toHaveAttribute("aria-pressed", "true");
  await expect(counter).toHaveAccessibleName("Result 0 of 0");
  await expect.poll(highlightCount).toBe(0);
  await regex.click();
  await expect(regex).toHaveAttribute("aria-pressed", "true");
  await input.fill("net(?:work)");
  await expect(counter).toHaveAccessibleName("Result 1 of 2");
  await expect(search.locator("mark")).toHaveText(["network", "network"]);
  await expect.poll(highlightCount).toBe(3);
  await input.press("Enter");
  await expect(page.getByText("Jumped to message 2")).toBeVisible();
  await expect
    .poll(() =>
      page.evaluate(() =>
        [
          ...(CSS.highlights.get("conversation-search-current") ?? []),
        ][0]?.toString(),
      ),
    )
    .toBe("network");
  await input.fill("(?:NETWORK|terminal)");
  await expect(counter).toHaveAccessibleName("Result 1 of 1");
  await expect(search.locator("mark")).toHaveText(["terminal"]);
  await expect.poll(highlightCount).toBe(1);
  await input.press("Enter");
  await expect(page.getByText("Jumped to message 1")).toBeVisible();
  await matchCase.click();
  await expect(counter).toHaveAccessibleName("Result 1 of 2");
  await expect(search.locator("mark")).toHaveText([
    "network",
    "network",
    "terminal",
  ]);
  await expect.poll(highlightCount).toBe(4);
  await input.fill("[");
  await expect(input).toHaveAttribute("aria-invalid", "true");
  await expect(search.getByRole("alert")).toHaveText(
    "Invalid regular expression",
  );
  await expect(counter).toHaveAccessibleName("Result 0 of 0");
  await expect.poll(highlightCount).toBe(0);
  await input.fill("network");
  await expect(input).not.toHaveAttribute("aria-invalid", "true");
  await expect(search.getByRole("alert")).toHaveCount(0);
  await expect(counter).toHaveAccessibleName("Result 1 of 2");
  await input.fill("(?=network)");
  await expect(counter).toHaveAccessibleName("Result 0 of 0");
  await expect(search.getByText("No matches", { exact: true })).toBeVisible();
  await expect.poll(highlightCount).toBe(0);
  await input.press("Escape");
  await expect(search).toHaveCount(0);
});

test("the populated options story handles Unicode whole words and keeps narrow controls within the search bar", async ({
  page,
}) => {
  await page.goto(
    "/iframe.html?id=conversation-sessionsearch--with-search-options&viewMode=story",
  );
  const search = page.getByRole("search", {
    name: "Search current conversation",
  });
  const input = search.getByRole("searchbox");
  await expect(input).toHaveValue("net(?:work)?");
  for (const name of [
    "Match case",
    "Match whole word",
    "Use regular expression",
  ])
    await expect(
      search.getByRole("button", { name, exact: true }),
    ).toHaveAttribute("aria-pressed", "true");
  await expect(search.locator("mark")).toHaveText([
    "network",
    "net",
    "network",
  ]);
  expect(
    await search.locator("[data-search-index]").evaluateAll((rows) =>
      rows.every((row) => {
        const snippet = row
          .querySelector("mark")!
          .parentElement!.getBoundingClientRect();
        return [...row.querySelectorAll("mark")].some((mark) =>
          [...mark.getClientRects()].some(
            (rect) => rect.top < snippet.bottom && rect.bottom > snippet.top,
          ),
        );
      }),
    ),
  ).toBe(true);
  await input.fill("한글");
  await expect(search.getByRole("status")).toHaveAccessibleName(
    "Result 1 of 1",
  );
  await expect(search.locator("mark")).toHaveText(["한글", "한글", "한글"]);
  await expect
    .poll(() =>
      page.evaluate(() => CSS.highlights.get("conversation-search")?.size),
    )
    .toBe(3);
  await page.setViewportSize({ width: 360, height: 800 });
  const geometry = await search.evaluate((el) => {
    const form = el.querySelector("form")!;
    const input = el.querySelector("input")!.getBoundingClientRect();
    const close = el
      .querySelector<HTMLElement>('button[aria-label="Close search"]')!
      .getBoundingClientRect();
    const mode = el
      .querySelector<HTMLElement>('[role="group"]')!
      .getBoundingClientRect();
    return {
      width: form.clientWidth,
      scrollWidth: form.scrollWidth,
      inputWidth: input.width,
      modeRight: mode.right,
      closeLeft: close.left,
    };
  });
  expect(geometry.scrollWidth).toBe(geometry.width);
  expect(geometry.inputWidth).toBeGreaterThan(40);
  expect(geometry.modeRight).toBeLessThan(geometry.closeLeft);
  await input.press("Escape");
  await expect(search).toHaveCount(0);
});
