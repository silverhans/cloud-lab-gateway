import { useState } from "react";
import { format } from "date-fns";
import { ru } from "date-fns/locale";
import { Clock3, FileWarning, PauseCircle, PlayCircle, Settings, TimerReset } from "lucide-react";
import { toast } from "sonner";
import { ErrorBlock, LoadingBlock } from "@/components/async-state";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from "@/components/ui/card";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { Progress } from "@/components/ui/progress";
import { Table, TableBody, TableCell, TableHead, TableHeader, TableRow } from "@/components/ui/table";
import { demoCourseId } from "@/lib/demo-config";
import { problemFrom } from "@/lib/problem";
import type { CheckRun, LabInstance, LabState, Setting, SettingUpdate } from "@/lib/types";
import { useCurrentUser } from "@/hooks/use-current-user";
import { useExtendLab, useFreezeLab, useLabChecks, useLabs, useUnfreezeLab } from "@/hooks/use-labs";
import { usePutSetting, useSettings } from "@/hooks/use-admin";

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
  const { data: user } = useCurrentUser();
  const courseId = user?.courses?.find((course) => course.role_in_course === "teacher")?.id ?? demoCourseId();
  const labsQuery = useLabs(courseId ? { courseId } : undefined);
  const settingsQuery = useSettings(courseId ? { scope: "per_course", scopeId: courseId } : { scope: "global" });
  const labs = labsQuery.data ?? [];
  const active = labs.filter((lab) => !["done", "rejected"].includes(lab.state)).length;
  const frozen = labs.filter((lab) => lab.state === "frozen").length;
  const failed = labs.filter((lab) => lab.state === "failed").length;

  if (labsQuery.isLoading) return <LoadingBlock label="Загружаем стенды курса..." />;

  return (
    <div className="mx-auto max-w-7xl space-y-6">
      <section className="flex flex-col gap-4 sm:flex-row sm:items-start sm:justify-between">
        <div>
          <h1 className="text-2xl font-semibold tracking-normal sm:text-3xl">Лабы курса</h1>
          <p className="mt-2 text-sm text-muted-foreground">course {courseId ? shortId(courseId) : "all"} • активных стендов {active}</p>
        </div>
        <Button variant="secondary" onClick={() => void settingsQuery.refetch()}>
          <Settings className="h-4 w-4" />
          Обновить настройки
        </Button>
      </section>

      {labsQuery.error ? <ErrorBlock error={labsQuery.error} onRetry={() => void labsQuery.refetch()} /> : null}

      <div className="grid gap-4 md:grid-cols-3">
        <SummaryTile label="Активные стенды" value={`${active}`} tone="primary" />
        <SummaryTile label="Заморожены" value={`${frozen}`} tone="violet" />
        <SummaryTile label="Failed state" value={`${failed}`} tone="danger" />
      </div>

      <div className="grid gap-6 xl:grid-cols-[1fr_360px]">
        <Card>
          <CardHeader>
            <CardTitle>Студенческие стенды</CardTitle>
            <CardDescription>Таблица обновляется после каждого teacher action.</CardDescription>
          </CardHeader>
          <CardContent>
            <Table>
              <TableHeader>
                <TableRow>
                  <TableHead>Студент</TableHead>
                  <TableHead>Состояние</TableHead>
                  <TableHead>Запуск</TableHead>
                  <TableHead>Удаление</TableHead>
                  <TableHead>Последняя проверка</TableHead>
                  <TableHead className="text-right">Действия</TableHead>
                </TableRow>
              </TableHeader>
              <TableBody>
                {labs.length === 0 ? (
                  <TableRow>
                    <TableCell colSpan={6} className="py-8 text-center text-sm text-muted-foreground">
                      Стендов по этому фильтру пока нет.
                    </TableCell>
                  </TableRow>
                ) : null}
                {labs.map((row) => (
                  <TableRow key={row.id}>
                    <TableCell>
                      <div className="font-medium">student {shortId(row.student_user_id)}</div>
                      <div className="text-xs text-muted-foreground">lab {shortId(row.id)}</div>
                    </TableCell>
                    <TableCell>
                      <Badge variant={stateVariants[row.state]}>{stateLabels[row.state]}</Badge>
                    </TableCell>
                    <TableCell>{formatDateTime(row.created_at)}</TableCell>
                    <TableCell>{row.state === "frozen" ? formatDateTime(row.unfreeze_at) : formatDateTime(row.cleanup_at)}</TableCell>
                    <TableCell>
                      <LabLastCheck labId={row.id} />
                    </TableCell>
                    <TableCell className="text-right">
                      <LabActions lab={row} />
                    </TableCell>
                  </TableRow>
                ))}
              </TableBody>
            </Table>
          </CardContent>
        </Card>

        <TimeoutSettingsCard courseId={courseId} settings={settingsQuery.data ?? []} error={settingsQuery.error} refetch={() => void settingsQuery.refetch()} />
      </div>

      <Card>
        <CardHeader>
          <CardTitle>Инциденты и поддержка</CardTitle>
          <CardDescription>Стенды в заморозке остаются доступными для разбора преподавателем.</CardDescription>
        </CardHeader>
        <CardContent className="grid gap-4 md:grid-cols-2">
          {labs.filter((lab) => lab.state === "frozen" || lab.state === "failed").length === 0 ? (
            <div className="rounded-lg border border-border bg-background p-4 text-sm text-muted-foreground">Инцидентов сейчас нет.</div>
          ) : null}
          {labs
            .filter((lab) => lab.state === "frozen" || lab.state === "failed")
            .map((lab) => (
              <div key={`${lab.id}-incident`} className="rounded-lg border border-border bg-background p-4">
                <div className="mb-3 flex items-start justify-between gap-3">
                  <div>
                    <div className="font-medium">student {shortId(lab.student_user_id)}</div>
                    <div className="text-xs text-muted-foreground">project {shortId(lab.project_id ?? undefined)}</div>
                  </div>
                  <Badge variant={lab.state === "frozen" ? "violet" : "danger"}>{lab.state === "frozen" ? "заморожен" : "ошибка"}</Badge>
                </div>
                <div className="text-sm text-muted-foreground">
                  {lab.state === "frozen" ? `Удаление приостановлено до: ${formatDateTime(lab.unfreeze_at)}` : (lab.state_reason ?? "Последний переход завершился ошибкой.")}
                </div>
              </div>
            ))}
        </CardContent>
      </Card>
    </div>
  );
}

