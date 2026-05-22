import { useMemo, useState } from "react";
import { CheckCircle2, Clock3, Copy, Download, FileWarning, Play, ShieldCheck, SquareTerminal, Trash2 } from "lucide-react";
import { differenceInMinutes, format, formatDistanceToNowStrict } from "date-fns";
import { ru } from "date-fns/locale";
import { toast } from "sonner";
import { ErrorBlock, LoadingBlock } from "@/components/async-state";
import { QuotaBar } from "@/components/quota-bar";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from "@/components/ui/card";
import { Progress } from "@/components/ui/progress";
import { demoCheckTemplateId, demoCourseId, demoLabTemplateId } from "@/lib/demo-config";
import { cn } from "@/lib/cn";
import { isProblemCode, problemFrom } from "@/lib/problem";
import type { CheckRun, LabDeployStep, LabInstance, LabInstanceDetail, LabState, QuotaSnapshot } from "@/lib/types";
import { useLabsStream } from "@/hooks/use-labs-stream";
import { useCreateLab, useDownloadSSHKey, useFreezeLab, useLab, useLabChecks, useLabs, useRunCheck, useStopLab } from "@/hooks/use-labs";
import { useQuota } from "@/hooks/use-admin";

const stateLabels: Record<LabState, string> = {
  pending_quota: "Проверка ресурсов",
  pending_project: "Выделение проекта",
  deploying: "Развертывание",
  ready: "Готов",
  checking: "Проверка",
  frozen: "Заморожен",
  failed: "Ошибка",
  cleaning: "Очистка",
  done: "Завершен",
  rejected: "Отклонен",
};

const stepLabels: Record<NonNullable<LabDeployStep["step_name"]>, string> = {
  allocate_project: "Quota",
  create_keypair: "Keypair",
  provision_network: "Network",
  boot_vm: "Boot",
  wait_ssh: "SSH",
  initial_check: "Check",
};

const fallbackSteps: NonNullable<LabDeployStep["step_name"]>[] = ["allocate_project", "create_keypair", "provision_network", "boot_vm", "wait_ssh", "initial_check"];

