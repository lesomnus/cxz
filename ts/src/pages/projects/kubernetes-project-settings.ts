import { Validator, type Schema } from "@cfworker/json-schema";
import { parse, parseDocument } from "yaml";
import schemaSource from "./kubernetes-project.schema.yaml?raw";

export const kubernetesProjectSchema: Schema = parse(schemaSource);
export const defaultKubernetesProjectSettings =
  "image: ghcr.io/lesomnus/dev-golang:1.27\nuser: hypnos\n";

const validator = new Validator(kubernetesProjectSchema, "7");

export function readKubernetesProjectSettings(source: string) {
  const document = parseDocument(source);
  const problem = document.errors[0] ?? document.warnings[0];
  if (problem) return { error: problem.message };
  try {
    const value = document.contents === null ? {} : document.toJS();
    const result = validator.validate(value);
    if (!result.valid) {
      const { instanceLocation, error } = result.errors[0];
      return {
        error:
          instanceLocation === "#"
            ? error
            : `${instanceLocation.slice(1)}: ${error}`,
      };
    }
    return { document, value: value as Record<string, unknown> };
  } catch (error) {
    return { error: String(error) };
  }
}
