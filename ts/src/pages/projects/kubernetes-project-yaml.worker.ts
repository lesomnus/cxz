import "monaco-yaml/yaml.worker.js";

// The legacy worker wrapper installs Monaco's message handler on its first
// message. Pass through the configuration so Monaco 0.55 can initialize it.
const bootstrap = self.onmessage!;
self.onmessage = (event) => {
  bootstrap.call(self, event);
  self.onmessage?.(event);
};