function LabLastCheck({ labId }: { labId: string }): JSX.Element {
  const checksQuery = useLabChecks(labId);
  if (checksQuery.isLoading) return <span className="text-xs text-muted-foreground">loading</span>;
  if (checksQuery.error) return <Badge variant="warning">скоро</Badge>;
  const latest = latestCheck(checksQuery.data ?? []);
  if (!latest) return <span className="text-xs text-muted-foreground">none</span>;
  return <Badge variant={latest.state === "passed" ? "success" : latest.state === "failed" || latest.state === "errored" ? "danger" : "warning"}>{latest.state ?? "queued"}</Badge>;
}

function LabActions({ lab }: { lab: LabInstance }): JSX.Element {
  const freezeLab = useFreezeLab();
  const unfreezeLab = useUnfreezeLab();
  const extendLab = useExtendLab();

  async function freeze(): Promise<void> {
    const reason = window.prompt("Причина заморозки", "Разбор обращения студента");
    if (!reason) return;
    await freezeLab.mutateAsync({ id: lab.id, reason, freezeForSeconds: 24 * 60 * 60 });
    toast.success("Стенд заморожен");
  }

  async function extend(): Promise<void> {
    const minutes = window.prompt("На сколько минут продлить?", "30");
    if (!minutes) return;
    const parsed = Number(minutes);
    if (!Number.isFinite(parsed) || parsed < 1) return;
    await extendLab.mutateAsync({ id: lab.id, extendBySeconds: Math.round(parsed * 60) });
    toast.success("Стенд продлён");
  }

  if (lab.state === "frozen") {
    return (
      <Button variant="secondary" size="sm" onClick={() => void unfreezeLab.mutateAsync(lab.id)} disabled={unfreezeLab.isPending}>
        <PlayCircle className="h-4 w-4" />
        Разморозить
      </Button>
    );
  }

  if (lab.state === "ready" || lab.state === "checking") {
    return (
      <div className="flex justify-end gap-2">
        <Button variant="ghost" size="sm" onClick={() => void extend()} disabled={extendLab.isPending}>
          <TimerReset className="h-4 w-4" />
          +30м
        </Button>
        <Button variant="ghost" size="sm" onClick={() => void freeze()} disabled={freezeLab.isPending}>
          <PauseCircle className="h-4 w-4" />
          Freeze
        </Button>
      </div>
    );
  }

  return (
    <Button variant="ghost" size="sm" disabled>
      <FileWarning className="h-4 w-4" />
      Открыть
    </Button>
  );
}

