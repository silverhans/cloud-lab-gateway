import { useMemo } from "react";
import {
  CheckCircle2,
  Clock3,
  Copy,
  Download,
  FileWarning,
  Play,
  ShieldCheck,
  SquareTerminal,
  Trash2,
} from "lucide-react";
import { format, formatDistanceToNowStrict, differenceInMinutes } from "date-fns";
import { ru } from "date-fns/locale";
import { toast } from "sonner";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from "@/components/ui/card";
import { Progress } from "@/components/ui/progress";
import { demoLab } from "@/lib/mock-data";
import type { LabInstance, LabState } from "@/lib/types";
import { cn } from "@/lib/cn";
import { useLabsStream } from "@/hooks/use-labs-stream";

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

const stepStates = ["Quota", "Pool", "Boot", "SSH", "Check"] as const;

function completedSteps(state: LabState): number {
  switch (state) {
    case "pending_quota":
      return 0;
    case "pending_project":
      return 1;
    case "deploying":
      return 3;
    case "ready":
    case "checking":
    case "frozen":
      return 5;
    case "failed":
    case "cleaning":
    case "done":
    case "rejected":
      return 4;
  }
}

function stateBadgeVariant(state: LabState): "success" | "warning" | "danger" | "violet" | "secondary" {
  if (state === "ready") return "success";
  if (state === "checking" || state === "deploying") return "warning";
  if (state === "frozen") return "violet";
  if (state === "failed" || state === "rejected") return "danger";
  return "secondary";
}

function ttlProgress(lab: LabInstance): number {
  if (!lab.cleanupAt) return 0;
  const created = new Date(lab.createdAt);
  const cleanup = new Date(lab.cleanupAt);
  const total = Math.max(1, differenceInMinutes(cleanup, created));
  const elapsed = Math.max(0, differenceInMinutes(new Date(), created));
  return Math.min(100, Math.round((elapsed / total) * 100));
}

export function StudentPage(): JSX.Element {
  useLabsStream();
  const lab = demoLab;
  const cleanupLabel = useMemo(() => {
    if (!lab.cleanupAt) return "Таймер не активен";
    return formatDistanceToNowStrict(new Date(lab.cleanupAt), { locale: ru });
  }, [lab.cleanupAt]);

  async function copyCommand(): Promise<void> {
    if (!lab.sshCommand) return;
    await navigator.clipboard.writeText(lab.sshCommand);
    toast.success("SSH-команда скопирована");
  }

  return (
    <div className="mx-auto max-w-7xl space-y-6">
      <section className="flex flex-col gap-4 sm:flex-row sm:items-start sm:justify-between">
        <div>
          <div className="mb-3 flex flex-wrap items-center gap-2">
            <Badge variant={stateBadgeVariant(lab.state)}>{stateLabels[lab.state]}</Badge>
            <span className="text-sm text-muted-foreground">
              Стенд выделен {format(new Date(lab.createdAt), "HH:mm", { locale: ru })}
            </span>
          </div>
          <h1 className="max-w-4xl text-2xl font-semibold tracking-normal sm:text-3xl">{lab.title}</h1>
          <p className="mt-2 text-sm text-muted-foreground">{lab.courseName} • {lab.stateReason}</p>
        </div>
        <Button variant="outline">
          <FileWarning className="h-4 w-4" />
          Сообщить о проблеме
        </Button>
      </section>

      <Card>
        <CardHeader>
          <CardTitle>Жизненный цикл стенда</CardTitle>
          <CardDescription>Каждый шаг соответствует состоянию state machine на бэкенде.</CardDescription>
        </CardHeader>
        <CardContent>
          <LabProgressStepper state={lab.state} />
        </CardContent>
      </Card>

      <div className="grid gap-6 lg:grid-cols-[1.15fr_0.85fr]">
        <AccessCard lab={lab} onCopy={() => void copyCommand()} />
        <TtlCard lab={lab} cleanupLabel={cleanupLabel} />
      </div>

      <div className="grid gap-6 xl:grid-cols-[1fr_360px]">
        <ChecksCard lab={lab} />
        <QuotaCard lab={lab} />
      </div>
    </div>
  );
}

function LabProgressStepper({ state }: { state: LabState }): JSX.Element {
  const done = completedSteps(state);

  return (
    <div className="grid gap-4 sm:grid-cols-5">
      {stepStates.map((step, index) => {
        const isDone = index < done;
        const isCurrent = index === done && !["ready", "checking", "frozen"].includes(state);
        return (
          <div key={step} className="relative rounded-lg border border-border bg-background p-4">
            <div
              className={cn(
                "mb-3 flex h-9 w-9 items-center justify-center rounded-full border",
                isDone && "border-emerald-400/40 bg-emerald-400/15 text-emerald-300",
                isCurrent && "border-amber-400/40 bg-amber-400/15 text-amber-300",
                !isDone && !isCurrent && "border-border bg-muted text-muted-foreground",
              )}
            >
              {isDone ? <CheckCircle2 className="h-4 w-4" /> : index + 1}
            </div>
            <div className="text-sm font-medium">{step}</div>
            <div className="mt-1 text-xs text-muted-foreground">
              {isDone ? "Готово" : isCurrent ? "В процессе" : "Ожидает"}
            </div>
          </div>
        );
      })}
    </div>
  );
}

