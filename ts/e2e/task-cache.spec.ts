import { Buffer } from "node:buffer";
import { expect, test, type Page } from "@playwright/test";
import { create, toJson } from "@bufbuild/protobuf";
import { TaskSchema, TaskStatus } from "../gen/cxz/task_pb";
import { browserProject } from "../test-support/projects";

test.use({
  viewport: { width: 1440, height: 1000 },
  isMobile: false,
  hasTouch: false,
});

async function cachedRows(page: Page) {
  return page.evaluate(async () => {
    const names = (await indexedDB.databases()).flatMap((db) =>
      db.name?.startsWith("payday/cxz.tasks:") ? [db.name] : [],
    );
    let count = 0;
    for (const name of names) {
      count += await new Promise<number>((resolve, reject) => {
        const request = indexedDB.open(name!);
        request.onerror = () => reject(request.error);
        request.onsuccess = () => {
          const db = request.result;
          const rows = db.transaction("rows").objectStore("rows").getAllKeys();
          rows.onsuccess = () => {
            db.close();
            resolve(
              rows.result.filter((key) => String(key).startsWith("cxz.Task/"))
                .length,
            );
          };
          rows.onerror = () => {
            db.close();
            reject(rows.error);
          };
        };
      });
    }
    return count;
  });
}

test("Tasks survive reload while Watch is pending, reconcile remote changes and are removed on sign-out", async ({
  page,
}) => {
  await page.request.post("/auth/login", {
    headers: { Origin: "https://127.0.0.1:18081" },
    data: { token: "a".repeat(32) },
  });
  const project = await browserProject(page.request);
  const value = create(TaskSchema, {
    id: Buffer.from("0000000000008000800b000000000001", "hex"),
    alias: "cached-work",
    name: "Task saved before reload",
    priority: 2,
    status: TaskStatus.TODO,
    project: { id: Buffer.from(project.id, "base64") },
    dateUpdated: { seconds: 1793358000n },
  });
  let hold = false,
    release!: () => void;
  const pending = new Promise<void>((resolve) => {
    release = resolve;
  });
  await page.route("**/cxz.TaskService/Watch", async (route) => {
    if (hold) await pending;
    const body = Buffer.from(
      JSON.stringify({
        items: [
          {
            id: Buffer.from(value.id).toString("base64"),
            value: toJson(TaskSchema, value),
          },
        ],
      }),
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
  try {
    await page.goto("/c/_/task?project=demo");
    await expect(
      page.getByRole("button", {
        name: "Open task: Task saved before reload",
        exact: true,
      }),
    ).toBeVisible();
    await expect.poll(() => cachedRows(page)).toBe(1);
    hold = true;
    value.name = "Task updated while the browser was away";
    value.dateUpdated = {
      $typeName: "google.protobuf.Timestamp",
      seconds: 1793358001n,
      nanos: 0,
    };
    await page.reload();
    await expect(
      page.getByRole("button", {
        name: "Open task: Task saved before reload",
        exact: true,
      }),
    ).toBeVisible();
    await expect(
      page.getByRole("button", {
        name: "Open task: Task updated while the browser was away",
        exact: true,
      }),
    ).toHaveCount(0);
    release();
    await expect(
      page.getByRole("button", {
        name: "Open task: Task updated while the browser was away",
        exact: true,
      }),
    ).toBeVisible();
    await expect(
      page.getByRole("button", {
        name: "Open task: Task saved before reload",
        exact: true,
      }),
    ).toHaveCount(0);
    await page.getByRole("button", { name: "Profile", exact: true }).click();
    await page.getByRole("menuitem", { name: "Sign out", exact: true }).click();
    await expect(page.getByLabel("Web access token")).toBeVisible();
    await expect.poll(() => cachedRows(page)).toBe(0);
  } finally {
    release();
  }
});

test("Tasks remain live when IndexedDB is unavailable", async ({ page }) => {
  const errors: string[] = [];
  page.on("pageerror", (error) => errors.push(error.message));
  await page.addInitScript(() => {
    Object.defineProperty(window, "indexedDB", {
      value: {
        open() {
          throw new DOMException("Storage is blocked", "SecurityError");
        },
      },
    });
  });
  await page.request.post("/auth/login", {
    headers: { Origin: "https://127.0.0.1:18081" },
    data: { token: "a".repeat(32) },
  });
  await page.route("**/cxz.TaskService/Watch", async (route) => {
    const value = create(TaskSchema, {
      id: Buffer.from("0000000000008000800b000000000002", "hex"),
      alias: "live-only",
      name: "Live task without browser storage",
      priority: 2,
      status: TaskStatus.TODO,
    });
    const body = Buffer.from(
      JSON.stringify({
        items: [
          {
            id: Buffer.from(value.id).toString("base64"),
            value: toJson(TaskSchema, value),
          },
        ],
      }),
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
  await page.goto("/c/_/task?project=demo");
  await expect(
    page.getByRole("button", {
      name: "Open task: Live task without browser storage",
      exact: true,
    }),
  ).toBeVisible();
  expect(errors).toEqual([]);
});
