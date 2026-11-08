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
test("explicit zero survives the API JSON and displays as a known byte count", () => {
  const version = fromJson(
    MemoryVersionSchema,
    toJson(
      MemoryVersionSchema,
      create(MemoryVersionSchema, {
        revision: 2n,
        bytesAdded: 0n,
        bytesRemoved: 12n,
      }),
    ),
  );
  const markup = renderToStaticMarkup(
    <MemoryVersionChanges id="version" version={version} />,
  );
  expect(markup).toContain('aria-label="0 bytes added"');
  expect(markup).toContain('aria-label="12 bytes removed"');
  expect(markup).toContain('data-muted="true"');
  expect(markup).toContain("0 bytes added, 12 bytes removed");
  expect(markup).toContain("Compared with r1");
  expect(markup).toContain("UTF-8 bytes");
  expect(markup).toMatch(/>B<\/span>/);
});
test("large byte counts use CompactNumber units", () => {
  const markup = renderToStaticMarkup(
    <MemoryVersionChanges
      version={create(MemoryVersionSchema, {
        revision: 3n,
        bytesAdded: 1250n,
        bytesRemoved: 1000000n,
      })}
    />,
  );
  expect(markup).toContain('aria-label="1250 bytes added"');
  expect(markup).toContain('aria-label="1000000 bytes removed"');
  expect(markup).toMatch(/>K<\/span>/);
  expect(markup).toMatch(/>M<\/span>/);
});
test("missing counts from an older server do not look like zero changes", () => {
  const markup = renderToStaticMarkup(
    <MemoryVersionChanges
      id="version"
      version={create(MemoryVersionSchema, { revision: 4n })}
    />,
  );
  expect(markup).toContain("Byte changes unavailable");
  expect(markup).toContain("—");
  expect(markup).not.toContain("+0");
  expect(markup).not.toContain("Created");
});

test("older line statistics are never relabeled as bytes", () => {
  const markup = renderToStaticMarkup(
    <MemoryVersionChanges
      version={create(MemoryVersionSchema, {
        revision: 2n,
        linesAdded: 1,
        linesRemoved: 1,
      })}
    />,
  );
  expect(markup).toContain("Byte changes unavailable");
  expect(markup).not.toContain("1 bytes added");
});