export function StudentPage(): JSX.Element {
  useLabsStream();
  const [inlineMessage, setInlineMessage] = useState<string | null>(null);
  const labsQuery = useLabs();
  const lab = useMemo(() => labsQuery.data?.find((item) => !["done", "rejected"].includes(item.state)) ?? null, [labsQuery.data]);
  const detailQuery = useLab(lab?.id);
  const checksQuery = useLabChecks(lab?.id);
  const quotaQuery = useQuota();
  const createLab = useCreateLab();
  const stopLab = useStopLab();
  const freezeLab = useFreezeLab();
  const runCheck = useRunCheck();
  const downloadSSHKey = useDownloadSSHKey();

  async function requestLab(): Promise<void> {
    setInlineMessage(null);
    const courseId = demoCourseId();
    const labTemplateId = demoLabTemplateId();
    if (!courseId || !labTemplateId) {
      setInlineMessage("Задайте VITE_DEMO_COURSE_ID и VITE_DEMO_LAB_TEMPLATE_ID для demo-запуска стенда.");
      return;
    }
    try {
      await createLab.mutateAsync({ courseId, labTemplateId });
      toast.success("Стенд запрошен");
    } catch (error) {
      if (isProblemCode(error, "ERR_QUOTA_EXCEEDED")) {
        setInlineMessage("Кластер сейчас выше безопасного порога. Попробуйте позже или обратитесь к преподавателю.");
      } else if (isProblemCode(error, "ERR_POOL_EMPTY")) {
        setInlineMessage("В пуле курса нет свободных проектов. Преподавателю или админу нужно пополнить пул.");
      } else if (isProblemCode(error, "ERR_LAB_ALREADY_ACTIVE")) {
        setInlineMessage("У вас уже есть активный стенд в этом курсе. Обновляем список...");
      } else {
        const problem = problemFrom(error, undefined, "Не удалось запросить стенд");
        setInlineMessage(problem.problem.detail ?? problem.problem.title);
      }
    }
  }

  async function reportProblem(activeLab: LabInstance): Promise<void> {
    const reason = window.prompt("Опишите проблему со стендом", activeLab.state_reason ?? "Не могу продолжить лабораторную работу");
    if (!reason) return;
    await freezeLab.mutateAsync({ id: activeLab.id, reason });
    toast.success("Стенд заморожен для разбора");
  }

  async function startCheck(activeLab: LabInstance): Promise<void> {
    const checkTemplateId = demoCheckTemplateId();
    if (!checkTemplateId) {
      setInlineMessage("Задайте VITE_DEMO_CHECK_TEMPLATE_ID, чтобы запускать проверки из UI.");
      return;
    }
    await runCheck.mutateAsync({ id: activeLab.id, checkTemplateId });
    toast.success("Проверка поставлена в очередь");
  }

  if (labsQuery.isLoading) return <LoadingBlock label="Ищем ваш активный стенд..." />;
  if (labsQuery.error) return <ErrorBlock error={labsQuery.error} onRetry={() => void labsQuery.refetch()} />;

  if (!lab) {
    return (
      <div className="mx-auto max-w-4xl space-y-6">
        <Card className="border-primary/30 bg-primary/10">
          <CardHeader>
            <CardTitle>Стенд ещё не запущен</CardTitle>
            <CardDescription>Запрос создаст LabInstance, проверит quota guard и поставит deploy saga в очередь.</CardDescription>
          </CardHeader>
          <CardContent className="space-y-4">
            {inlineMessage ? <div className="rounded-lg border border-amber-400/30 bg-amber-400/10 p-3 text-sm text-amber-100">{inlineMessage}</div> : null}
            <Button size="lg" onClick={() => void requestLab()} disabled={createLab.isPending}>
              {createLab.isPending ? "Запрашиваем..." : "Запросить стенд"}
            </Button>
          </CardContent>
        </Card>
      </div>
    );
  }

  const detail: LabInstanceDetail = detailQuery.data ?? lab;
  const checks = checksQuery.data ?? compactCheck(detail.latest_check_run);
  const cleanupLabel = detail.cleanup_at ? formatDistanceToNowStrict(new Date(detail.cleanup_at), { locale: ru }) : "Таймер не активен";

  return (
    <div className="mx-auto max-w-7xl space-y-6">
      <section className="flex flex-col gap-4 sm:flex-row sm:items-start sm:justify-between">
        <div>
          <div className="mb-3 flex flex-wrap items-center gap-2">
            <Badge variant={stateBadgeVariant(detail.state)}>{stateLabels[detail.state]}</Badge>
            <span className="text-sm text-muted-foreground">Стенд выделен {format(new Date(detail.created_at), "HH:mm", { locale: ru })}</span>
          </div>
          <h1 className="max-w-4xl text-2xl font-semibold tracking-normal sm:text-3xl">Лабораторный стенд {shortId(detail.id)}</h1>
          <p className="mt-2 text-sm text-muted-foreground">
            course {shortId(detail.course_id)} • {detail.state_reason ?? "состояние обновляется через API/SSE"}
          </p>
        </div>
        <Button variant="outline" onClick={() => void reportProblem(detail)} disabled={freezeLab.isPending}>
          <FileWarning className="h-4 w-4" />
          Сообщить о проблеме
        </Button>
      </section>

      {inlineMessage ? <div className="rounded-lg border border-amber-400/30 bg-amber-400/10 p-3 text-sm text-amber-100">{inlineMessage}</div> : null}
      {detailQuery.error ? <ErrorBlock error={detailQuery.error} onRetry={() => void detailQuery.refetch()} /> : null}

      <Card>
        <CardHeader>
          <CardTitle>Жизненный цикл стенда</CardTitle>
          <CardDescription>Шаги берутся из deploy saga, а не из локальной эвристики.</CardDescription>
        </CardHeader>
        <CardContent>
          <LabProgressStepper state={detail.state} steps={detail.deploy_steps ?? []} />
        </CardContent>
      </Card>

      <div className="grid gap-6 lg:grid-cols-[1.15fr_0.85fr]">
        <AccessCard lab={detail} onCopy={() => void copyCommand(detail)} onDownload={() => void downloadSSHKey.mutateAsync(detail.id)} downloading={downloadSSHKey.isPending} />
        <TtlCard lab={detail} cleanupLabel={cleanupLabel} onStop={() => void stopLab.mutateAsync(detail.id)} stopping={stopLab.isPending} />
      </div>

      <ChecksCard checks={checks} loading={checksQuery.isFetching} onRun={() => void startCheck(detail)} running={runCheck.isPending} />
    </div>
  );
}

function stateBadgeVariant(state: LabState): "success" | "warning" | "danger" | "violet" | "secondary" {
  if (state === "ready") return "success";
  if (state === "checking" || state === "deploying") return "warning";
  if (state === "frozen") return "violet";
  if (state === "failed" || state === "rejected") return "danger";
  return "secondary";
}

function ttlProgress(lab: LabInstance): number {
  if (!lab.cleanup_at) return 0;
  const created = new Date(lab.created_at);
  const cleanup = new Date(lab.cleanup_at);
  const total = Math.max(1, differenceInMinutes(cleanup, created));
  const elapsed = Math.max(0, differenceInMinutes(new Date(), created));
  return Math.min(100, Math.round((elapsed / total) * 100));
}

