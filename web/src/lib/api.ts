import createClient from "openapi-fetch";

type FallbackPaths = Record<string, never>;

export const api = createClient<FallbackPaths>({
  baseUrl: "/api/v1",
  credentials: "include",
});

export async function apiFetch<T>(path: string, init?: RequestInit): Promise<T> {
  const response = await fetch(`/api/v1${path}`, {
    credentials: "include",
    headers: { "content-type": "application/json", ...init?.headers },
    ...init,
  });

  if (!response.ok) {
    throw new Error(`API request failed: ${response.status}`);
  }

  if (response.status === 204) {
    return undefined as T;
  }

  return (await response.json()) as T;
}
