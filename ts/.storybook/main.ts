import type { StorybookConfig } from "@storybook/react-vite";

const config: StorybookConfig = {
  stories: [
    "../src/**/*.stories.@(ts|tsx)",
    "../packages/ui/src/components/action-menu/*.stories.tsx",
    "../packages/ui/src/components/suggestion-grid/*.stories.tsx",
    "../packages/ui/src/components/split-layout/*.stories.tsx",
    "../packages/ui/src/components/resize-handle/*.stories.tsx",
    "../packages/ui/src/components/audio-waveform/*.stories.tsx",
    "../packages/ui/src/terminal/*.stories.tsx",
  ],
  addons: ["@storybook/addon-docs"],
  framework: "@storybook/react-vite",
  core: { disableTelemetry: true },
};

export default config;
