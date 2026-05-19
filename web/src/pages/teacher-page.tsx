import { Clock3, FileWarning, PauseCircle, PlayCircle, Settings, TimerReset } from "lucide-react";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from "@/components/ui/card";
import { Progress } from "@/components/ui/progress";
import { Table, TableBody, TableCell, TableHead, TableHeader, TableRow } from "@/components/ui/table";
import { teacherLabs, type TeacherLabRow } from "@/lib/mock-data";
import type { LabState } from "@/lib/types";

const stateLabels: Record<LabState, string> = {
  pending_quota: "Quota",
  pending_project: "Pool",
  deploying: "Deploying",
  ready: "Ready",
  checking: "Checking",
  frozen: "Frozen",
  failed: "Failed",
  cleaning: "Cleaning",
  done: "Done",
  rejected: "Rejected",
};

const stateVariants: Record<LabState, "success" | "warning" | "danger" | "violet" | "secondary"> = {
  pending_quota: "secondary",
  pending_project: "secondary",
  deploying: "warning",
  ready: "success",
  checking: "warning",
  frozen: "violet",
  failed: "danger",
  cleaning: "secondary",
  done: "secondary",
  rejected: "danger",
};

export function TeacherPage(): JSX.Element {
  const active = teacherLabs.filter((lab) => !["done", "rejected"].includes(lab.state)).length;
  const frozen = teacherLabs.filter((lab) => lab.state === "frozen").length;
  const failedChecks = teacherLabs.filter((lab) => lab.lastCheck === "failed").length;

  return (
    <div className="mx-auto max-w-7xl space-y-6">
      <section className="flex flex-col gap-4 sm:flex-row sm:items-start sm:justify-between">
        <div>
          <h1 className="text-2xl font-semibold tracking-normal sm:text-3xl">Лабы курса</h1>
          <p className="mt-2 text-sm text-muted-foreground">Linux основы • активных стендов {active} / 30</p>
        </div>
        <Button variant="secondary">
          <Settings className="h-4 w-4" />
          Настройки таймаутов
        </Button>
      </section>

      <div className="grid gap-4 md:grid-cols-3">
        <SummaryTile label="Активные стенды" value={`${active}/30`} tone="primary" />
        <SummaryTile label="Заморожены" value={`${frozen}`} tone="violet" />
        <SummaryTile label="Проверки с ошибками" value={`${failedChecks}`} tone="danger" />
      </div>

      <div className="grid gap-6 xl:grid-cols-[1fr_360px]">
        <Card>
          <CardHeader>
            <CardTitle>Студенческие стенды</CardTitle>
            <CardDescription>Быстрый обзор статусов и оставшегося времени.</CardDescription>
          </CardHeader>
          <CardContent>
            <Table>
              <TableHeader>
                <TableRow>
                  <TableHead>Студент</TableHead>
                  <TableHead>Состояние</TableHead>
                  <TableHead>Запуск</TableHead>
                  <TableHead>Удаление</TableHead>
                  <TableHead className="text-right">Действия</TableHead>
                </TableRow>
              </TableHeader>
              <TableBody>
                {teacherLabs.map((row) => (
                  <TableRow key={row.id}>
                    <TableCell>
                      <div className="font-medium">{row.student}</div>
                      <div className="text-xs text-muted-foreground">{row.email}</div>
                    </TableCell>
                    <TableCell>
                      <Badge variant={stateVariants[row.state]}>{stateLabels[row.state]}</Badge>
                    </TableCell>
                    <TableCell>{row.startedAt}</TableCell>
                    <TableCell>{row.state === "frozen" ? row.frozenUntil : row.cleanupAt ?? "-"}</TableCell>
                    <TableCell className="text-right">
                      <LabActions lab={row} />
                    </TableCell>
                  </TableRow>
                ))}
              </TableBody>
            </Table>
          </CardContent>
        </Card>

        <Card>
          <CardHeader>
            <CardTitle>Настройки лимитов</CardTitle>
            <CardDescription>Применяются к новым стендам курса.</CardDescription>
          </CardHeader>
          <CardContent className="space-y-4">
            <TimeoutControl label="Время жизни стенда" value="2ч 00м" progress={42} />
            <TimeoutControl label="Заморозка инцидента" value="24ч" progress={58} />
            <div className="rounded-lg border border-cyan-400/20 bg-cyan-400/10 p-3 text-sm text-cyan-100">
              <Clock3 className="mr-2 inline h-4 w-4" />
              Новые стенды будут автоудаляться через 2 часа. Уже созданные стенды сохраняют текущий таймер.
            </div>
            <Button className="w-full">
              <Settings className="h-4 w-4" />
              Применить к новым стендам
            </Button>
          </CardContent>
        </Card>
      </div>

      <Card>
        <CardHeader>
          <CardTitle>Инциденты и поддержка</CardTitle>
          <CardDescription>Стенды в заморозке остаются доступными для разбора преподавателем.</CardDescription>
        </CardHeader>
        <CardContent className="grid gap-4 md:grid-cols-2">
          {teacherLabs
            .filter((lab) => lab.state === "frozen" || lab.lastCheck === "failed")
            .map((lab) => (
              <div key={`${lab.id}-incident`} className="rounded-lg border border-border bg-background p-4">
                <div className="mb-3 flex items-start justify-between gap-3">
                  <div>
                    <div className="font-medium">{lab.student}</div>
                    <div className="text-xs text-muted-foreground">{lab.project}</div>
                  </div>
                  <Badge variant={lab.state === "frozen" ? "violet" : "danger"}>
                    {lab.state === "frozen" ? "заморожен" : "ошибка"}
                  </Badge>
                </div>
                <div className="text-sm text-muted-foreground">
                  {lab.state === "frozen"
                    ? `Удаление приостановлено до: ${lab.frozenUntil}`
                    : "Последняя проверка завершилась с ошибками, требуется консультация."}
                </div>
              </div>
            ))}
        </CardContent>
      </Card>
    </div>
  );
}

