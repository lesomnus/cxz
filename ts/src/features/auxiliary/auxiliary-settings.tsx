import { useEffect, useState } from "react";
import { create } from "@bufbuild/protobuf";
import { FieldLabel, Section, Switch, ValueMenu } from "@lesomnus/nox";
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
import { Icon } from "#src/shared/icons/icon.tsx";
import { auxKinds } from "./auxiliary-controls";
import actionStyles from "#src/shared/components/resource-action.module.css";
import styles from "./auxiliary-settings.module.css";

const mixedValue = "__mixed__";
type SavedConfiguration = { account: string; profiles: AgentProfile[] };
const descriptions = {
  [AuxKind.SUMMARY]:
    "Summarize completed replies to keep the conversation easy to follow.",
  [AuxKind.SUGGESTION]:
    "Draft a suggested next message in the conversation input.",
  [AuxKind.TITLE]:
    "Name new sessions automatically from their early conversation.",
};
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
  const [savedConfiguration, setSavedConfiguration] =
    useState<SavedConfiguration>();
  const [busy, setBusy] = useState(false);
  const [login, setLogin] = useState(false);
  const [error, setError] = useState("");
  useEffect(() => {
    if (!configuration.data) return;
    acceptSaved(configuration.data);
  }, [configuration.data]);
  function acceptSaved(saved: SavedConfiguration) {
    const shared =
      saved.account ||
      saved.profiles.find((p) => p.enabled && p.account)?.account ||
      saved.profiles.find((p) => p.account)?.account ||
      "";
    const restored = auxKinds.map(
      ({ kind }) =>
        saved.profiles.find((p) => p.kind === kind) ??
        create(AgentProfileSchema, { kind, account: shared }),
    );
    setSavedConfiguration({ account: shared, profiles: restored });
    setAccount(shared);
    setProfiles(restored);
  }
  const models = useRemote(
    (signal) =>
      c.accounts.models(accountRef(account), { signal, timeoutMs: 20000 }),
    [c, account],
    { enabled: !!account },
  );
  const disabled = busy || login || configuration.loading || accounts.loading;
  const dirty =
    !!savedConfiguration &&
    (account !== savedConfiguration.account ||
      profiles.some((profile) => {
        const saved = savedConfiguration.profiles.find(
          (p) => p.kind === profile.kind,
        );
        return (
          !saved ||
          profile.account !== saved.account ||
          profile.model !== saved.model ||
          profile.effort !== saved.effort ||
          profile.enabled !== saved.enabled
        );
      }));
  const accountAvailable = !!accounts.data?.some(
    (candidate) => candidate.alias === account,
  );
  // Preserve older per-task choices until the user explicitly picks a common
  // value. An unconfigured task inherits the common model when enabled/saved.
  const configured = profiles.filter((profile) => profile.model);
  const modelValues = [...new Set(configured.map((profile) => profile.model))];
  const effortValues = [
    ...new Set(configured.map((profile) => profile.effort)),
  ];
  const mixedModel = modelValues.length > 1;
  const mixedEffort = effortValues.length > 1;
  const model = modelValues.length === 1 ? modelValues[0] : "";
  const effort = effortValues.length === 1 ? effortValues[0] : "";
  const selectedModel = models.data?.models.find((entry) => entry.id === model);
  const authentication = !accountAvailable
    ? "No account"
    : models.loading
      ? "Checking…"
      : models.data?.needsLogin
        ? "Login required"
        : models.error
          ? "Unavailable"
          : models.data
            ? "Ready"
            : "—";
  function update(kind: AuxKind, patch: Partial<AgentProfile>) {
    setProfiles((old) =>
      old.map((profile) =>
        profile.kind === kind ? { ...profile, ...patch } : profile,
      ),
    );
  }
  function chooseAccount(next: string) {
    if (next === account) return;
    setAccount(next);
    setError("");
    setProfiles((old) =>
      old.map((profile) => ({
        ...profile,
        account: next,
        enabled: false,
        model: "",
        effort: "",
      })),
    );
  }
  async function save() {
    setBusy(true);
    setError("");
    try {
      const saved = await c.installation.saveAgents(
        {
          account,
          profiles: profiles.map((profile) => ({
            ...profile,
            account,
            model: mixedModel ? profile.model : model,
            effort: mixedEffort || mixedModel ? profile.effort : effort,
          })),
        },
        { timeoutMs: 20000 },
      );
      acceptSaved(saved);
    } catch (error) {
      setError(String(error));
    } finally {
      setBusy(false);
    }
  }
  function revert() {
    if (!savedConfiguration || disabled) return;
    setAccount(savedConfiguration.account);
    setProfiles(savedConfiguration.profiles);
    setError("");
  }
  return (
    <div className={styles.content} aria-label="Auxiliary AI settings">
      <div
        className={styles.configuration}
        role="group"
        aria-label="Auxiliary configuration"
      >
        <div className={styles.account}>
          <AccountSectionPicker
            c={c}
            accounts={accounts.data ?? []}
            sessions={sessions}
            value={account}
            choose={chooseAccount}
            disabled={disabled}
          />
        </div>
        <ValueMenu
          label="Auxiliary model"
          variant="section"
          alignItemWithTrigger
          value={mixedModel ? mixedValue : model}
          disabled={
            disabled ||
            !accountAvailable ||
            models.loading ||
            models.data?.needsLogin
          }
          renderLabel={(option) => (
            <SelectionLabel label="Model" value={option.label} />
          )}
          options={[
            {
              value: "",
              label: models.loading ? "Loading models…" : "Choose a model",
              disabled: true,
            },
            ...(mixedModel
              ? [{ value: mixedValue, label: "Mixed models", disabled: true }]
              : []),
            ...(models.data?.models ?? []).map((entry) => ({
              value: entry.id,
              label: entry.name || entry.id,
            })),
            ...(!selectedModel && model
              ? [{ value: model, label: model }]
              : []),
          ]}
          choose={(model) =>
            setProfiles((old) =>
              old.map((profile) => ({ ...profile, model, effort: "" })),
            )
          }
        />
        <ValueMenu
          label="Auxiliary effort"
          variant="section"
          alignItemWithTrigger
          value={mixedEffort ? mixedValue : effort}
          disabled={
            disabled || !model || models.loading || models.data?.needsLogin
          }
          renderLabel={(option) => (
            <SelectionLabel label="Effort" value={option.label} />
          )}
          options={[
            { value: "", label: "Provider default" },
            ...(mixedEffort
              ? [{ value: mixedValue, label: "Mixed efforts", disabled: true }]
              : []),
            ...(selectedModel?.efforts ?? []).map((value) => ({
              value,
              label: value,
            })),
            ...(!mixedEffort &&
            effort &&
            !selectedModel?.efforts.includes(effort)
              ? [{ value: effort, label: effort }]
              : []),
          ]}
          choose={(effort) =>
            setProfiles((old) => old.map((profile) => ({ ...profile, effort })))
          }
        />
        <div className={styles.actions}>
          <Section
            as="button"
            className={styles.authentication}
            aria-label="Log in"
            aria-description={authentication}
            aria-busy={models.loading}
            disabled={disabled || !accountAvailable || models.loading}
            onClick={() => setLogin(true)}
          >
            <span
              className={styles.authenticationState}
              role="status"
              data-needs-login={models.data?.needsLogin || undefined}
            >
              <SelectionLabel label="Authentication" value={authentication} />
            </span>
            <Icon name="login" className={styles.login} />
          </Section>
          <Section
            as="button"
            className={actionStyles.button}
            aria-label="Save configuration"
            title="Save configuration"
            disabled={
              disabled ||
              !profiles.length ||
              profiles.some(
                (profile) =>
                  profile.enabled &&
                  (!accountAvailable || !(profile.model || model)),
              )
            }
            onClick={save}
          >
            <Icon name="save" />
          </Section>
          <Section
            as="button"
            className={actionStyles.button}
            aria-label="Revert changes"
            title="Revert changes"
            disabled={disabled || !dirty}
            onClick={revert}
          >
            <Icon name="undo" />
          </Section>
        </div>
      </div>
      {(error || configuration.error || accounts.error || models.error) && (
        <ErrorNotice
          error={error || configuration.error || accounts.error || models.error}
        />
      )}
      {configuration.loading && !profiles.length && (
        <p className={styles.muted}>Loading auxiliary settings…</p>
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
      <div className={styles.profiles}>
        {profiles.map((profile) => {
          const label = auxKinds.find(
            (kind) => kind.kind === profile.kind,
          )!.label;
          const unavailable =
            disabled || !accountAvailable || !(profile.model || model);
          const toggle = (enabled: boolean) =>
            update(profile.kind, {
              enabled,
              model: profile.model || model,
              effort: mixedEffort ? profile.effort : effort,
            });
          return (
            <Section
              className={styles.profile}
              aria-label={`${label} configuration`}
              data-disabled={unavailable || undefined}
              key={profile.kind}
              onClick={(event) => {
                if (
                  unavailable ||
                  (event.target instanceof Element &&
                    event.target.closest("button"))
                )
                  return;
                toggle(!profile.enabled);
              }}
            >
              <div className={styles.profileHeader}>
                <h2>{label}</h2>
                <Switch
                  variant="compact"
                  label={`${label} enabled`}
                  checked={profile.enabled}
                  disabled={unavailable}
                  onChange={toggle}
                />
              </div>
              <p>{descriptions[profile.kind as keyof typeof descriptions]}</p>
            </Section>
          );
        })}
      </div>
    </div>
  );
}

function SelectionLabel({ label, value }: { label: string; value: string }) {
  return (
    <span className={styles.selection}>
      <span className={styles.selectionName} aria-hidden="true">
        <FieldLabel label={label} />
      </span>
      <span className={styles.selectionValue}>{value}</span>
    </span>
  );
}
