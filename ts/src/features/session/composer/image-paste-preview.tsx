import { useCallback } from "react";
import { ImagePreviewDialog } from "#src/shared/components/image-preview-dialog.tsx";
import type { ComposerPaste } from "./composer-pastes";
import type { DownloadAttachment } from "./composer-download";

export function ImagePastePreview({
  paste,
  download,
  close,
}: {
  paste: ComposerPaste;
  download?: DownloadAttachment;
  close: () => void;
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
    <ImagePreviewDialog
      name={paste.attachment!.name}
      load={load}
      close={close}
    />
  );
}
