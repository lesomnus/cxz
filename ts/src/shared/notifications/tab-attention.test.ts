import { afterEach, expect, it, vi } from "vitest";
import { tabAttention } from "./tab-attention";
import { SESSION_ERROR_COLOR } from "./error-color";
import { SESSION_ATTENTION_COLOR } from "./attention-color";

afterEach(() => vi.unstubAllGlobals());
it("prioritizes the error favicon and restores completion color after errors are read", () => {
  const icon = { rel: "", dataset: {}, href: "", remove: vi.fn() };
  const doc = {
    title: "cxz",
    createElement: () => icon,
    head: { append: vi.fn() },
  };
  vi.stubGlobal("document", doc);
  const badge = tabAttention();
  badge.update(3, true);
  expect(decodeURIComponent(icon.href)).toContain(SESSION_ERROR_COLOR);
  expect(doc.title).toBe("(3) cxz");
  badge.update(2);
  expect(decodeURIComponent(icon.href)).toContain(SESSION_ATTENTION_COLOR);
  badge.update(0);
  expect(decodeURIComponent(icon.href)).not.toContain("<circle");
  badge.dispose();
  expect(doc.title).toBe("cxz");
  expect(icon.remove).toHaveBeenCalled();
});
