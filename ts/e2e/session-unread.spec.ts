import { Buffer } from "node:buffer";
import { createHash } from "node:crypto";
import { fromBinary } from "@bufbuild/protobuf";
import { SessionMarkReadRequestSchema } from "../gen/cxz/session_svc_pb";
import { expect, test } from "@playwright/test";
import { browserProject } from "../test-support/projects";

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
  const id = (domain: number, name: string) => {
    const value = createHash("sha256")
      .update(`cxz.resource:${name}`)
      .digest()
      .subarray(0, 16);
    value[6] = (value[6]! & 15) | 0x80;
    value[8] = (value[8]! & 63) | 0x80;
    value[9] = domain;
    return value.toString("base64");
  };
  const readerId = id(16, "operator");
  let throughSeq = "0";
  const state = () => ({ readerId, generation: "fixture", throughSeq });
  await page.route("**/cxz.SessionService/ReadState", (route) =>
    route.fulfill({ json: state() }),
  );
  await page.route("**/cxz.SessionService/MarkRead", async (route) => {
    const req = route.request();
    const body = /application\/proto/.test(req.headers()["content-type"] ?? "")
      ? fromBinary(SessionMarkReadRequestSchema, req.postDataBuffer()!)
      : req.postDataJSON();
    throughSeq = String(body.throughSeq);
    await route.fulfill({ json: state() });
  });
  await page.route("**/cxz.NotificationService/Read", (route) =>
    route.fulfill({
      json: {
        readerId,
        items: [
          {
            notification: {
              id: id(14, "completed"),
              kind: "session.completed",
              sourceId: id(8, "session"),
              payload: {
                "@type": "type.googleapis.com/cxz.SessionNotification",
                runtimeId: "session",
                generation: "fixture",
                seq: "3",
              },
            },
            ...(BigInt(throughSeq) >= 3n
              ? { receipt: { id: id(15, "read"), readerId } }
              : {}),
          },
        ],
      },
    }),
  );
  await page.route("**/cxz.NotificationService/Listen", async (route) => {
    const body = Buffer.from(JSON.stringify({ ready: { readerId } }));
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
  // Other tests may resolve the fixture's question. Seed every catalog path
  // consistently so this case does not depend on test order.
  const session = {
    id: id(8, "session"),
    runtimeId: "session",
    alias: "demo-chat",
    name: "Demo conversation",
    agent: "claude",
    listed: true,
    project: await browserProject(page.request),
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
  await page.goto("/c/_/session");
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
