import { expect, test, type Locator } from "@playwright/test";

const flowchart =
  "flowchart LR\n  accTitle: Message lifecycle\n  Input[사용자 입력] --> Manager[Manager]\n  Manager --> Agent[Agent]\n  Agent --> Response[응답]\n";
const story = (name: string, theme = "dark") =>
  `/iframe.html?id=content-markdown--${name}&viewMode=story&globals=theme:${theme}`;
const loaded = async (image: Locator) => {
  await expect(image).toBeVisible();
  await expect
    .poll(() =>
      image.evaluate(
        (el: HTMLImageElement) => el.complete && el.naturalWidth > 0,
      ),
    )
    .toBe(true);
};
const svg = (image: Locator) =>
  image.evaluate((el: HTMLImageElement) =>
    fetch(el.src).then((response) => response.text()),
  );
const labels = async (image: Locator) =>
  image.page().evaluate(
    (markup) => {
      const doc = new DOMParser().parseFromString(markup, "image/svg+xml");
      return [...doc.querySelectorAll("text")]
        .map((node) => node.textContent)
        .join("\n");
    },
    await svg(image),
  );

test("Mermaid renders flowcharts and sequences, and source/copy/language controls preserve the definition", async ({
  page,
  context,
}) => {
  await context.grantPermissions(["clipboard-read", "clipboard-write"]);
  const errors: string[] = [];
  page.on("pageerror", (error) => errors.push(error.message));
  await page.goto(story("mermaid"));
  const image = page.getByRole("img", { name: "Message lifecycle" });
  await loaded(image);
  await loaded(page.getByRole("img", { name: "Mermaid diagram", exact: true }));
  const markup = await svg(image);
  expect(await labels(image)).toContain("사용자 입력");
  expect(markup).toContain("Manager");
  const block = page.locator(".markdown pre").first().locator("..");
  await expect(block.locator("pre")).toBeHidden();
  await block.hover();
  await block.getByRole("button", { name: "Copy", exact: true }).click();
  expect(await page.evaluate(() => navigator.clipboard.readText())).toBe(
    flowchart,
  );
  await block.getByRole("button", { name: "View source", exact: true }).click();
  await expect(block.locator("pre")).toBeVisible();
  await expect(block.locator("code")).toHaveText(flowchart);
  await block
    .getByRole("button", { name: "View diagram", exact: true })
    .click();
  await loaded(image);
  const language = block.getByRole("combobox", {
    name: "Language",
    exact: true,
  });
  await language.click();
  await page.getByRole("option", { name: "plaintext", exact: true }).click();
  await expect(block.locator("pre")).toBeVisible();
  await expect(block.getByRole("img")).toHaveCount(0);
  await language.click();
  await page.getByRole("option", { name: "mermaid", exact: true }).click();
  await loaded(image);
  await expect(
    page.locator('.markdown [data-language="javascript"]'),
  ).toContainText("console.log");
  expect(errors).toEqual([]);
});

test("partial and invalid definitions retain source and recover; unrelated streamed text does not redraw diagrams", async ({
  page,
}) => {
  const errors: string[] = [];
  page.on("pageerror", (error) => errors.push(error.message));
  await page.goto(story("mermaid-streaming"));
  await expect(page.getByRole("status")).toHaveText(
    "Unable to render this Mermaid diagram. Showing source.",
  );
  await expect(page.locator(".markdown pre")).toBeVisible();
  await expect(page.locator(".markdown code")).toHaveText(
    "flowchart LR\n  Input[\n",
  );
  await page
    .getByRole("button", { name: "Complete diagram", exact: true })
    .click();
  const image = page.getByRole("img", { name: "Message lifecycle" });
  await loaded(image);
  const url = await image.getAttribute("src");
  const original = await svg(image);
  await page
    .getByRole("button", { name: "Append response", exact: true })
    .click();
  await expect(image).toHaveAttribute("src", url!);
  await expect(
    page.getByText("More response text.", { exact: true }),
  ).toBeVisible();
  await page
    .getByRole("button", { name: "Remove diagram", exact: true })
    .click();
  await expect(page.locator(".markdown img")).toHaveCount(0);
  expect(
    await page.evaluate(async (url) => {
      try {
        await fetch(url);
        return false;
      } catch {
        return true;
      }
    }, url!),
  ).toBe(true);
  await page
    .getByRole("button", { name: "Restore diagram", exact: true })
    .click();
  await loaded(image);
  expect(await svg(image)).toBe(original);
  await page
    .getByRole("button", { name: "Invalid diagram", exact: true })
    .click();
  await expect(page.getByRole("status")).toHaveText(
    "Unable to render this Mermaid diagram. Showing source.",
  );
  await page
    .getByRole("button", { name: "Complete diagram", exact: true })
    .click();
  await page
    .getByRole("button", { name: "Replace diagram", exact: true })
    .click();
  const latest = page.getByRole("img", {
    name: "Mermaid diagram",
    exact: true,
  });
  await loaded(latest);
  expect(await labels(latest)).toContain("Latest definition");
  await expect(
    page.getByRole("img", { name: "Message lifecycle" }),
  ).toHaveCount(0);
  await expect(page.locator("body > div:has(> svg)")).toHaveCount(0);
  expect(errors).toEqual([]);
});

