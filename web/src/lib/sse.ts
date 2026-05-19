import { z } from "zod";

export const sseEventSchema = z.discriminatedUnion("type", [
  z.object({
    type: z.literal("lab.state_changed"),
    labId: z.string(),
    state: z.string(),
    reason: z.string(),
  }),
  z.object({
    type: z.literal("lab.deploy_progress"),
    labId: z.string(),
    step: z.string(),
    status: z.enum(["done", "running"]),
  }),
  z.object({
    type: z.literal("check.state_changed"),
    labId: z.string(),
    checkId: z.string(),
    state: z.enum(["passed", "failed", "running"]),
  }),
  z.object({
    type: z.literal("quota.snapshot"),
    cpu: z.number(),
    ram: z.number(),
    disk: z.number(),
  }),
]);

export type ParsedSSEEvent = z.infer<typeof sseEventSchema>;

export function parseSSEEvent(raw: string): ParsedSSEEvent | null {
  const parsedJson = JSON.parse(raw) as unknown;
  const parsed = sseEventSchema.safeParse(parsedJson);
  return parsed.success ? parsed.data : null;
}
