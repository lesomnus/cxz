import { useState } from "react";
import type { Session } from "#gen/cxz/session_pb";
import { ActionMenu } from "@lesomnus/nox";
import { ComposerOptions } from "#src/features/session/composer/composer-options.tsx";
import { ConfirmationDialog } from "#src/shared/components/confirmation-dialog.tsx";
import { useFloatingCard } from "#src/features/session/cards/floating-card.tsx";
import { t } from "#src/shared/i18n/i18n.ts";
import { useLocale } from "#src/shared/i18n/i18n-react.tsx";
import { SessionActionIcon } from "./session-action-icon";
import { SessionDetails } from "./session-details";
import type { SessionField } from "#src/features/session/model/session-edit.ts";
import {
  resumableStates,
  type SessionOperation,
} from "#src/features/session/model/session-actions.ts";
import type { SessionInfo } from "#src/features/session/model/session-info.ts";

export type SessionMenuProps = {
  session?: Session;
  info: SessionInfo;
  busy: boolean;
  manage: (operation: SessionOperation, run: string) => Promise<boolean>;
  edit: (field: SessionField, value: string) => Promise<Session>;
  fork?: () => void | Promise<unknown>;
  upload?: (directory?: boolean) => void;
  terminal?: {
    visible: boolean;
    available: boolean;
    toggle: () => void;
  };
  tools?: { label: string; run: () => void; disabled?: boolean }[];
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
  info,
  busy,
  manage,
  edit,
  fork,
  upload,
  terminal,
  tools = [],
}: SessionMenuProps) {
  useLocale();
  const open = useFloatingCard();
  const resume = resumableStates.has(session?.status?.state ?? "");
  const unavailable = busy || !session?.status?.runId;
  const run = session?.status?.runId ?? "";
  const [confirmation, setConfirmation] = useState<{
    sessionId: string;
    run: string;
    kind: "stop" | "restart";
  }>();
  function confirm(kind: "stop" | "restart") {
    if (session)
      setConfirmation({
        sessionId: session.runtimeId,
        run,
        kind,
      });
  }
  return (
    <>
      <ActionMenu
        label={t("Session menu")}
        placement="above"
        gap={toolbarMenuGap}
        scrollbarOffset={8}
        motion={{ from: "bottom-right", effect: "reveal" }}
        disabled={!session}
        status={session?.status?.state}
        footer={<ComposerOptions />}
        groups={[
          {
            label: "",
            layout: "row",
            items: [
              {
                label: resume ? t("Resume") : t("Stop"),
                icon: <SessionActionIcon action={resume ? "resume" : "stop"} />,
                disabled: unavailable,
                run: () =>
                  resume ? void manage("resume", run) : confirm("stop"),
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
                      run: () => void fork(),
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
                label: t("Details"),
                icon: <SessionActionIcon action="details" />,
                run: () =>
                  open({
                    title: t("Session details"),
                    content: () => (
                      <SessionDetails
                        session={session!}
                        info={info}
                        edit={edit}
                      />
                    ),
                  }),
              },
            ],
          },
        ]}
      />
      {confirmation?.sessionId === session?.runtimeId && confirmation && (
        <ConfirmationDialog
          key={`${confirmation.sessionId}:${confirmation.kind}:${confirmation.run}`}
          title={
            confirmation.kind === "restart"
              ? t("Restart session")
              : t("Stop session")
          }
          confirmLabel={
            confirmation.kind === "restart" ? t("Restart") : t("Stop")
          }
          close={() => setConfirmation(undefined)}
          failureMessage={t(
            "Action failed. Check session status before retrying.",
          )}
          execute={() => manage(confirmation.kind, confirmation.run)}
        >
          <p>
            {t(
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
