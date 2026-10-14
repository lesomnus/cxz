import { Marked } from "marked";

// Tildes in conversation text are literal, including ranges and doubled tildes.
// Keep other GFM features (tables, autolinks, fenced code) enabled.
export const conversationMarkdown = new Marked({
  tokenizer: {
    del() {
      return undefined;
    },
  },
});
