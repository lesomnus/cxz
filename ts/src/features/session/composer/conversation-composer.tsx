import {
  useId,
  useRef,
  useState,
  type FormEventHandler,
  type ReactNode,
  type Ref,
} from "react";
import { t } from "#src/shared/i18n/i18n.ts";
import { useLocale } from "#src/shared/i18n/i18n-react.tsx";
import { AudioWaveform, Button } from "@lesomnus/cxz-ui";
import { ComposerAurora } from "./composer-aurora";
import { TurnControls } from "./turn-controls";
import type { TurnProgress } from "#src/features/session/model/turn-progress.ts";
import { ComposerEditor, type ComposerEditorHandle } from "./composer-editor";
import type { ComposerCommand } from "./composer-commands";
import type { LookupPaths } from "./composer-paths";
import { attachmentsReady, type ComposerPaste } from "./composer-pastes";
import type { UploadFile } from "./composer-upload";
import { useSpeechInput } from "#src/shared/browser/use-speech-input.ts";
import { MicrophoneControl } from "./microphone-control";
import { useMicrophoneLevel } from "#src/shared/browser/use-microphone-level.ts";
import { useSettings } from "#src/shared/settings/settings.ts";
import microphoneStyles from "./microphone-control.module.css";

// Presentation shared by the connected conversation and serverless UI previews.
export function ConversationComposer({
  formRef,
  inputRef,
  draft,
  pastes,
  onChange,
  onSubmit,
  commands,
  lookupPaths,
  workspace,
  canSend,
  sending,
  busy,
  turn,
  working,
  interrupt,
  latestVisible = false,
  onLatest,
  terminalVisible = false,
  terminalAvailable = false,
  onTerminal,
  menu,
  upload,
  onDraftAssetsChange,
  children,
}: {
  formRef?: Ref<HTMLFormElement>;
  inputRef?: Ref<HTMLDivElement>;
  draft: string;
  pastes: Map<string, ComposerPaste>;
  onChange: (value: string) => void;
  onSubmit: FormEventHandler<HTMLFormElement>;
  commands?: readonly ComposerCommand[];
  lookupPaths?: LookupPaths;
  workspace?: string;
  canSend: boolean;
  sending: boolean;
  busy: boolean;
  turn: TurnProgress;
  working: boolean;
  interrupt: () => void;
  latestVisible?: boolean;
  onLatest?: () => void;
  terminalVisible?: boolean;
  terminalAvailable?: boolean;
  onTerminal?: () => void;
  menu?: ReactNode | ((pick: (directory?: boolean) => void) => ReactNode);
  upload?: UploadFile;
  onDraftAssetsChange?: () => void;
  children?: ReactNode;
}) {
  useLocale();
  const terminalHint = useId();
  const sendHint = useId();
  const editor = useRef<ComposerEditorHandle>(null);
  const { store, snapshot } = useSettings();
  const language =
    typeof snapshot.document["voice.language"] === "string"
      ? (snapshot.document["voice.language"] as string)
      : undefined;
  const [languageError, setLanguageError] = useState("");
  const speech = useSpeechInput({
    disabled: sending,
    language,
    processLocally: snapshot.document["voice.processLocally"] === true,
    onTranscript: (text) => editor.current?.insertText(text),
  });
  const meter = useMicrophoneLevel(speech.listening);
  const files = useRef<HTMLInputElement>(null);
  const directory = useRef<HTMLInputElement>(null);
  const [, refreshUploads] = useState(0);
  const ready = attachmentsReady(draft, pastes);
  const pick = (folder = false) => {
    if (!sending && upload) (folder ? directory : files).current?.click();
  };
  return (
    <form
      ref={formRef}
      className="composer"
      onSubmit={(event) => {
        if (!ready) event.preventDefault();
        else {
          speech.cancel();
          onSubmit(event);
        }
      }}
    >
      {upload && (
        <>
          <input
            type="file"
            ref={files}
            hidden
            multiple
            aria-label={t("Upload files")}
            onChange={(event) => {
              editor.current?.insertFiles(
                Array.from(event.currentTarget.files ?? []),
              );
              event.currentTarget.value = "";
            }}
          />
          <input
            type="file"
            hidden
            multiple
            aria-label={t("Upload folder")}
            ref={(node) => {
              directory.current = node;
              node?.setAttribute("webkitdirectory", "");
            }}
            onChange={(event) => {
              editor.current?.insertFiles(
                Array.from(event.currentTarget.files ?? []),
              );
              event.currentTarget.value = "";
            }}
          />
        </>
      )}
      <div className="composer-wrapper">
        <ComposerAurora active={working} />
        <div className="composer-toolbar">
          <TurnControls turn={turn} busy={busy} interrupt={interrupt} />
          <span
            className="latest-slot"
            data-visible={latestVisible}
            inert={!latestVisible}
            aria-hidden={!latestVisible}
          >
            <Button
              className="toolbar-button latest-button"
              type="button"
              aria-label={t("Latest")}
              onClick={onLatest}
            >
              <svg
                width="18"
                height="18"
                viewBox="0 0 24 24"
                fill="none"
                stroke="currentColor"
                strokeWidth="1.8"
                strokeLinecap="round"
                strokeLinejoin="round"
                aria-hidden="true"
              >
                <path d="M12 5v14m-6-6 6 6 6-6" />
              </svg>
            </Button>
          </span>
          {typeof menu === "function" ? menu(pick) : menu}
          <span className="terminal-control">
            <Button
              className="toolbar-button terminal-toggle"
              type="button"
              aria-label={t("Terminal")}
              aria-pressed={terminalVisible}
              aria-keyshortcuts="Control+Backquote"
              aria-describedby={terminalHint}
              disabled={!terminalAvailable}
              onClick={onTerminal}
            >
              <svg
                width="18"
                height="18"
                viewBox="0 0 24 24"
                fill="none"
                stroke="currentColor"
                strokeWidth="1.8"
                strokeLinecap="round"
                strokeLinejoin="round"
                aria-hidden="true"
              >
                <path d="m4 6 6 6-6 6m9 0h7" />
              </svg>
            </Button>
            <span id={terminalHint} className="send-shortcut" role="tooltip">
              {t("Terminal")} · ctrl+`
            </span>
          </span>
          <MicrophoneControl
            speech={speech}
            disabled={sending}
            language={language}
            languageError={languageError}
            meterUnavailable={meter.status === "unavailable"}
            onLanguage={(language) => {
              try {
                store.set("voice.language", language);
                setLanguageError("");
              } catch (error) {
                setLanguageError(
                  t("Cannot save voice language: {error}", {
                    error: String(error),
                  }),
                );
              }
            }}
          />
          <span className="send-control">
            <Button
              className="toolbar-button send"
              type="submit"
              aria-label={t("Send")}
              aria-keyshortcuts="Control+Enter"
              aria-describedby={sendHint}
              disabled={!canSend || !ready}
            >
              <svg
                width="18"
                height="18"
                viewBox="0 0 24 24"
                fill="none"
                stroke="currentColor"
                strokeWidth="1.8"
                strokeLinecap="round"
                strokeLinejoin="round"
                aria-hidden="true"
              >
                <path d="M12 19V5m-6 6 6-6 6 6" />
              </svg>
            </Button>
            <span id={sendHint} className="send-shortcut" role="tooltip">
              ctrl+enter
            </span>
          </span>
        </div>
        <div
          className="composer-input"
          ref={inputRef}
          data-sending={sending}
          aria-busy={sending}
        >
          <div
            className={microphoneStyles.visualization}
            data-voice-waveform={meter.status}
          >
            <AudioWaveform
              variant="upward"
              active={meter.status === "ready"}
              readLevel={meter.readLevel}
            />
          </div>
          <ComposerEditor
            value={draft}
            readOnly={sending}
            onChange={onChange}
            pastes={pastes}
            canSend={canSend}
            commands={commands}
            lookupPaths={lookupPaths}
            workspace={workspace}
            editorRef={editor}
            upload={upload}
            onUploadChange={() => {
              refreshUploads((value) => value + 1);
              onDraftAssetsChange?.();
            }}
          />
        </div>
      </div>
      {children && (
        <div className="composer-meta" aria-label={t("Session information")}>
          {children}
        </div>
      )}
    </form>
  );
}
