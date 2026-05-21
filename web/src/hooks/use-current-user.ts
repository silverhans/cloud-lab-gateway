import { useQuery } from "@tanstack/react-query";
import { api } from "@/lib/api";
import { getStoredUser } from "@/lib/auth";
import { problemFrom } from "@/lib/problem";
import type { CurrentUser } from "@/lib/types";

async function fetchCurrentUser(): Promise<CurrentUser | null> {
  const { data, error, response } = await api.GET("/auth/me");
  if (response.status === 401) {
    return getStoredUser();
  }
  if (error) throw problemFrom(error, response, "Не удалось проверить сессию");
  return data ?? null;
}

export function useCurrentUser() {
  return useQuery({
    queryKey: ["auth", "me"],
    queryFn: fetchCurrentUser,
  });
}