function AccessCard({ lab, onCopy }: { lab: LabInstance; onCopy: () => void }): JSX.Element {
  return (
    <Card>
      <CardHeader>
        <CardTitle>Доступ к виртуальной машине</CardTitle>
        <CardDescription>Ключ генерируется в КИ и доступен только владельцу стенда.</CardDescription>
      </CardHeader>
      <CardContent className="space-y-4">
        <div className="grid gap-3 sm:grid-cols-2">
          <InfoItem label="IP-адрес" value={lab.publicIp ?? "Ожидает"} />
          <InfoItem label="Логин" value={lab.sshLogin ?? "Ожидает"} />
        </div>
        <div className="rounded-lg border border-border bg-background p-3 font-mono text-sm text-cyan-100">
          {lab.sshCommand}
        </div>
        <div className="flex flex-col gap-2 sm:flex-row">
          <Button>
            <Download className="h-4 w-4" />
            Скачать SSH-ключ
          </Button>
          <Button variant="secondary" onClick={onCopy}>
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

function TtlCard({ lab, cleanupLabel }: { lab: LabInstance; cleanupLabel: string }): JSX.Element {
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
          {lab.cleanupAt ? `Точное время очистки: ${format(new Date(lab.cleanupAt), "HH:mm", { locale: ru })}` : "Очистка не запланирована"}
        </div>
        <Button variant="destructive" className="w-full">
          <Trash2 className="h-4 w-4" />
          Завершить стенд
        </Button>
      </CardContent>
    </Card>
  );
}

function ChecksCard({ lab }: { lab: LabInstance }): JSX.Element {
  return (
    <Card>
      <CardHeader className="flex-row items-start justify-between gap-4">
        <div>
          <CardTitle>Проверки</CardTitle>
          <CardDescription>Безагентные SSH-проверки конфигурации.</CardDescription>
        </div>
        <Button>
          <Play className="h-4 w-4" />
          Запустить проверку
        </Button>
      </CardHeader>
      <CardContent className="space-y-3">
        {lab.checkRuns.map((run) => (
          <div key={run.id} className="flex items-center justify-between rounded-lg border border-border bg-background p-3">
            <div>
              <div className="font-mono text-sm">{run.title}</div>
              <div className="text-xs text-muted-foreground">
                {run.finishedAt ? format(new Date(run.finishedAt), "HH:mm", { locale: ru }) : "В процессе"}
              </div>
            </div>
            <Badge variant={run.state === "passed" ? "success" : run.state === "failed" ? "danger" : "warning"}>
              {run.state === "passed" ? "PASSED" : run.state === "failed" ? `${run.failedSteps ?? 1} ошибки` : "RUNNING"}
            </Badge>
          </div>
        ))}
      </CardContent>
    </Card>
  );
}

function QuotaCard({ lab }: { lab: LabInstance }): JSX.Element {
  const quota = lab.quota ?? { cpu: 0, ram: 0, disk: 0 };
  return (
    <Card>
      <CardHeader>
        <CardTitle>Утилизация кластера</CardTitle>
        <CardDescription>Запуск блокируется при прогнозе выше 90%.</CardDescription>
      </CardHeader>
      <CardContent className="space-y-4">
        <QuotaBar label="CPU" value={quota.cpu} />
        <QuotaBar label="RAM" value={quota.ram} />
        <QuotaBar label="Disk" value={quota.disk} />
        <div className="rounded-lg border border-emerald-400/20 bg-emerald-400/10 p-3 text-sm text-emerald-100">
          <ShieldCheck className="mr-2 inline h-4 w-4" />
          Ресурсы ниже порога, новые стенды разрешены.
        </div>
      </CardContent>
    </Card>
  );
}

function QuotaBar({ label, value }: { label: string; value: number }): JSX.Element {
  const color = value >= 90 ? "bg-red-400" : value >= 80 ? "bg-orange-400" : value >= 60 ? "bg-amber-400" : "bg-emerald-400";
  return (
    <div>
      <div className="mb-2 flex items-center justify-between text-sm">
        <span className="text-muted-foreground">{label}</span>
        <span className="font-medium">{value}%</span>
      </div>
      <div className="h-2.5 overflow-hidden rounded-full bg-secondary">
        <div className={cn("h-full rounded-full", color)} style={{ width: `${value}%` }} />
      </div>
    </div>
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
