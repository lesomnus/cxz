import type { ReactNode } from "react";
import {
  Button,
  ImageViewer,
  ImageViewerViewport,
  ImageViewerControls,
} from "@lesomnus/nox";
import { t } from "#src/shared/i18n/i18n.ts";
export function ImagePreviewProvider({
  name,
  ...props
}: {
  name: string;
  load: (signal: AbortSignal) => Promise<Blob>;
  children: ReactNode;
}) {
  return <ImageViewer alt={name} {...props} />;
}
export const ImagePreviewBody = ImageViewerViewport;
export function ImagePreviewActions({ close }: { close: () => void }) {
  return (
    <ImageViewerControls>
      <Button type="button" data-preview-close onClick={close}>
        {t("Close")}
      </Button>
    </ImageViewerControls>
  );
}
