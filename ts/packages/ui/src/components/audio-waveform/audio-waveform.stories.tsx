import { useCallback, useRef } from "react";
import type { Meta, StoryObj } from "@storybook/react-vite";
import { AudioWaveform } from "@lesomnus/cxz-ui";

const meta = {
  title: "Components/AudioWaveform",
  component: AudioWaveform,
  tags: ["autodocs"],
  args: { active: true, readLevel: () => 0 },
  parameters: {
    docs: {
      description: {
        component:
          "A monochrome, smoothed audio-level visualization. The host supplies readLevel (0–1); capture, permissions and recognition are independent. The upward variant folds the wave above its bottom baseline for input borders. This story simulates volume without opening a microphone. Motion pauses in hidden tabs; reduced motion freezes the wave phase while retaining volume feedback.",
      },
    },
  },
  render: function Preview(args) {
    const volume = useRef(0.6);
    const readLevel = useCallback(() => volume.current, []);
    return (
      <div
        style={{
          width: "min(600px, 85vw)",
          background: "var(--ui-shade-27)",
          padding: 16,
        }}
      >
        <label>
          Volume{" "}
          <input
            type="range"
            min="0"
            max="1"
            step="0.01"
            defaultValue={volume.current}
            onChange={(event) => {
              volume.current = Number(event.target.value);
            }}
          />
        </label>
        <AudioWaveform {...args} readLevel={readLevel} />
      </div>
    );
  },
} satisfies Meta<typeof AudioWaveform>;
export default meta;
export const Listening: StoryObj<typeof meta> = {};
export const Stopped: StoryObj<typeof meta> = { args: { active: false } };

export const Upward: StoryObj<typeof meta> = { args: { variant: "upward" } };
