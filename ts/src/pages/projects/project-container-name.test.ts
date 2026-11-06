import { expect, it } from "vitest";
import { containerNamePrefixLength } from "./project-container-name";

it.each([
  [
    "cxz-012345abcdef-0123456789abcdef01234567-dev-1",
    "cxz-012345abcdef-0123456",
  ],
  [
    "cxz-012345abcdef-019a1234-abcd-8000-8007-0123456789ab-app-2",
    "cxz-012345abcdef-019a123",
  ],
  [
    "cxz-012345abcdef-fedcba987654321001234567_worker_1",
    "cxz-012345abcdef-fedcba9",
  ],
  ["custom-workspace-container", "custom-"],
  ["short", "short"],
  ["", ""],
])(
  "previews %s through the storage ID prefix without changing the source name",
  (name, preview) => {
    expect(name.slice(0, containerNamePrefixLength(name))).toBe(preview);
  },
);
