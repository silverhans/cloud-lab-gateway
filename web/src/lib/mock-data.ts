import type { CurrentUser } from "./types";

export const demoUsers: Record<CurrentUser["role"], CurrentUser> = {
  student: {
    id: "student-001",
    display_name: "Иван Петров",
    email: "student-001@emulator.local",
    role: "student",
  },
  teacher: {
    id: "teacher-001",
    display_name: "Преподаватель Сергей",
    email: "teacher-001@emulator.local",
    role: "teacher",
  },
  admin: {
    id: "admin-001",
    display_name: "Администратор КИ",
    email: "admin-001@emulator.local",
    role: "admin",
  },
};
