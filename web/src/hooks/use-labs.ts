import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { api } from "@/lib/api";
import { problemFrom, toastProblem } from "@/lib/problem";
import type { CheckRun, LabInstance, LabInstanceDetail, LabState } from "@/lib/types";

type LabsFilters = {
  courseId?: string;
  states?: LabState[];
};

type CreateLabInput = {
  courseId: string;
  labTemplateId: string;
};

type FreezeLabInput = {
  id: string;
  reason: string;
  freezeForSeconds?: number;
};

type ExtendLabInput = {
  id: string;
  extendBySeconds: number;
};

type RunCheckInput = {
  id: string;
  checkTemplateId: string;
};

export const labsKeys = {
  all: ["labs"] as const,
  list: (filters?: LabsFilters) => ["labs", "list", filters?.courseId ?? "all", filters?.states?.join(",") ?? "all"] as const,
  detail: (id: string) => ["labs", "detail", id] as const,
  checks: (id: string) => ["labs", "checks", id] as const,
};

export function useLabs(filters?: LabsFilters) {
  return useQuery({
    queryKey: labsKeys.list(filters),
    queryFn: async (): Promise<LabInstance[]> => {
      const { data, error, response } = await api.GET("/labs", {
        params: {
          query: {
            course_id: filters?.courseId,
            state: filters?.states,
          },
        },
      });
      if (error) throw problemFrom(error, response, "Не удалось загрузить стенды");
      return data ?? [];
    },
  });
}

export function useLab(id?: string) {
  return useQuery({
    queryKey: id ? labsKeys.detail(id) : ["labs", "detail", "none"],
    enabled: Boolean(id),
    queryFn: async (): Promise<LabInstanceDetail> => {
      if (!id) throw problemFrom(undefined, undefined, "Не выбран стенд");
      const { data, error, response } = await api.GET("/labs/{id}", {
        params: { path: { id } },
      });
      if (error) throw problemFrom(error, response, "Не удалось загрузить детали стенда");
      if (!data) throw problemFrom(undefined, response, "Пустой ответ по стенду");
      return data;
    },
  });
}

export function useLabChecks(id?: string) {
  return useQuery({
    queryKey: id ? labsKeys.checks(id) : ["labs", "checks", "none"],
    enabled: Boolean(id),
    queryFn: async (): Promise<CheckRun[]> => {
      if (!id) throw problemFrom(undefined, undefined, "Не выбран стенд");
      const { data, error, response } = await api.GET("/labs/{id}/checks", {
        params: { path: { id } },
      });
      if (error) throw problemFrom(error, response, "Не удалось загрузить проверки");
      return data ?? [];
    },
  });
}

export function useCreateLab() {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: async (input: CreateLabInput): Promise<LabInstance> => {
      const { data, error, response } = await api.POST("/labs", {
        body: {
          course_id: input.courseId,
          lab_template_id: input.labTemplateId,
        },
      });
      if (error) throw problemFrom(error, response, "Не удалось запросить стенд");
      if (!data) throw problemFrom(undefined, response, "Пустой ответ создания стенда");
      return data;
    },
    onSuccess: (lab) => {
      queryClient.invalidateQueries({ queryKey: labsKeys.all });
      queryClient.setQueryData(labsKeys.detail(lab.id), lab);
    },
  });
}

export function useStopLab() {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: async (id: string): Promise<LabInstance> => {
      const { data, error, response } = await api.DELETE("/labs/{id}", {
        params: { path: { id } },
      });
      if (error) throw problemFrom(error, response, "Не удалось завершить стенд");
      if (!data) throw problemFrom(undefined, response, "Пустой ответ завершения стенда");
      return data;
    },
    onSuccess: (lab) => refreshLabQueries(queryClient, lab),
    onError: (error) => {
      toastProblem(error, "Не удалось завершить стенд");
    },
  });
}

