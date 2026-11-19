import { errorInfos } from "#src/shared/api/error-info.ts";

// Decode the standard google.rpc.ErrorInfo detail, not prose that happens to
// mention login. Only the runtime's explicit missing-profile signal can retry
// a creation. Other authentication failures remain errors.
export function requiredSessionLogin(
  error: unknown,
  account: string | undefined,
  creationKey: string,
) {
  for (const { reason, domain, metadata } of errorInfos(error)) {
    const key = metadata.session_key || creationKey;
    // A cached session may only contain an account ID. In that case require
    // the server to name the account and explicitly echo this creation key.
    const matchesAccount =
      account === undefined
        ? metadata.session_key === creationKey
        : metadata.account === account;
    if (
      reason === "PROJECT_LOGIN_REQUIRED" &&
      domain === "cxz.auth" &&
      metadata.account &&
      matchesAccount &&
      key === creationKey
    )
      return { account: metadata.account, key };
  }
}
