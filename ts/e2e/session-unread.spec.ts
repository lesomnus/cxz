import { Buffer } from "node:buffer";
import { expect, test } from "@playwright/test";

test.use({
  viewport: { width: 1280, height: 900 },
  isMobile: false,
  hasTouch: false,
});

test("unread response uses the left indicator and Sessions icon count clears when read", async ({
  page,
}) => {
  await page.request.post("/auth/login", {
    headers: { Origin: "https://127.0.0.1:18081" },
    data: { token: "a".repeat(32) },
  });
  await page.addInitScript(() => {
    sessionStorage.setItem(
      `cxz.attention.v1:${encodeURIComponent(location.origin)}`,
      JSON.stringify({
        session: {
          run: "run",
          last: "3",
          done: "3",
          seen: "0",
          notified: "3",
          pending: "[]",
        },
      }),
    );
  });
  // Other tests may resolve the fixture's question. Seed every catalog path
  // consistently so this case does not depend on test order.
  const session = {
    id: Buffer.from("abcdefghijklmnop").toString("base64"),
    runtimeId: "session",
    alias: "demo-chat",
    name: "Demo conversation",
    agent: "claude",
    listed: true,
    project: {
      id: Buffer.from("1234567890123456").toString("base64"),
      runtimeId: "project",
      alias: "demo",
      name: "Demo project",
      listed: true,
    },
    status: {
      runId: "run",
      state: "idle",
      lastSeq: "3",
      pending: [
        {
          seq: "3",
          runId: "run",
          kind: "approval",
          requestId: "q",
          text: "AskUserQuestion",
          payload: Buffer.from(
            JSON.stringify({
              input: {
                questions: [
                  {
                    question: "Which environment?",
                    options: [{ label: "Development" }],
                  },
                ],
              },
            }),
          ).toString("base64"),
        },
      ],
    },
  };
  await page.route("**/cxz.SessionService/Get", (route) =>
    route.fulfill({ json: session }),
  );
  await page.route("**/cxz.SessionService/List", (route) =>
    route.fulfill({ json: { items: [session] } }),
  );
  await page.route("**/cxz.SessionService/Watch", async (route) => {
    const body = Buffer.from(
      JSON.stringify({ items: [{ id: session.id, value: session }] }),
    );
    const header = Buffer.alloc(5);
    header.writeUInt32BE(body.length, 1);
    await route.fulfill({
      contentType: "application/connect+json",
      body: Buffer.concat([
        header,
        body,
        Buffer.from([2, 0, 0, 0, 2, 123, 125]),
      ]),
    });
  });
  await page.goto("/sessions");
  const row = page.locator(".tree-session").first();
  const unread = row
    .locator(".session-indicator")
    .getByRole("img", { name: "Unread response", exact: true });
  await expect(unread).toBeVisible();
  await expect(unread.locator("circle")).toHaveCSS("fill", "rgb(217, 255, 48)");
  await expect(row.locator(".session-unread")).toHaveCount(0);
  const tab = page.getByRole("link", { name: "Sessions view", exact: true });
  const count = tab.getByRole("img", {
    name: "1 unread sessions",
    exact: true,
  });
  await expect(count).toBeVisible();
  await expect(count).toHaveText("1");
  await expect(count).toHaveCSS("background-color", "rgb(217, 255, 48)");
  const favicon = await page
    .locator("link[data-session-attention]")
    .getAttribute("href");
  expect(decodeURIComponent(favicon!)).toContain('fill="#d9ff30"');
  await row.click();
  await expect(page.locator(".transcript")).toHaveAttribute(
    "aria-busy",
    "false",
  );
  await expect(unread).toHaveCount(0);
  await expect(count).toHaveCount(0);
  // A pending question retains its own status indicator once unread clears.
  await expect(row.locator(".session-indicator")).toHaveAttribute(
    "aria-label",
    "Awaiting answer",
  );
});
