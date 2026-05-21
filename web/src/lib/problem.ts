import { toast } from "sonner";
import type { Problem } from "./types";

const notImplementedProblem: Problem = {
  type: "about:blank",
  title: "Функция скоро будет",
  status: 501,
  detail: "Бэкенд-контракт уже подключён, реализация endpoint ещё в работе.",
  code: "ERR_NOT_IMPLEMENTED",
};

export class ApiProblem extends Error {
  readonly problem: Problem;
  readonly response?: Response;

  constructor(problem: Problem, response?: Response) {
    super(problem.detail ?? problem.title);
    this.name = "ApiProblem";
    this.problem = problem;
    this.response = response;
  }

  get code(): string {
    return this.problem.code ?? `HTTP_${this.problem.status}`;
  }

  get status(): number {
    return this.problem.status;
  }
}

export function problemFrom(error: unknown, response?: Response, fallback = "Ошибка API"): ApiProblem {
  if (error instanceof ApiProblem) return error;
  if (isProblem(error)) return new ApiProblem(error, response);
  if (response?.status === 501) return new ApiProblem(notImplementedProblem, response);
  if (error instanceof Error) {
    return new ApiProblem(
      {
        type: "about:blank",
        title: fallback,
        status: response?.status ?? 500,
        detail: error.message,
        code: response ? `HTTP_${response.status}` : "ERR_CLIENT",
      },
      response,
    );
  }
  return new ApiProblem(
    {
      type: "about:blank",
      title: fallback,
      status: response?.status ?? 500,
      detail: response ? `HTTP ${response.status}` : "Не удалось выполнить запрос.",
      code: response ? `HTTP_${response.status}` : "ERR_CLIENT",
    },
    response,
  );
}

export function problemMessage(error: unknown): string {
  const problem = problemFrom(error).problem;
  return problem.detail ?? problem.title;
}

export function toastProblem(error: unknown, fallback = "Ошибка API"): ApiProblem {
  const problem = problemFrom(error, undefined, fallback);
  toast.error(problem.problem.title, {
    description: problem.problem.detail ?? problem.code,
  });
  return problem;
}

export function isProblemCode(error: unknown, code: string): boolean {
  return problemFrom(error).code === code;
}

export function isNotImplemented(error: unknown): boolean {
  const problem = problemFrom(error);
  return problem.status === 501 || problem.code === "ERR_NOT_IMPLEMENTED";
}

function isProblem(value: unknown): value is Problem {
  if (!value || typeof value !== "object") return false;
  const candidate = value as Partial<Problem>;
  return typeof candidate.type === "string" && typeof candidate.title === "string" && typeof candidate.status === "number";
}
