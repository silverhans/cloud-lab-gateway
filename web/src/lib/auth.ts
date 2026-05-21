import { demoUsers } from "./mock-data";
import type { CurrentUser, Role } from "./types";

const storageKey = "clg.demoUser";

export function demoAuthEnabled(): boolean {
  return import.meta.env.VITE_DEMO_AUTH === "true";
}

export function moodleEmulatorUrl(): string {
  return import.meta.env.VITE_MOODLE_URL ?? "http://localhost:9000";
}

export function getStoredUser(): CurrentUser | null {
  if (!demoAuthEnabled() || typeof window === "undefined") return null;

  const raw = window.localStorage.getItem(storageKey);
  if (!raw) return null;

  try {
    return JSON.parse(raw) as CurrentUser;
  } catch {
    window.localStorage.removeItem(storageKey);
    return null;
  }
}

export function setDemoUser(role: Role): CurrentUser {
  const user = demoUsers[role];
  if (demoAuthEnabled() && typeof window !== "undefined") {
    window.localStorage.setItem(storageKey, JSON.stringify(user));
  }
  return user;
}

export function clearStoredUser(): void {
  if (typeof window === "undefined") return;
  window.localStorage.removeItem(storageKey);
}
