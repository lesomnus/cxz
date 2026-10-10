import { execFileSync } from "node:child_process";
import { expect, test } from "vitest";
import { archiveDirectory, droppedFiles, pickedFiles } from "./composer-files";
import {
  attachmentsReady,
  createFileChip,
  expandPastes,
  pasteRanges,
} from "./composer-pastes";

test("directory tar preserves nested, Unicode, long paths and empty directories", async () => {
  const name = "한글 folder";
  const path = `${name}/${"nested/".repeat(25)}report.txt`;
  const source = "한글 😀\r\nexact\0bytes";
  const archive = archiveDirectory(name, [
    { path: name },
    { path: name + "/empty" },
    { path, file: new File([source], "report.txt") },
  ]);
  const input = Buffer.from(await archive.file.arrayBuffer());
  const listing = execFileSync("tar", ["-tf", "-"], {
    input,
    timeout: 5000,
    encoding: "utf8",
  });
  expect(listing.split("\n")).toEqual([name + "/", name + "/empty/", path, ""]);
  expect(
    execFileSync("tar", ["-xOf", "-", path], {
      input,
      timeout: 5000,
      encoding: "utf8",
    }),
  ).toBe(source);
  expect(() => archiveDirectory(name, [{ path: name + "/../escape" }])).toThrow(
    "Invalid upload filename",
  );
});

test("directory drop drains all readEntries pages and captures the entry synchronously", async () => {
  let captured = 0,
    page = 0;
  const files = Array.from({ length: 120 }, (_, i) => ({
    isFile: true,
    isDirectory: false,
    name: `${i}.txt`,
    file: (resolve: (file: File) => void) =>
      resolve(new File([String(i)], `${i}.txt`)),
  }));
  const entry = {
    isDirectory: true,
    isFile: false,
    name: "project",
    createReader: () => ({
      readEntries: (resolve: (entries: unknown[]) => void) =>
        resolve([files.slice(0, 100), files.slice(100), []][page++]),
    }),
  };
  const data = {
    files: [],
    items: [
      {
        kind: "file",
        webkitGetAsEntry: () => {
          captured++;
          return entry;
        },
        getAsFile: () => null,
      },
    ],
  } as unknown as DataTransfer;
  const picking = droppedFiles(data);
  expect(captured).toBe(1);
  const [picked] = await picking;
  const input = Buffer.from(await picked.file.arrayBuffer());
  const listing = execFileSync("tar", ["-tf", "-"], {
    input,
    timeout: 5000,
    encoding: "utf8",
  });
  expect(listing).toContain("project/119.txt");
  expect(listing.trim().split("\n")).toHaveLength(121);
  expect(page).toBe(3);
});

test("folder picker groups roots and file chips transmit paths only after acknowledgement", () => {
  const file = new File(["body"], "name [copy].txt");
  Object.defineProperty(file, "webkitRelativePath", {
    value: "folder/nested/name [copy].txt",
  });
  const [picked] = pickedFiles([file]);
  expect(picked.directory).toBe(true);
  expect(picked.name).toBe("folder");
  const chip = createFileChip(file, file.name);
  const pastes = new Map([[chip.token, chip]]);
  const draft = "Before " + chip.token + " after";
  expect(pasteRanges(draft, pastes)).toHaveLength(1);
  expect(attachmentsReady(draft, pastes)).toBe(false);
  chip.attachment!.state = "error";
  expect(attachmentsReady(draft, pastes)).toBe(false);
  chip.attachment!.state = "ready";
  chip.attachment!.path = "/cxz/assets/session/unique/name [copy].txt";
  expect(attachmentsReady(draft, pastes)).toBe(true);
  expect(expandPastes(draft, pastes)).toBe(
    "Before [Attached file: /cxz/assets/session/unique/name [copy].txt — read this file for the full content] after",
  );
  expect(attachmentsReady("Before after", pastes)).toBe(true);
});
