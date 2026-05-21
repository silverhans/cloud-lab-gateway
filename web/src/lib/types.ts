import type { components } from "./api.gen";

export type Problem = components["schemas"]["Problem"];
export type Role = components["schemas"]["UserMe"]["role"];
export type CurrentUser = components["schemas"]["UserMe"];
export type UserCourse = NonNullable<CurrentUser["courses"]>[number];

export type LabState = components["schemas"]["LabState"];
export type LabInstance = components["schemas"]["LabInstance"];
export type LabInstanceDetail = components["schemas"]["LabInstanceDetail"];
export type LabDeployStep = components["schemas"]["LabDeployStep"];
export type CheckRun = components["schemas"]["CheckRun"];
export type CheckRunDetail = components["schemas"]["CheckRunDetail"];

export type Project = components["schemas"]["Project"];
export type ProjectState = components["schemas"]["ProjectState"];
export type QuotaSnapshot = components["schemas"]["QuotaSnapshot"];
export type Setting = components["schemas"]["Setting"];
export type SettingUpdate = components["schemas"]["SettingUpdate"];
export type AuditEvent = components["schemas"]["AuditEvent"];

export type SSEEvent =
  | { type: "lab.state_changed"; labId: string; state: LabState; reason: string }
  | { type: "lab.deploy_progress"; labId: string; step: string; status: "done" | "running" }
  | { type: "check.state_changed"; labId: string; checkId: string; state: NonNullable<CheckRun["state"]> }
  | { type: "quota.snapshot"; vcpus?: number; ram?: number; disk?: number; max?: number };
