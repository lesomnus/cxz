import { create } from "@bufbuild/protobuf";
import { describe, expect, it } from "vitest";
import { ResponseMetadataSchema } from "../gen/cxz/session_pb";
import { responseInfo } from "./response-info";

describe("response snapshots", () => {
  it("keeps legacy and unknown values empty without inventing defaults", () => {
    expect(responseInfo().label).toBe("");
    expect(responseInfo(create(ResponseMetadataSchema)).label).toBe("");
    expect(
      responseInfo(create(ResponseMetadataSchema, { model: "m" })).label,
    ).toBe("m");
    expect(
      responseInfo(create(ResponseMetadataSchema, { effort: "high" })).label,
    ).toBe("high");
  });
  it("distinguishes reported values from requests in the tooltip", () => {
    const info = responseInfo(
      create(ResponseMetadataSchema, {
        model: "m",
        effort: "high",
        modelSource: "response",
        effortSource: "requested",
      }),
    );
    expect(info.label).toBe("m · high");
    expect(info.description).toBe(
      "Model: m (응답 보고값)\nEffort: high (요청한 설정)",
    );
  });
});
