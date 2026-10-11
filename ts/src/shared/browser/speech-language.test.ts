import { describe, expect, it } from "vitest";
import { speechLanguage } from "./speech-language";

describe("speech recognition language", () => {
  it("supplies a regional locale for bare browser languages", () => {
    expect(speechLanguage("ko")).toBe("ko-KR");
    expect(speechLanguage("en")).toBe("en-US");
    expect(speechLanguage("ja")).toBe("ja-JP");
  });
  it("preserves explicit regions and compatible browser preferences", () => {
    expect(speechLanguage("en-GB", ["en-US"])).toBe("en-GB");
    expect(speechLanguage("en", ["fr-FR", "en-GB"])).toBe("en-GB");
    expect(speechLanguage("ko", ["en-US", "ko-KR"])).toBe("ko-KR");
    expect(speechLanguage("zh-Hant", ["zh-CN", "zh-TW"])).toBe("zh-TW");
    expect(speechLanguage("zh-Hant")).toBe("zh-Hant-TW");
  });
  it("ignores invalid alternatives and removes unrelated locale extensions", () => {
    expect(speechLanguage("ko", ["invalid_locale", "en-US"])).toBe("ko-KR");
    expect(speechLanguage("ko-KR-u-nu-latn")).toBe("ko-KR");
    expect(speechLanguage("invalid_locale")).toBe("en-US");
    expect(speechLanguage(undefined)).toBe("en-US");
    expect(speechLanguage(undefined, ["ko", "ko-KR"])).toBe("ko-KR");
  });
});
