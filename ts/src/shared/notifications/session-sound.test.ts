import { afterEach, expect, it, vi } from "vitest";
import { sessionSound } from "./session-sound";

afterEach(() => vi.unstubAllGlobals());
it("uses a distinct error cue, interrupts queued cues, and respects mute", async () => {
  const listeners = new Map<string, () => void>();
  vi.stubGlobal("document", {
    addEventListener: (name: string, fn: () => void) => listeners.set(name, fn),
    removeEventListener: (name: string) => listeners.delete(name),
  });
  const sources: any[] = [];
  let decoded = 0;
  vi.stubGlobal(
    "fetch",
    vi.fn(async () => ({
      ok: true,
      arrayBuffer: async () => new ArrayBuffer(0),
    })),
  );
  vi.stubGlobal(
    "AudioContext",
    class {
      state = "running";
      currentTime = 0;
      destination = {};
      decodeAudioData = async () => ({ duration: 0.5, cue: decoded++ });
      close = async () => {};
      createGain() {
        return {
          gain: { value: 0 },
          connect() {
            return this;
          },
          disconnect() {},
        };
      }
      createBufferSource() {
        const source = {
          buffer: undefined,
          start: vi.fn(),
          stop: vi.fn(),
          connect: (gain: unknown) => gain,
          disconnect: vi.fn(),
          onended: undefined,
        };
        sources.push(source);
        return source;
      }
    },
  );
  let enabled = true;
  const sound = sessionSound(() => enabled);
  listeners.get("pointerdown")!();
  await vi.waitFor(() => expect(decoded).toBe(3));
  sound.play("complete");
  sound.play("question");
  sound.play("error");
  expect(sources).toHaveLength(3);
  expect(sources[0].stop).toHaveBeenCalled();
  expect(sources[1].stop).toHaveBeenCalled();
  expect(sources[2].buffer.cue).not.toBe(sources[0].buffer.cue);
  expect(sources[2].start).toHaveBeenCalledWith(0);
  enabled = false;
  sound.play("error");
  expect(sources).toHaveLength(3);
  sound.dispose();
  expect(listeners.size).toBe(0);
});