function SummaryTile({ label, value, tone }: { label: string; value: string; tone: "primary" | "violet" | "danger" }): JSX.Element {
  const toneClass =
    tone === "primary"
      ? "border-primary/25 bg-primary/10 text-primary"
      : tone === "violet"
        ? "border-violet-400/25 bg-violet-400/10 text-violet-200"
        : "border-red-400/25 bg-red-400/10 text-red-200";

  return (
    <div className={`rounded-lg border p-4 ${toneClass}`}>
      <div className="text-xs uppercase text-muted-foreground">{label}</div>
      <div className="mt-2 text-2xl font-semibold">{value}</div>
    </div>
  );
}

function TimeoutControl({ label, value, progress }: { label: string; value: string; progress: number }): JSX.Element {
  return (
    <div className="rounded-lg border border-border bg-background p-4">
      <div className="mb-3 flex items-center justify-between gap-3">
        <div className="text-sm text-muted-foreground">{label}</div>
        <div className="font-medium">{value}</div>
      </div>
      <Progress value={progress} />
      <div className="mt-3 flex items-center justify-between text-xs text-muted-foreground">
        <span>30м</span>
        <span>2ч</span>
        <span>8ч</span>
      </div>
    </div>
  );
}

function LabActions({ lab }: { lab: TeacherLabRow }): JSX.Element {
  if (lab.state === "frozen") {
    return (
      <Button variant="secondary" size="sm">
        <PlayCircle className="h-4 w-4" />
        Разморозить
      </Button>
    );
  }

  if (lab.state === "ready" || lab.state === "checking") {
    return (
      <div className="flex justify-end gap-2">
        <Button variant="ghost" size="sm">
          <TimerReset className="h-4 w-4" />
          +30м
        </Button>
        <Button variant="ghost" size="sm">
          <PauseCircle className="h-4 w-4" />
          Freeze
        </Button>
      </div>
    );
  }

  return (
    <Button variant="ghost" size="sm">
      <FileWarning className="h-4 w-4" />
      Открыть
    </Button>
  );
}
