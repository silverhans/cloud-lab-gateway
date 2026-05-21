import { useQuery } from "@tanstack/react-query";
import { getStoredUser } from "@/lib/auth";
import type { CurrentUser, Role } from "@/lib/types";

type ApiCurrentUser = {
  id: string;
  display_name: string;
  email?: string | null;
  role: string;
};

async function fetchCurrentUser(): Promise<CurrentUser | null> {
  try {
    const response = await fetch("/api/v1/auth/me", { credentials: "include" });
    if (response.status === 401) return getStoredUser();
    if (!response.ok) return null;
    const payload = (await response.json()) as ApiCurrentUser;
    if (!isRole(payload.role)) return null;
    return {
      id: payload.id,
      displayName: payload.display_name,
      email: payload.email ?? "",
      role: payload.role,
    };
  } catch {
    return getStoredUser();
  }
}

export function useCurrentUser() {
  return useQuery({
    queryKey: ["auth", "me"],
    queryFn: fetchCurrentUser,
  });
}

function isRole(value: string): value is Role {
  return value === "student" || value === "teacher" || value === "admin";
}
