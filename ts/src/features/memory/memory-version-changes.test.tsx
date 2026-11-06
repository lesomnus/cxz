import { create, fromJson, toJson } from "@bufbuild/protobuf";
import { renderToStaticMarkup } from "react-dom/server";
import { expect, test } from "vitest";
import { MemoryVersionSchema } from "#gen/cxz/memory_svc_pb";
import { MemoryVersionChanges } from "./memory-version-changes";

test("the first version is creation rather than a large change", () => {
  const markup = renderToStaticMarkup(
    <MemoryVersionChanges
      id="version"
      version={create(MemoryVersionSchema, { revision: 1n })}
    />,
  );
  expect(markup).toContain("Created");
  expect(markup).not.toContain("+0");
});
test("explicit zero survives the API JSON and displays as a known line count", () => {
  const version = fromJson(
    MemoryVersionSchema,
    toJson(
      MemoryVersionSchema,
      create(MemoryVersionSchema, {
        revision: 2n,
        linesAdded: 0,
        linesRemoved: 12,
      }),
    ),
  );
  const markup = renderToStaticMarkup(
    <MemoryVersionChanges id="version" version={version} />,
  );
  expect(markup).toContain("+0 −12");
  expect(markup).toContain("0 lines added, 12 lines removed");
  expect(markup).toContain("Compared with r1");
});
test("missing counts from an older server do not look like zero changes", () => {
  const markup = renderToStaticMarkup(
    <MemoryVersionChanges
      id="version"
      version={create(MemoryVersionSchema, { revision: 4n })}
    />,
  );
  expect(markup).toContain("Line changes unavailable");
  expect(markup).toContain("—");
  expect(markup).not.toContain("+0");
  expect(markup).not.toContain("Created");
});
