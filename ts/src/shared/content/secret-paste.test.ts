import { expect, it } from "vitest";
import cases from "../../../../internal/secretpaste/testdata/cases.json?raw";
import { detectSecretPaste } from "./secret-paste";

for (const fixture of JSON.parse(cases) as {
  name: string;
  text: string;
  kind: string;
}[]) {
  it(fixture.name, () => {
    expect(detectSecretPaste(fixture.text) ?? "").toBe(fixture.kind);
  });
}
