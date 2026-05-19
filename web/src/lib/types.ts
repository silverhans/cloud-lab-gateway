export type Role = "student" | "teacher" | "admin";

export type CurrentUser = {
  id: string;
  displayName: string;
  email: string;
  role: Role;
};

export type LabState =
  | "pending_quota"
  | "pending_project"
  | "deploying"
  | "ready"
  | "checking"
  | "frozen"
  | "failed"
  | "cleaning"
  | "done"
  | "rejected";

export type CheckRun = {
  id: string;
  title: string;
  state: "passed" | "failed" | "running";
  finishedAt?: string;
  failedSteps?: number;
};

export type LabInstance = {
  id: string;
  title: string;
  courseName: string;
  state: LabState;
  stateReason: string;
  createdAt: string;
  cleanupAt?: string;
  unfreezeAt?: string;
  publicIp?: string;
  sshLogin?: string;
  sshCommand?: string;
  quota?: {
    cpu: number;
    ram: number;
    disk: number;
  };
  checkRuns: CheckRun[];
};

export type SSEEvent =
  | { type: "lab.state_changed"; labId: string; state: LabState; reason: string }
  | { type: "lab.deploy_progress"; labId: string; step: string; status: "done" | "running" }
  | { type: "check.state_changed"; labId: string; checkId: string; state: CheckRun["state"] }
  | { type: "quota.snapshot"; cpu: number; ram: number; disk: number };
