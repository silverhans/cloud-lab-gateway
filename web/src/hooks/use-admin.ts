import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { api } from "@/lib/api";
import { problemFrom, toastProblem } from "@/lib/problem";
import type { AuditEvent, Project, ProjectState, QuotaSnapshot, Setting, SettingUpdate } from "@/lib/types";

type ProjectsFilters = {
  kiDomainId?: string;
  state?: ProjectState;
};

type AuditFilters = {
  kind?: string;
  actorUserId?: string;
  since?: string;
  limit?: number;
};

type SettingsFilters = {
  scope?: "global" | "per_course" | "per_lab_template";
  scopeId?: string;
};

type QuarantineInput = {
  id: string;
  reason: string;
};

export const adminKeys = {
  projects: (filters?: ProjectsFilters) => ["admin", "projects", filters?.kiDomainId ?? "all", filters?.state ?? "all"] as const,
  quota: ["admin", "quota"] as const,
  audit: (filters?: AuditFilters) => ["admin", "audit", filters?.kind ?? "all", filters?.actorUserId ?? "all", filters?.since ?? "all", filters?.limit ?? 100] as const,
  settings: (filters?: SettingsFilters) => ["admin", "settings", filters?.scope ?? "all", filters?.scopeId ?? "all"] as const,
};

export function useProjects(filters?: ProjectsFilters) {
  return useQuery({
    queryKey: adminKeys.projects(filters),
    queryFn: async (): Promise<Project[]> => {
      const { data, error, response } = await api.GET("/admin/projects", {
        params: {
          query: {
            ki_domain_id: filters?.kiDomainId,
            state: filters?.state,
          },
        },
      });
      if (error) throw problemFrom(error, response, "Не удалось загрузить пул проектов");
      return data ?? [];
    },
  });
}

export function useQuarantineProject() {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: async (input: QuarantineInput): Promise<void> => {
      const { error, response } = await api.POST("/admin/projects/{id}/quarantine", {
        params: { path: { id: input.id } },
        body: { reason: input.reason },
      });
      if (error) throw problemFrom(error, response, "Не удалось отправить проект в карантин");
    },
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ["admin", "projects"] });
    },
    onError: (error) => {
      toastProblem(error, "Не удалось отправить проект в карантин");
    },
  });
}

export function useReleaseProject() {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: async (id: string): Promise<void> => {
      const { error, response } = await api.POST("/admin/projects/{id}/release", {
        params: { path: { id } },
      });
      if (error) throw problemFrom(error, response, "Не удалось вернуть проект в пул");
    },
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ["admin", "projects"] });
    },
    onError: (error) => {
      toastProblem(error, "Не удалось вернуть проект в пул");
    },
  });
}

export function useSettings(filters?: SettingsFilters) {
  return useQuery({
    queryKey: adminKeys.settings(filters),
    queryFn: async (): Promise<Setting[]> => {
      const { data, error, response } = await api.GET("/admin/settings", {
        params: {
          query: {
            scope: filters?.scope,
            scope_id: filters?.scopeId,
          },
        },
      });
      if (error) throw problemFrom(error, response, "Не удалось загрузить настройки");
      return data ?? [];
    },
  });
}

export function usePutSetting() {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: async (input: SettingUpdate): Promise<Setting> => {
      const { data, error, response } = await api.PUT("/admin/settings", {
        body: input,
      });
      if (error) throw problemFrom(error, response, "Не удалось сохранить настройку");
      if (!data) throw problemFrom(undefined, response, "Пустой ответ настройки");
      return data;
    },
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ["admin", "settings"] });
    },
  });
}

export function useQuota() {
  return useQuery({
    queryKey: adminKeys.quota,
    queryFn: async (): Promise<QuotaSnapshot> => {
      const { data, error, response } = await api.GET("/admin/quota");
      if (error) throw problemFrom(error, response, "Не удалось загрузить квоты");
      if (!data) throw problemFrom(undefined, response, "Пустой ответ квот");
      return data;
    },
  });
}

export function useAudit(filters?: AuditFilters) {
  return useQuery({
    queryKey: adminKeys.audit(filters),
    queryFn: async (): Promise<AuditEvent[]> => {
      const { data, error, response } = await api.GET("/admin/audit", {
        params: {
          query: {
            kind: filters?.kind,
            actor_user_id: filters?.actorUserId,
            since: filters?.since,
            limit: filters?.limit ?? 100,
          },
        },
      });
      if (error) throw problemFrom(error, response, "Не удалось загрузить аудит");
      return data ?? [];
    },
  });
}
