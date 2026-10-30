import { useCallback, type ReactNode } from "react";
import { ImagePreviewProvider } from "#src/shared/components/image-preview.tsx";
import type { ComposerPaste } from "./composer-pastes";
import type { DownloadAttachment } from "./composer-download";

export function ImagePastePreview({
  paste,
  download,
  children,
}: {
  paste: ComposerPaste;
  download?: DownloadAttachment;
  children: ReactNode;
}) {
  const load = useCallback(
    async (signal: AbortSignal) => {
      const attachment = paste.attachment!;
      signal.throwIfAborted();
      const blob =
        attachment.file ??
        (attachment.path && download
          ? await download(attachment.path, signal)
          : undefined);
      signal.throwIfAborted();
      if (!blob) throw new Error("Image unavailable");
      const type =
        attachment.mediaType ||
        (/\.svg$/i.test(attachment.name) ? "image/svg+xml" : blob.type);
      return type && type !== blob.type ? blob.slice(0, blob.size, type) : blob;
    },
    [paste, download],
  );
  return (
    <ImagePreviewProvider name={paste.attachment!.name} load={load}>
      {children}
    </ImagePreviewProvider>
  );
}
