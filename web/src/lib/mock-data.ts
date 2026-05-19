import type { CurrentUser, LabInstance, LabState } from "./types";

export const demoUsers: Record<CurrentUser["role"], CurrentUser> = {
  student: {
    id: "student-001",
    displayName: "Иван Петров",
    email: "student-001@emulator.local",
    role: "student",
  },
  teacher: {
    id: "teacher-001",
    displayName: "Преподаватель Сергей",
    email: "teacher-001@emulator.local",
    role: "teacher",
  },
  admin: {
    id: "admin-001",
    displayName: "Администратор КИ",
    email: "admin-001@emulator.local",
    role: "admin",
  },
};

export const demoLab: LabInstance = {
  id: "lab-demo-001",
  title: "Лабораторная работа №1: пользователи и права",
  courseName: "Linux основы",
  state: "ready",
  stateReason: "Стенд готов к работе",
  createdAt: new Date(Date.now() - 17 * 60_000).toISOString(),
  cleanupAt: new Date(Date.now() + 103 * 60_000).toISOString(),
  publicIp: "203.0.113.42",
  sshLogin: "ubuntu",
  sshCommand: "ssh -i lab-key.pem ubuntu@203.0.113.42",
  quota: { cpu: 67, ram: 54, disk: 41 },
  checkRuns: [
    {
      id: "check-3",
      title: "initial-check",
      state: "passed",
      finishedAt: new Date(Date.now() - 4 * 60_000).toISOString(),
    },
    {
      id: "check-2",
      title: "linux-users",
      state: "failed",
      failedSteps: 2,
      finishedAt: new Date(Date.now() - 9 * 60_000).toISOString(),
    },
    {
      id: "check-1",
      title: "ssh-connectivity",
      state: "passed",
      finishedAt: new Date(Date.now() - 13 * 60_000).toISOString(),
    },
  ],
};

export type TeacherLabRow = {
  id: string;
  student: string;
  email: string;
  state: LabState;
  startedAt: string;
  cleanupAt?: string;
  frozenUntil?: string;
  lastCheck: "passed" | "failed" | "running" | "none";
  project: string;
};

export const teacherLabs: TeacherLabRow[] = [
  {
    id: "lab-ivan",
    student: "Иван Петров",
    email: "student-001@emulator.local",
    state: "ready",
    startedAt: "12:43",
    cleanupAt: "14:43",
    lastCheck: "passed",
    project: "proj-linux-007",
  },
  {
    id: "lab-anna",
    student: "Анна Смирнова",
    email: "student-002@emulator.local",
    state: "checking",
    startedAt: "12:21",
    cleanupAt: "14:21",
    lastCheck: "running",
    project: "proj-linux-011",
  },
  {
    id: "lab-oleg",
    student: "Олег Иванов",
    email: "student-003@emulator.local",
    state: "frozen",
    startedAt: "11:55",
    frozenUntil: "завтра 11:55",
    lastCheck: "failed",
    project: "proj-linux-014",
  },
  {
    id: "lab-maria",
    student: "Мария Кузнецова",
    email: "student-004@emulator.local",
    state: "deploying",
    startedAt: "12:48",
    lastCheck: "none",
    project: "proj-linux-020",
  },
  {
    id: "lab-sergey",
    student: "Сергей Орлов",
    email: "student-005@emulator.local",
    state: "failed",
    startedAt: "12:07",
    lastCheck: "failed",
    project: "proj-linux-003",
  },
];

export type PoolGroup = {
  course: string;
  free: number;
  allocated: number;
  cleaning: number;
  quarantine: number;
};

export const poolGroups: PoolGroup[] = [
  { course: "Linux основы", free: 7, allocated: 5, cleaning: 2, quarantine: 1 },
  { course: "Nginx и конфигурация", free: 5, allocated: 4, cleaning: 1, quarantine: 0 },
  { course: "Kubernetes старт", free: 2, allocated: 7, cleaning: 1, quarantine: 1 },
];

export const auditEvents = [
  {
    time: "12:48:22",
    kind: "lab.state_changed",
    actor: "system",
    detail: "lab-maria -> deploying",
  },
  {
    time: "12:47:58",
    kind: "project.allocated",
    actor: "system",
    detail: "proj-linux-020 -> lab-maria",
  },
  {
    time: "12:46:10",
    kind: "quota.blocked",
    actor: "system",
    detail: "prediction 92% > threshold 90%",
  },
  {
    time: "12:43:39",
    kind: "settings.changed",
    actor: "teacher-001",
    detail: "lab_lifetime: 2h -> 3h",
  },
  {
    time: "12:40:03",
    kind: "project.quarantined",
    actor: "system",
    detail: "proj-linux-014 cleanup failures: 3",
  },
];

export const quarantineProjects = [
  {
    id: "proj-linux-014",
    course: "Linux основы",
    since: "12:40",
    failures: 3,
    error: "nova server delete timeout",
  },
  {
    id: "proj-k8s-002",
    course: "Kubernetes старт",
    since: "11:23",
    failures: 3,
    error: "neutron router interface still attached",
  },
];
