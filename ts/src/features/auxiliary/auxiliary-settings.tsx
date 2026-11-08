import { useEffect, useState } from "react";
import { create } from "@bufbuild/protobuf";
import { Button, FormField, Switch, ValueMenu } from "@lesomnus/nox";
import {
  AgentProfileSchema,
  type AgentProfile,
  AuxKind,
} from "#gen/cxz/installation_pb";
import type { Session } from "#gen/cxz/session_pb";
import type { Connection } from "#src/shared/api/connection.ts";
import { useRemote } from "#src/shared/api/use-remote.ts";
import {
  ACCOUNT_INFO_TTL,
  ACCOUNT_LIST_KEY,
} from "#src/shared/api/read-cache.ts";
import {
  accountRef,
  listAccounts,
} from "#src/features/accounts/account-data.ts";
import { AccountSectionPicker } from "#src/features/accounts/account-section-picker.tsx";
import { LoginPanel } from "#src/features/accounts/login-panel.tsx";
import { ErrorNotice } from "#src/shared/errors/error-notice.tsx";
import { auxKinds } from "./auxiliary-controls";
import styles from "./auxiliary-settings.module.css";

export function AuxiliarySettings({
  c,
  sessions = [],
}: {
  c: Connection;
  sessions?: Session[];
}) {
  const configuration = useRemote(
    (signal) => c.installation.agents({}, { signal, timeoutMs: 10000 }),
    [c],
  );
  const accounts = useRemote((signal) => listAccounts(c, signal), [c], {
    cache: { store: c.reads, key: ACCOUNT_LIST_KEY, ttl: ACCOUNT_INFO_TTL },
  });
  const [profiles, setProfiles] = useState<AgentProfile[]>([]);
  const [account, setAccount] = useState("");
  const [busy, setBusy] = useState(false);
  const [login, setLogin] = useState(false);
  const [error, setError] = useState("");
  useEffect(() => {
    if (!configuration.data) return;
    const saved = configuration.data;
    const shared =
      saved.account ||
      saved.profiles.find((p) => p.enabled && p.account)?.account ||
      saved.profiles.find((p) => p.account)?.account ||
      "";
    setAccount(shared);
    setProfiles(
      auxKinds.map(
        ({ kind }) =>
          saved.profiles.find((p) => p.kind === kind) ??
          create(AgentProfileSchema, { kind, account: shared }),
      ),
    );
  }, [configuration.data]);
  const models = useRemote(
    (signal) =>
      c.accounts.models(accountRef(account), { signal, timeoutMs: 20000 }),
    [c, account],
    { enabled: !!account },
  );
  const disabled = busy || login || configuration.loading || accounts.loading;
  const accountAvailable = !!accounts.data?.some(
    (candidate) => candidate.alias === account,
  );
  function update(kind: AuxKind, patch: Partial<AgentProfile>) {
    setProfiles((old) =>
      old.map((p) => (p.kind === kind ? { ...p, ...patch } : p)),
    );
  }
  function chooseAccount(next: string) {
    if (next === account) return;
    setAccount(next);
    setError("");
    setProfiles((old) =>
      old.map((p) => ({
        ...p,
        account: next,
        enabled: false,
        model: "",
        effort: "",
      })),
    );
  }
  return (
    <div className={styles.content} aria-label="Auxiliary AI settings">
      <AccountSectionPicker
        c={c}
        accounts={accounts.data ?? []}
        sessions={sessions}
        value={account}
        choose={chooseAccount}
        disabled={disabled}
      />
      {(error || configuration.error || accounts.error || models.error) && (
        <ErrorNotice
          error={error || configuration.error || accounts.error || models.error}
        />
      )}
      {configuration.loading && !profiles.length && (
        <p className={styles.muted}>Loading auxiliary settings…</p>
      )}
      <div className={styles.accountActions}>
        <span className={styles.muted}>
          Shared by Summary, Suggestion and Title
        </span>
        {accountAvailable && (
          <Button
            disabled={disabled || models.loading}
            onClick={() => setLogin(true)}
          >
            Log in to helper account
          </Button>
        )}
      </div>
      {models.data?.needsLogin && (
        <p className={styles.muted}>This helper account needs a login.</p>
      )}
      {login && (
        <LoginPanel
          modal
          c={c}
          target={{ account }}
          complete={() => {
            setLogin(false);
            models.reload();
          }}
          cancel={() => setLogin(false)}
        />
      )}
      {profiles.map((profile) => {
        const label = auxKinds.find((k) => k.kind === profile.kind)!.label;
        const model = models.data?.models.find((m) => m.id === profile.model);
        return (
          <section
            className={styles.profile}
            aria-label={`${label} configuration`}
            key={profile.kind}
          >
            <div className={styles.heading}>
              <h2>{label}</h2>
              <Switch
                label={`${label} enabled`}
                checked={profile.enabled}
                disabled={disabled || !accountAvailable || !profile.model}
                onChange={(enabled) => update(profile.kind, { enabled })}
              />
            </div>
            <div className={styles.fields}>
              <FormField label="Model">
                <ValueMenu
                  label={`${label} model`}
                  disabled={disabled || !accountAvailable || models.loading}
                  value={profile.model}
                  options={[
                    {
                      value: "",
                      label: models.loading
                        ? "Loading models…"
                        : "Choose a model",
                      disabled: true,
                    },
                    ...(models.data?.models ?? []).map((m) => ({
                      value: m.id,
                      label: m.name || m.id,
                    })),
                    ...(!model && profile.model
                      ? [{ value: profile.model, label: profile.model }]
                      : []),
                  ]}
                  choose={(model) =>
                    update(profile.kind, { model, effort: "" })
                  }
                />
              </FormField>
              <FormField label="Effort">
                <ValueMenu
                  label={`${label} effort`}
                  disabled={disabled || !profile.model || models.loading}
                  value={profile.effort}
                  options={[
                    { value: "", label: "Provider default" },
                    ...(model?.efforts ?? []).map((value) => ({
                      value,
                      label: value,
                    })),
                  ]}
                  choose={(effort) => update(profile.kind, { effort })}
                />
              </FormField>
            </div>
          </section>
        );
      })}
      <Button
        className={styles.save}
        disabled={
          disabled ||
          !profiles.length ||
          profiles.some((p) => p.enabled && (!accountAvailable || !p.model))
        }
        onClick={async () => {
          setBusy(true);
          setError("");
          try {
            const saved = await c.installation.saveAgents(
              { account, profiles: profiles.map((p) => ({ ...p, account })) },
              { timeoutMs: 20000 },
            );
            setProfiles(saved.profiles);
            setAccount(
              saved.account ||
                saved.profiles.find((p) => p.account)?.account ||
                "",
            );
          } catch (error) {
            setError(String(error));
          } finally {
            setBusy(false);
          }
        }}
      >
        Save configuration
      </Button>
    </div>
  );
}
