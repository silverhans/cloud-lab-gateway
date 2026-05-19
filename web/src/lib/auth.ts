import { demoUsers } from "./mock-data";
import type { CurrentUser, Role } from "./types";

const storageKey = "clg.demoUser";

export function getStoredUser(): CurrentUser | null {
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
  window.localStorage.setItem(storageKey, JSON.stringify(user));
  return user;
}

export function clearStoredUser(): void {
  window.localStorage.removeItem(storageKey);
}
