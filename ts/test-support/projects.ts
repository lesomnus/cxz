import { Buffer } from "node:buffer";
import { expect, type APIRequestContext } from "@playwright/test";
import { parse } from "@lesomnus/payday/pdid";

export function projectFixture(index = 1) {
  const uuid = `00000000-0000-8000-8007-${index.toString(16).padStart(12, "0")}`;
  return { id: Buffer.from(parse(uuid).bytes).toString("base64"), uuid };
}

export async function browserProject(request: APIRequestContext) {
  const response = await request.post("/cxz.ProjectService/Get", {
    headers: {
      Origin: "https://127.0.0.1:18081",
      "Connect-Protocol-Version": "1",
    },
    data: { ref: { alias: "demo" }, select: { all: true } },
  });
  expect(response.ok(), await response.text()).toBe(true);
  return (await response.json()) as {
    id: string;
    alias: string;
    name: string;
  };
}
