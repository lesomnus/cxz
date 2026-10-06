import { describe, expect, it } from "vitest";
import { messageDate, relativeMessageTime } from "./message-time";

describe("input timestamps", () => {
  it("shows minute and hour boundaries and stops relative labels at one week", () => {
    const now = new Date(2026, 9, 6, 12);
    const ago = (ms: number) =>
      relativeMessageTime(new Date(now.getTime() - ms), now);
    expect(ago(0)).toBe("1분 이내");
    expect(ago(59_999)).toBe("1분 이내");
    expect(ago(60_000)).toBe("1분 전");
    expect(ago(5 * 60_000)).toBe("5분 전");
    expect(ago(3_600_000)).toBe("1시간 전");
    expect(ago(86_400_000)).toBe("어제");
    expect(ago(7 * 86_400_000)).toBe("");
    expect(ago(-1000)).toBe("");
  });

  it("uses calendar days and only calls an earlier calendar week 지난주", () => {
    const now = new Date(2026, 9, 11, 12); // Sunday
    expect(relativeMessageTime(new Date(2026, 9, 8, 12), now)).toBe("3일 전");
    expect(relativeMessageTime(new Date(2026, 9, 7, 12), now)).toBe("4일 전");
    const monday = new Date(2026, 9, 12, 12);
    expect(relativeMessageTime(new Date(2026, 9, 7, 12), monday)).toBe(
      "지난주",
    );
    expect(relativeMessageTime(new Date(2026, 9, 6, 12), monday)).toBe(
      "지난주",
    );
    expect(relativeMessageTime(new Date(2026, 9, 5, 12), monday)).toBe("");
  });

  it("does not invent a date for missing or invalid journal timestamps", () => {
    expect(messageDate(0n)).toBeUndefined();
    expect(messageDate(-1n)).toBeUndefined();
    expect(messageDate(2n ** 63n - 1n)).toBeUndefined();
    expect(messageDate(1_700_000_000_000n)?.getTime()).toBe(1_700_000_000_000);
  });
});
