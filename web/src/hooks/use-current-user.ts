import { useQuery } from "@tanstack/react-query";
import { getStoredUser } from "@/lib/auth";
import type { CurrentUser } from "@/lib/types";

async function fetchCurrentUser(): Promise<CurrentUser | null> {
  const stored = getStoredUser();
  if (stored) return stored;

  try {
    const response = await fetch("/api/v1/auth/me", { credentials: "include" });
    if (response.status === 401) return null;
    if (!response.ok) return null;
    return (await response.json()) as CurrentUser;
  } catch {
    return null;
  }
}

export function useCurrentUser() {
  return useQuery({
    queryKey: ["auth", "me"],
    queryFn: fetchCurrentUser,
  });
}