function LabProgressStepper({ state, steps }: { state: LabState; steps: LabDeployStep[] }): JSX.Element {
  const effectiveSteps = steps.length > 0 ? steps : fallbackSteps.map((stepName) => fallbackStep(stepName, state));

  return (
    <div className="grid gap-4 sm:grid-cols-3 xl:grid-cols-6">
      {effectiveSteps.map((step, index) => {
        const status = step.status ?? "pending";
        const isDone = status === "succeeded";
        const isCurrent = status === "in_progress";
        const stepName = step.step_name ?? fallbackSteps[index] ?? "allocate_project";
        return (
          <div key={`${stepName}-${index}`} className="relative rounded-lg border border-border bg-background p-4">
            <div
              className={cn(
                "mb-3 flex h-9 w-9 items-center justify-center rounded-full border",
                isDone && "border-emerald-400/40 bg-emerald-400/15 text-emerald-300",
                isCurrent && "border-amber-400/40 bg-amber-400/15 text-amber-300",
                status === "failed" && "border-red-400/40 bg-red-400/15 text-red-300",
                !isDone && !isCurrent && status !== "failed" && "border-border bg-muted text-muted-foreground",
              )}
            >
              {isDone ? <CheckCircle2 className="h-4 w-4" /> : index + 1}
            </div>
            <div className="text-sm font-medium">{stepLabels[stepName]}</div>
            <div className="mt-1 text-xs text-muted-foreground">{statusLabel(status)}</div>
          </div>
        );
      })}
    </div>
  );
}

function fallbackStep(stepName: NonNullable<LabDeployStep["step_name"]>, state: LabState): LabDeployStep {
  const doneCount = state === "ready" || state === "checking" || state === "frozen" ? fallbackSteps.length : state === "deploying" ? 3 : state === "pending_project" ? 1 : 0;
  const index = fallbackSteps.indexOf(stepName);
  return { step_name: stepName, status: index < doneCount ? "succeeded" : index === doneCount && state === "deploying" ? "in_progress" : "pending" };
}

function statusLabel(status: LabDeployStep["status"]): string {
  switch (status) {
    case "succeeded":
      return "Готово";
    case "in_progress":
      return "В процессе";
    case "failed":
      return "Ошибка";
    case "compensated":
      return "Компенсирован";
    default:
      return "Ожидает";
  }
}

function AccessCard({
  lab,
  onCopy,
  onDownload,
  downloading,
}: {
  lab: LabInstanceDetail;
  onCopy: () => void;
  onDownload: () => void;
  downloading: boolean;
}): JSX.Element {
  const ip = lab.ki_resources?.floating_ips?.[0];
  const command = sshCommand(lab);
  return (
    <Card>
      <CardHeader>
        <CardTitle>Доступ к виртуальной машине</CardTitle>
        <CardDescription>Ключ скачивается отдельным endpoint и не хранится во фронте.</CardDescription>
      </CardHeader>
      <CardContent className="space-y-4">
        <div className="grid gap-3 sm:grid-cols-2">
          <InfoItem label="IP-адрес" value={ip ?? "Ожидает"} />
          <InfoItem label="Network" value={lab.ki_resources?.network_id ?? "Ожидает"} />
        </div>
        <div className="rounded-lg border border-border bg-background p-3 font-mono text-sm text-cyan-100">{command ?? "SSH-команда появится после назначения floating IP"}</div>
        <div className="flex flex-col gap-2 sm:flex-row">
          <Button onClick={onDownload} disabled={downloading}>
            <Download className="h-4 w-4" />
            {downloading ? "Скачиваем..." : "Скачать SSH-ключ"}
          </Button>
          <Button variant="secondary" onClick={onCopy} disabled={!command}>
            <Copy className="h-4 w-4" />
            Скопировать команду
          </Button>
        </div>
        <div className="rounded-lg border border-cyan-400/20 bg-cyan-400/10 p-3 text-sm text-cyan-100">
          <SquareTerminal className="mr-2 inline h-4 w-4" />
          После скачивания выполните `chmod 600 lab-key.pem`, затем подключайтесь командой выше.
        </div>
      </CardContent>
    </Card>
  );
}

