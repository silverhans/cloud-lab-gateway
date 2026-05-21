import { z } from "zod";
import type { SSEEvent } from "./types";

const baseEventSchema = z.object({ type: z.string() }).passthrough();

export function parseSSEEvent(raw: string): SSEEvent | null {
  const parsedJson = JSON.parse(raw) as unknown;
  const parsed = baseEventSchema.safeParse(parsedJson);
  if (!parsed.success) return null;
  const event = parsed.data;
  const labId = stringField(event, "labId") ?? stringField(event, "lab_id");
  switch (event.type) {
    case "lab.state_changed": {
      const state = stringField(event, "state");
      if (!labId || !state) return null;
      return { type: event.type, labId, state, reason: stringField(event, "reason") ?? "" } as SSEEvent;
    }
    case "lab.deploy_progress": {
      const step = stringField(event, "step");
      const status = stringField(event, "status");
      if (!labId || !step || !status) return null;
      return { type: event.type, labId, step, status: status === "done" ? "done" : "running" };
    }
    case "check.state_changed": {
      const checkId = stringField(event, "checkId") ?? stringField(event, "check_id");
      const state = stringField(event, "state");
      if (!labId || !checkId || !state) return null;
      return { type: event.type, labId, checkId, state } as SSEEvent;
    }
    case "quota.snapshot":
      return {
        type: event.type,
        vcpus: numberField(event, "vcpus") ?? numberField(event, "cpu"),
        ram: numberField(event, "ram"),
        disk: numberField(event, "disk"),
        max: numberField(event, "max"),
      };
    default:
      return null;
  }
}

function stringField(source: Record<string, unknown>, key: string): string | undefined {
  const value = source[key];
  return typeof value === "string" ? value : undefined;
}

function numberField(source: Record<string, unknown>, key: string): number | undefined {
  const value = source[key];
  return typeof value === "number" ? value : undefined;
}
