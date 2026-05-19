import type { CurrentUser, LabInstance } from "./types";

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
