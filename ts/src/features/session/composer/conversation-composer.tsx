import { useId, type FormEventHandler, type ReactNode, type Ref } from "react";
import { t } from "#src/shared/i18n/i18n.ts";
import { useLocale } from "#src/shared/i18n/i18n-react.tsx";
import { Button } from "@lesomnus/cxz-ui";
import { ComposerAurora } from "./composer-aurora";
import { TurnControls } from "./turn-controls";
import type { TurnProgress } from "#src/features/session/model/turn-progress.ts";
import { ComposerEditor } from "./composer-editor";
import type { ComposerCommand } from "./composer-commands";
import type { ComposerPaste } from "./composer-pastes";

// Presentation shared by the connected conversation and serverless UI previews.
export function ConversationComposer({
  formRef,
  inputRef,
  draft,
  pastes,
  onChange,
  onSubmit,
  commands,
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
  children,
}: {
  formRef?: Ref<HTMLFormElement>;
  inputRef?: Ref<HTMLDivElement>;
  draft: string;
  pastes: Map<string, ComposerPaste>;
  onChange: (value: string) => void;
  onSubmit: FormEventHandler<HTMLFormElement>;
  commands?: readonly ComposerCommand[];
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
  menu?: ReactNode;
  children?: ReactNode;
}) {
  useLocale();
  const terminalHint = useId();
  const sendHint = useId();
  return (
    <form ref={formRef} className="composer" onSubmit={onSubmit}>
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
          {menu}
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
          <span className="send-control">
            <Button
              className="toolbar-button send"
              type="submit"
              aria-label={t("Send")}
              aria-keyshortcuts="Control+Enter"
              aria-describedby={sendHint}
              disabled={!canSend}
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
          <ComposerEditor
            value={draft}
            readOnly={sending}
            onChange={onChange}
            pastes={pastes}
            canSend={canSend}
            commands={commands}
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