for (const theme of ["dark", "light"]) {
  test(`Mermaid uses ${theme} theme tokens and fits a narrow conversation`, async ({
    page,
  }) => {
    await page.setViewportSize({ width: 380, height: 740 });
    await page.goto(story("mermaid", theme));
    const image = page.getByRole("img", { name: "Message lifecycle" });
    await loaded(image);
    const markup = await svg(image);
    const foreground = await page.evaluate(() =>
      getComputedStyle(document.documentElement)
        .getPropertyValue("--ui-shade-237")
        .trim(),
    );
    expect(markup.toLowerCase()).toContain(foreground.toLowerCase());
    const bounds = (await image.boundingBox())!;
    expect(bounds.x).toBeGreaterThanOrEqual(0);
    expect(bounds.x + bounds.width).toBeLessThanOrEqual(380);
    expect(
      await page.evaluate(() => document.documentElement.scrollWidth),
    ).toBeLessThanOrEqual(380);
  });
}

test("untrusted Mermaid directives cannot enable HTML, scripts, links or page styles", async ({
  page,
}) => {
  const dialogs: string[] = [];
  page.on("dialog", (dialog) => {
    dialogs.push(dialog.message());
    void dialog.dismiss();
  });
  await page.goto(story("mermaid-untrusted"));
  const image = page.getByRole("img", { name: "Mermaid diagram", exact: true });
  await loaded(image);
  const markup = await svg(image);
  const dangerous = await page.evaluate((markup) => {
    const doc = new DOMParser().parseFromString(markup, "image/svg+xml");
    return [
      ...doc.querySelectorAll(
        'script,foreignObject,image,[onload],[onerror],[onclick],a[href^="javascript:"]',
      ),
    ].map((el) => el.localName);
  }, markup);
  expect(dangerous).toEqual([]);
  expect(markup).not.toContain("body { display: none; }");
  await expect(page.locator("body")).toBeVisible();
  await expect(
    page.locator(
      ".markdown script, .markdown svg[aria-roledescription], .markdown iframe",
    ),
  ).toHaveCount(0);
  expect(dialogs).toEqual([]);
});

test("live theme changes recolor a loaded diagram without flashing its source", async ({
  page,
}) => {
  await page.goto(story("mermaid-streaming"));
  await page
    .getByRole("button", { name: "Complete diagram", exact: true })
    .click();
  const image = page.getByRole("img", { name: "Message lifecycle" });
  await loaded(image);
  const original = await svg(image);
  const pre = page.locator(".markdown pre");
  const shown = pre.evaluate(
    (node) =>
      new Promise<boolean>((resolve) => {
        let visible = false;
        const observer = new MutationObserver(() => {
          if (!(node as HTMLElement).hidden) visible = true;
        });
        observer.observe(node, {
          attributes: true,
          attributeFilter: ["hidden"],
        });
        setTimeout(() => {
          observer.disconnect();
          resolve(visible);
        }, 1500);
      }),
  );
  await page.getByRole("button", { name: "Toggle theme", exact: true }).click();
  await expect(page.locator("html")).toHaveAttribute("data-theme", "light");
  const foreground = await page.evaluate(() =>
    getComputedStyle(document.documentElement)
      .getPropertyValue("--ui-shade-237")
      .trim(),
  );
  await expect
    .poll(() =>
      svg(image)
        .then((markup) => markup.includes(foreground))
        .catch(() => false),
    )
    .toBe(true);
  expect(await shown).toBe(false);
  await page.getByRole("button", { name: "Toggle theme", exact: true }).click();
  await expect(page.locator("html")).toHaveAttribute("data-theme", "dark");
  await expect.poll(() => svg(image).catch(() => "")).toBe(original);
});

test("ordinary Markdown does not load the Mermaid renderer", async ({
  page,
}) => {
  const requests: string[] = [];
  page.on("request", (request) => requests.push(request.url()));
  await page.goto(story("code-blocks"));
  await expect(page.locator(".markdown code")).toHaveCount(3);
  expect(requests.filter((url) => /mermaid-renderer/.test(url))).toEqual([]);
});

test("the actual conversation response renders Mermaid in its message body", async ({
  page,
}) => {
  await page.goto(
    "/iframe.html?id=conversation-eventcards--mermaid-response&viewMode=story",
  );
  const image = page.getByRole("img", {
    name: "Conversation routing",
    exact: true,
  });
  await loaded(image);
  await expect(page.locator(".transcript")).toContainText(
    "The manager forwards the message",
  );
  expect(await labels(image)).toContain("Project runtime");
});
