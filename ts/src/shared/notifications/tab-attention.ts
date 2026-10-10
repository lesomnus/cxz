export function tabAttention() {
  const original = document.title;
  const icon = document.createElement("link");
  icon.rel = "icon";
  icon.dataset.sessionAttention = "true";
  document.head.append(icon);
  return {
    update(count: number) {
      document.title = count ? `(${count}) ${original}` : original;
      icon.href = `data:image/svg+xml,${encodeURIComponent(`<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 32 32"><rect width="32" height="32" rx="7" fill="#181818"/><path d="m7 11 5 5-5 5m9 0h9" fill="none" stroke="#eee" stroke-width="3"/>${count ? '<circle cx="26" cy="6" r="5" fill="#fff" stroke="#181818" stroke-width="2"/>' : ""}</svg>`)}`;
    },
    dispose() {
      document.title = original;
      icon.remove();
    },
  };
}