function TimeoutSettingsCard({
  courseId,
  settings,
  error,
  refetch,
}: {
  courseId: string;
  settings: Setting[];
  error: unknown;
  refetch: () => void;
}): JSX.Element {
  const putSetting = usePutSetting();
  const [lifetime, setLifetime] = useState(secondsSetting(settings, "lab_lifetime_seconds", 7200));
  const [freeze, setFreeze] = useState(secondsSetting(settings, "freeze_lifetime_seconds", 86400));
  const [message, setMessage] = useState<string | null>(null);

  async function save(): Promise<void> {
    setMessage(null);
    const scope: SettingUpdate["scope"] = courseId ? "per_course" : "global";
    try {
      await putSetting.mutateAsync({ key: "lab_lifetime_seconds", value: lifetime, scope, scope_id: courseId || null });
      await putSetting.mutateAsync({ key: "freeze_lifetime_seconds", value: freeze, scope, scope_id: courseId || null });
      toast.success("Настройки сохранены");
      refetch();
    } catch (caught) {
      const problem = problemFrom(caught, undefined, "Не удалось сохранить настройки");
      setMessage(problem.problem.detail ?? problem.problem.title);
    }
  }

  return (
    <Card>
      <CardHeader>
        <CardTitle>Настройки лимитов</CardTitle>
        <CardDescription>Teacher-visible `/admin/settings`, применяется к новым стендам курса.</CardDescription>
      </CardHeader>
      <CardContent className="space-y-4">
        {error ? <ErrorBlock error={error} onRetry={refetch} /> : null}
        <TimeoutControl label="Время жизни стенда" value={lifetime} onChange={setLifetime} />
        <TimeoutControl label="Заморозка инцидента" value={freeze} onChange={setFreeze} />
        {message ? <div className="rounded-lg border border-amber-400/30 bg-amber-400/10 p-3 text-sm text-amber-100">{message}</div> : null}
        <div className="rounded-lg border border-cyan-400/20 bg-cyan-400/10 p-3 text-sm text-cyan-100">
          <Clock3 className="mr-2 inline h-4 w-4" />
          Уже созданные стенды сохраняют текущий таймер; новые берут значения из Settings.
        </div>
        <Button className="w-full" onClick={() => void save()} disabled={putSetting.isPending}>
          <Settings className="h-4 w-4" />
          {putSetting.isPending ? "Сохраняем..." : "Применить к новым стендам"}
        </Button>
      </CardContent>
    </Card>
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

function TimeoutControl({ label, value, onChange }: { label: string; value: number; onChange: (next: number) => void }): JSX.Element {
  const hours = Math.max(1, Math.round(value / 3600));
  return (
    <div className="rounded-lg border border-border bg-background p-4">
      <div className="mb-3 flex items-center justify-between gap-3">
        <Label className="text-sm text-muted-foreground">{label}</Label>
        <div className="font-medium">{hours}ч</div>
      </div>
      <Input type="number" min={60} step={60} value={value} onChange={(event) => onChange(Number(event.target.value))} />
      <Progress className="mt-3" value={Math.min(100, (value / 86400) * 100)} />
    </div>
  );
}

function latestCheck(checks: CheckRun[]): CheckRun | undefined {
  return [...checks].sort((left, right) => Date.parse(right.finished_at ?? right.started_at ?? "") - Date.parse(left.finished_at ?? left.started_at ?? ""))[0];
}

function secondsSetting(settings: Setting[], key: string, fallback: number): number {
  const value = settings.find((setting) => setting.key === key)?.value;
  if (typeof value === "number") return value;
  if (typeof value === "string") {
    const parsed = Number(value);
    return Number.isFinite(parsed) ? parsed : fallback;
  }
  return fallback;
}

function formatDateTime(value?: string | null): string {
  return value ? format(new Date(value), "dd.MM HH:mm", { locale: ru }) : "-";
}

function shortId(id?: string): string {
  if (!id) return "-";
  return id.length > 8 ? id.slice(0, 8) : id;
}