function TtlCard({ lab, cleanupLabel, onStop, stopping }: { lab: LabInstance; cleanupLabel: string; onStop: () => void; stopping: boolean }): JSX.Element {
  const progress = ttlProgress(lab);
  return (
    <Card>
      <CardHeader>
        <CardTitle>Время жизни</CardTitle>
        <CardDescription>Автоочистка освобождает проект обратно в пул курса.</CardDescription>
      </CardHeader>
      <CardContent className="space-y-5">
        <div>
          <div className="mb-2 flex items-center justify-between text-sm">
            <span className="text-muted-foreground">Удаление через</span>
            <span className="font-medium">{cleanupLabel}</span>
          </div>
          <Progress value={progress} />
          <div className="mt-2 text-xs text-muted-foreground">{progress}% времени использовано</div>
        </div>
        <div className="rounded-lg border border-border bg-background p-3 text-sm">
          <Clock3 className="mr-2 inline h-4 w-4 text-primary" />
          {lab.cleanup_at ? `Точное время очистки: ${format(new Date(lab.cleanup_at), "HH:mm", { locale: ru })}` : "Очистка не запланирована"}
        </div>
        <Button variant="destructive" className="w-full" onClick={onStop} disabled={stopping}>
          <Trash2 className="h-4 w-4" />
          {stopping ? "Завершаем..." : "Завершить стенд"}
        </Button>
      </CardContent>
    </Card>
  );
}

function ChecksCard({ checks, loading, onRun, running }: { checks: CheckRun[]; loading: boolean; onRun: () => void; running: boolean }): JSX.Element {
  return (
    <Card>
      <CardHeader className="flex-row items-start justify-between gap-4">
        <div>
          <CardTitle>Проверки</CardTitle>
          <CardDescription>Безагентные SSH-проверки конфигурации.</CardDescription>
        </div>
        <Button onClick={onRun} disabled={running}>
          <Play className="h-4 w-4" />
          {running ? "Запускаем..." : "Запустить проверку"}
        </Button>
      </CardHeader>
      <CardContent className="space-y-3">
        {loading ? <div className="text-sm text-muted-foreground">Обновляем список проверок...</div> : null}
        {checks.length === 0 ? <div className="rounded-lg border border-border bg-background p-3 text-sm text-muted-foreground">Проверок пока нет.</div> : null}
        {checks.map((run, index) => (
          <div key={run.id ?? `${run.state}-${index}`} className="flex items-center justify-between rounded-lg border border-border bg-background p-3">
            <div>
              <div className="font-mono text-sm">{run.check_template_id ? shortId(run.check_template_id) : "check"}</div>
              <div className="text-xs text-muted-foreground">{run.finished_at ? format(new Date(run.finished_at), "HH:mm", { locale: ru }) : "В процессе"}</div>
            </div>
            <Badge variant={run.state === "passed" ? "success" : run.state === "failed" || run.state === "errored" ? "danger" : "warning"}>{run.state ?? "queued"}</Badge>
          </div>
        ))}
      </CardContent>
    </Card>
  );
}

function QuotaCard({ quota, error, refetch }: { quota?: QuotaSnapshot; error: unknown; refetch: () => void }): JSX.Element {
  if (error) return <ErrorBlock error={error} onRetry={refetch} />;
  const threshold = quota?.threshold_pct ?? 90;
  const max = quota?.utilization_pct?.max ?? 0;
  return (
    <Card>
      <CardHeader>
        <CardTitle>Утилизация кластера</CardTitle>
        <CardDescription>Запуск блокируется при прогнозе выше {threshold}%.</CardDescription>
      </CardHeader>
      <CardContent className="space-y-4">
        <QuotaBar label="CPU" value={quota?.utilization_pct?.vcpus} threshold={threshold} />
        <QuotaBar label="RAM" value={quota?.utilization_pct?.ram} threshold={threshold} />
        <QuotaBar label="Disk" value={quota?.utilization_pct?.disk} threshold={threshold} />
        <div className={cn("rounded-lg border p-3 text-sm", max >= threshold ? "border-red-400/20 bg-red-400/10 text-red-100" : "border-emerald-400/20 bg-emerald-400/10 text-emerald-100")}>
          <ShieldCheck className="mr-2 inline h-4 w-4" />
          {max >= threshold ? "Ресурсы выше порога, новые стенды будут заблокированы." : "Ресурсы ниже порога, новые стенды разрешены."}
        </div>
      </CardContent>
    </Card>
  );
}

function InfoItem({ label, value }: { label: string; value: string }): JSX.Element {
  return (
    <div className="rounded-lg border border-border bg-background p-3">
      <div className="text-xs uppercase text-muted-foreground">{label}</div>
      <div className="mt-1 font-mono text-sm">{value}</div>
    </div>
  );
}

async function copyCommand(lab: LabInstanceDetail): Promise<void> {
  const command = sshCommand(lab);
  if (!command) return;
  await navigator.clipboard.writeText(command);
  toast.success("SSH-команда скопирована");
}

function sshCommand(lab: LabInstanceDetail): string | null {
  const ip = lab.ki_resources?.floating_ips?.[0];
  return ip ? `ssh -i lab-${lab.id}-key.pem ubuntu@${ip}` : null;
}

function compactCheck(check?: CheckRun): CheckRun[] {
  return check ? [check] : [];
}

function shortId(id?: string): string {
  if (!id) return "-";
  return id.length > 8 ? id.slice(0, 8) : id;
}
