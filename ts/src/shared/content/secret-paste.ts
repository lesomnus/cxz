import patterns from "../../../../internal/secretpaste/patterns.json?raw";

const rules = (
  JSON.parse(patterns) as {
    name: string;
    pattern: string;
    validator?: "jwt";
  }[]
).map((rule) => ({ ...rule, compiled: new RegExp(`^(?:${rule.pattern})$`) }));

/** Recognize the entire pasted token, with no network or substring scanning. */
export function detectSecretPaste(text: string): string | undefined {
  const token = text.trim();
  return rules.find(
    (rule) =>
      rule.compiled.test(token) &&
      (!rule.validator || (rule.validator === "jwt" && isJWT(token))),
  )?.name;
}

function decode(part: string) {
  const binary = atob(part.replaceAll("-", "+").replaceAll("_", "/"));
  return new TextDecoder("utf-8", { fatal: true }).decode(
    Uint8Array.from(binary, (character) => character.charCodeAt(0)),
  );
}

function isJWT(token: string) {
  try {
    const [headerPart, payloadPart, signature] = token.split(".");
    const header: unknown = JSON.parse(decode(headerPart));
    const payload: unknown = JSON.parse(decode(payloadPart));
    // This is format recognition; all algorithms and expired tokens are
    // treated as sensitive. Header field order and issuer don't matter.
    atob(signature.replaceAll("-", "+").replaceAll("_", "/"));
    return (
      !!header &&
      typeof header === "object" &&
      !Array.isArray(header) &&
      "alg" in header &&
      typeof header.alg === "string" &&
      !!header.alg &&
      !!payload &&
      typeof payload === "object" &&
      !Array.isArray(payload)
    );
  } catch {
    return false;
  }
}
