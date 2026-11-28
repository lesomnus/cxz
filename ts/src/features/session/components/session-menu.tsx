import { useRef, useState, type ReactNode } from "react";
import type { Session } from "#gen/cxz/session_pb";
import { ActionMenu } from "@lesomnus/nox";
import { EditorOptions } from "#src/shared/editor/editor-options.tsx";
import { ConfirmationDialog } from "#src/shared/components/confirmation-dialog.tsx";
import { t } from "#src/shared/i18n/i18n.ts";
import { useLocale } from "#src/shared/i18n/i18n-react.tsx";
import actionStyles from "#src/shared/components/resource-action.module.css";
import { SessionActionIcon } from "./session-action-icon";
import {
  sessionStartLabel,
  type SessionOperation,
} from "#src/features/session/model/session-actions.ts";
import {
  controlTarget,
  type SessionTarget,
} from "#src/features/session/model/session-control-batch.ts";

export type SessionMenuProps = {
  session?: Session;
  busy: boolean;
  open?: boolean;
  onOpenChange?: (open: boolean) => void;
  manage: (operation: SessionOperation, run: string) => Promise<boolean>;
  erase?: (target: SessionTarget) => Promise<boolean>;
  fork?: () => void | Promise<unknown>;
  upload?: (directory?: boolean) => void;
  terminal?: {
    visible: boolean;
    available: boolean;
    toggle: () => void;
  };
  tools?: {
    label: string;
    icon?: ReactNode;
    run: () => void;
    disabled?: boolean;
  }[];
};

// Mirror the button's inset across the wrapper's visible top border.
function toolbarMenuGap(trigger: HTMLButtonElement) {
  const wrapper = trigger.closest(".composer-wrapper");
  if (!wrapper) return 4;
  const inset =
    trigger.getBoundingClientRect().top - wrapper.getBoundingClientRect().top;
  return 2 * Math.max(0, inset);
}

export function SessionMenu({
  session,
  busy,
  open: menuOpen,
  onOpenChange,
  manage,
  erase,
  fork,
  upload,
  terminal,
  tools = [],
}: SessionMenuProps) {
  useLocale();
  const start = sessionStartLabel(session);
  const unavailable = busy || !session?.status?.runId;
  const run = session?.status?.runId ?? "";
  const [confirmation, setConfirmation] = useState<{
    target: SessionTarget;
    kind: "stop" | "restart" | "erase";
  }>();
  // Fork needs the menu to stay open until it finishes, so every item here
  // runs under awaitAction: the menu closes after the item rather than before
  // it, handing focus back to its trigger as it goes. A confirmation opened
  // from inside the item would therefore come up over a menu that is still
  // there, and the focus the close hands back lands nowhere -- a modal is
  // open -- while the menu item it was recorded against is unmounted with the
  // menu. So an item only records what was asked for, and the dialog is opened
  // once the menu reports itself closed and the trigger has the focus again.
  const requested = useRef<"stop" | "restart" | "erase">(undefined);
  function confirm(kind: "stop" | "restart" | "erase") {
    requested.current = kind;
  }
  function menuOpenChanged(open: boolean) {
    onOpenChange?.(open);
    const kind = requested.current;
    requested.current = undefined;
    if (open || !kind || !session) return;
    setConfirmation({ target: controlTarget(session), kind });
  }
  return (
    <>
      <ActionMenu
        label={t("Session menu")}
        open={menuOpen}
        onOpenChange={menuOpenChanged}
        placement="above"
        gap={toolbarMenuGap}
        scrollbarOffset={-8}
        motion={{ from: "bottom-right", effect: "reveal" }}
        disabled={!session}
        awaitAction={!!fork}
        footer={<EditorOptions scope="session" />}
        groups={[
          {
            label: "",
            layout: "row",
            items: [
              {
                label: t(start ?? "Stop"),
                className: start ? actionStyles.start : undefined,
                icon: <SessionActionIcon action={start ? "resume" : "stop"} />,
                disabled: start ? busy : unavailable,
                run: () =>
                  start ? void manage("resume", run) : confirm("stop"),
              },
              {
                label: t("Restart"),
                icon: <SessionActionIcon action="restart" />,
                disabled: unavailable,
                run: () => confirm("restart"),
              },
              ...(fork
                ? [
                    {
                      label: t("Fork"),
                      icon: <SessionActionIcon action="fork" />,
                      disabled:
                        busy ||
                        !session?.status?.runId ||
                        !["idle", "stopped", "interrupted", "failed"].includes(
                          session?.status?.state ?? "",
                        ) ||
                        !!session?.status?.pending.length ||
                        !!session?.status?.queued,
                      run: fork,
                    },
                  ]
                : []),
            ],
          },
          {
            label: "",
            items: [
              {
                label: t("Terminal"),
                icon: <SessionActionIcon action="terminal" />,
                shortcut: ["Ctrl", "`"],
                checked: terminal?.visible ?? false,
                disabled: !terminal?.available,
                run: () => terminal?.toggle(),
              },
              ...tools,
              ...(upload
                ? [
                    {
                      label: t("Upload files"),
                      icon: <SessionActionIcon action="upload" />,
                      disabled: busy || !session?.status?.runId,
                      run: () => upload(),
                    },
                    {
                      label: t("Upload folder"),
                      icon: <SessionActionIcon action="folder" />,
                      disabled: busy || !session?.status?.runId,
                      run: () => upload(true),
                    },
                  ]
                : []),
              {
                label: t("Erase"),
                icon: <SessionActionIcon action="erase" />,
                danger: true,
                disabled: busy || !erase,
                run: () => confirm("erase"),
              },
            ],
          },
        ]}
      />
      {confirmation?.target.runtimeId === session?.runtimeId &&
        confirmation && (
          <ConfirmationDialog
            key={`${confirmation.target.runtimeId}:${confirmation.kind}:${confirmation.target.run}`}
            title={
              confirmation.kind === "erase"
                ? t("Erase session")
                : confirmation.kind === "restart"
                  ? t("Restart session")
                  : t("Stop session")
            }
            confirmLabel={
              confirmation.kind === "erase"
                ? t("Erase")
                : confirmation.kind === "restart"
                  ? t("Restart")
                  : t("Stop")
            }
            close={() => setConfirmation(undefined)}
            failureMessage={t(
              "Action failed. Check session status before retrying.",
            )}
            execute={() =>
              confirmation.kind === "erase"
                ? (erase?.(confirmation.target) ?? false)
                : manage(confirmation.kind, confirmation.target.run)
            }
          >
            {confirmation.kind === "erase" && (
              <p>
                <strong>{confirmation.target.name}</strong>
              </p>
            )}
            <p>
              {confirmation.kind === "erase"
                ? t(
                    "Permanently deletes this session and its stored data. This cannot be undone.",
                  )
                : t(
                    "Active work stops and pending approvals are cleared. Conversation history, login and permission policy are retained.",
                  )}
            </p>
            {confirmation.kind === "restart" && (
              <p className="muted">
                {t(
                  "This restarts the agent; it does not recreate the project container.",
                )}
              </p>
            )}
          </ConfirmationDialog>
        )}
    </>
  );
}