export function useFreezeLab() {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: async (input: FreezeLabInput): Promise<LabInstance> => {
      const { data, error, response } = await api.POST("/labs/{id}/freeze", {
        params: { path: { id: input.id } },
        body: {
          reason: input.reason,
          freeze_for_seconds: input.freezeForSeconds,
        },
      });
      if (error) throw problemFrom(error, response, "Не удалось заморозить стенд");
      if (!data) throw problemFrom(undefined, response, "Пустой ответ заморозки стенда");
      return data;
    },
    onSuccess: (lab) => refreshLabQueries(queryClient, lab),
    onError: (error) => {
      toastProblem(error, "Не удалось заморозить стенд");
    },
  });
}

export function useUnfreezeLab() {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: async (id: string): Promise<LabInstance> => {
      const { data, error, response } = await api.POST("/labs/{id}/unfreeze", {
        params: { path: { id } },
      });
      if (error) throw problemFrom(error, response, "Не удалось разморозить стенд");
      if (!data) throw problemFrom(undefined, response, "Пустой ответ разморозки стенда");
      return data;
    },
    onSuccess: (lab) => refreshLabQueries(queryClient, lab),
    onError: (error) => {
      toastProblem(error, "Не удалось разморозить стенд");
    },
  });
}

export function useExtendLab() {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: async (input: ExtendLabInput): Promise<LabInstance> => {
      const { data, error, response } = await api.POST("/labs/{id}/extend", {
        params: { path: { id: input.id } },
        body: { extend_by_seconds: input.extendBySeconds },
      });
      if (error) throw problemFrom(error, response, "Не удалось продлить стенд");
      if (!data) throw problemFrom(undefined, response, "Пустой ответ продления стенда");
      return data;
    },
    onSuccess: (lab) => refreshLabQueries(queryClient, lab),
    onError: (error) => {
      toastProblem(error, "Не удалось продлить стенд");
    },
  });
}

export function useRunCheck() {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: async (input: RunCheckInput): Promise<CheckRun> => {
      const { data, error, response } = await api.POST("/labs/{id}/checks", {
        params: { path: { id: input.id } },
        body: { check_template_id: input.checkTemplateId },
      });
      if (error) throw problemFrom(error, response, "Не удалось запустить проверку");
      if (!data) throw problemFrom(undefined, response, "Пустой ответ запуска проверки");
      return data;
    },
    onSuccess: (run, input) => {
      queryClient.invalidateQueries({ queryKey: labsKeys.checks(input.id) });
      queryClient.invalidateQueries({ queryKey: labsKeys.detail(input.id) });
      queryClient.setQueryData(["checks", run.id ?? "latest"], run);
    },
    onError: (error) => {
      toastProblem(error, "Не удалось запустить проверку");
    },
  });
}

export function useDownloadSSHKey() {
  return useMutation({
    mutationFn: async (id: string): Promise<{ id: string; pem: string }> => {
      const { data, error, response } = await api.GET("/labs/{id}/ssh-key", {
        params: { path: { id } },
        parseAs: "text",
      });
      if (error) throw problemFrom(error, response, "Не удалось скачать SSH-ключ");
      return { id, pem: data ?? "" };
    },
    onSuccess: ({ id, pem }) => {
      downloadText(`lab-${id}-key.pem`, pem, "application/x-pem-file");
    },
    onError: (error) => {
      toastProblem(error, "Не удалось скачать SSH-ключ");
    },
  });
}

function refreshLabQueries(queryClient: ReturnType<typeof useQueryClient>, lab: LabInstance): void {
  queryClient.invalidateQueries({ queryKey: labsKeys.all });
  queryClient.setQueryData(labsKeys.detail(lab.id), lab);
}

function downloadText(filename: string, content: string, type: string): void {
  const blob = new Blob([content], { type });
  const url = URL.createObjectURL(blob);
  const anchor = document.createElement("a");
  anchor.href = url;
  anchor.download = filename;
  anchor.click();
  URL.revokeObjectURL(url);
}
