import { create } from "@bufbuild/protobuf";
import { renderToStaticMarkup } from "react-dom/server";
import { expect, test, vi } from "vitest";
import {
  MemoryReadFormat,
  MemoryReadReplySchema,
} from "#gen/cxz/memory_svc_pb";
import { MemoryPreview, type MemoryComparison } from "./memory-preview";

vi.mock("#src/shared/content/markdown.tsx", () => ({ Markdown: () => null }));
vi.mock("#src/shared/theme/theme.tsx", () => ({ useTheme: () => "dark" }));
vi.mock("#src/shared/settings/settings.ts", () => ({
  useEditorSettings: () => ({}),
}));

const render = (data?: MemoryComparison["data"], revision = 2n) =>
  renderToStaticMarkup(
    <MemoryPreview
      content="The full Markdown body"
      mode="changes"
      changeMode={() => {}}
      comparison={{
        revision,
        data,
        loading: false,
        error: "",
        retry: () => {},
      }}
    />,
  );

test("an inconsistent patch is reported instead of displaying misleading changes", () => {
  const html = render(
    create(MemoryReadReplySchema, {
      format: MemoryReadFormat.DIFF,
      diff: "--- r1\n+++ r2\n@@ -1,2 +1,2 @@\n-<img src=x onerror=alert(1)>\n--- body text\n+updated\n+++ body text",
    }),
  );
  expect(html).toContain(
    "The comparison does not match the selected document.",
  );
  expect(html).not.toContain("data-diff-line");
  expect(html).not.toContain("<img");
});

test("a full-text fallback is not presented as a diff", () => {
  const html = render(
    create(MemoryReadReplySchema, {
      format: MemoryReadFormat.FULL,
      fallbackReason: "base revision is not retained",
      content: "This is a complete document, not a patch",
    }),
  );
  expect(html).toContain("The previous revision is no longer retained.");
  expect(html).not.toContain("This is a complete document, not a patch");
  expect(html).not.toContain("data-diff-line");
});

test("first revisions explain why there is no comparison", () => {
  expect(render(undefined, 1n)).toContain("This is the first revision.");
});

test("an empty diff reports no changes", () => {
  const html = render(
    create(MemoryReadReplySchema, { format: MemoryReadFormat.DIFF }),
  );
  expect(html).toContain("No changes in this revision.");
  expect(html).not.toContain("data-diff-line");
});
