import { create } from "@bufbuild/protobuf";
import { expect, it } from "vitest";
import {
  SessionSchema,
  SessionEventSchema,
} from "../../../../gen/cxz/session_pb";
import { mergeMetadata } from "./session-metadata";
import { sessionInfo } from "./session-info";
import { modelCatalog } from "../composer/model-settings";

it("preserves model capabilities and quota across hundreds of context updates and out-of-order history", () => {
  const session = create(SessionSchema, {
    agent: "codex",
    status: { runId: "current" },
  });
  const event = (seq: number, kind: string, text: string, value: unknown) =>
    create(SessionEventSchema, {
      seq: BigInt(seq),
      runId: "current",
      kind,
      text,
      payload: new TextEncoder().encode(JSON.stringify(value)),
    });
  const catalog = event(1, "models", "catalog", {
    model: "sample",
    models: [{ id: "sample", efforts: ["low", "high"] }],
  });
  const quota = event(2, "usage", "account/rateLimits/updated", {
    rateLimits: { primary: { usedPercent: 20 } },
  });
  let metadata = mergeMetadata([], [catalog, quota]);
  for (let seq = 3; seq < 1000; seq++)
    metadata = mergeMetadata(metadata, [
      event(seq, "usage", "thread/tokenUsage/updated", {
        tokenUsage: { last: { totalTokens: seq }, modelContextWindow: 128000 },
      }),
    ]);
  metadata = mergeMetadata(metadata, [
    event(10, "usage", "thread/tokenUsage/updated", {
      tokenUsage: { last: { totalTokens: 10 }, modelContextWindow: 128000 },
    }),
  ]);
  expect(modelCatalog(session, metadata)?.models[0].id).toBe("sample");
  expect(sessionInfo(session, metadata).remaining).toBe(80);
  expect(sessionInfo(session, metadata).contextUsed).toBe(999);
  expect(metadata.length).toBeLessThanOrEqual(202);
});
