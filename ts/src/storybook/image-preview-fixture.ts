// Transparent, self-contained image for preview interaction examples.
const svg = `<svg xmlns="http://www.w3.org/2000/svg" width="800" height="600" viewBox="0 0 800 600">
  <rect x="110" y="90" width="580" height="420" rx="64" fill="#293640" fill-opacity=".75"/>
  <circle cx="300" cy="280" r="150" fill="#b9d75e" fill-opacity=".85"/>
  <circle cx="485" cy="320" r="135" fill="#b29ad6" fill-opacity=".75"/>
  <path d="M200 425 390 165l210 260" fill="none" stroke="#eee" stroke-width="10" stroke-linecap="round"/>
</svg>`;

export const previewImage = () =>
  new File([svg], "image.svg", { type: "image/svg+xml" });
export const readPreviewImage = async (_path: string, signal: AbortSignal) => {
  signal.throwIfAborted();
  const file = previewImage();
  return file.slice(0, file.size, "application/octet-stream");
};
