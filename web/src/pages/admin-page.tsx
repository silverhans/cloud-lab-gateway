import {
  AlertTriangle,
  Database,
  Filter,
  RefreshCw,
  RotateCcw,
  Search,
  ServerCog,
  ShieldAlert,
  Trash2,
} from "lucide-react";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from "@/components/ui/card";
import { Input } from "@/components/ui/input";
import { Progress } from "@/components/ui/progress";
import { auditEvents, poolGroups, quarantineProjects, type PoolGroup } from "@/lib/mock-data";

export function AdminPage(): JSX.Element {
  const clusterMax = 92;
  const launchBlocked = clusterMax >= 90;

  return (
    <div className="mx-auto max-w-7xl space-y-6">
      <section className="flex flex-col gap-4 sm:flex-row sm:items-start sm:justify-between">
        <div>
          <div className="mb-3 flex flex-wrap items-center gap-2">
            <Badge variant={launchBlocked ? "danger" : "success"}>
              {launchBlocked ? "Запуски заблокированы" : "Запуски разрешены"}
            </Badge>
            <span className="text-sm text-muted-foreground">Последнее обновление 12 сек назад</span>
          </div>
          <h1 className="text-2xl font-semibold tracking-normal sm:text-3xl">Панель администратора</h1>
          <p className="mt-2 text-sm text-muted-foreground">
            Утилизация кластера, пул проектов и аудит событий в одном месте.
          </p>
        </div>
        <Button variant="secondary">
          <RefreshCw className="h-4 w-4" />
          Обновить квоты
        </Button>
      </section>

      {launchBlocked ? (
        <Card className="border-red-400/35 bg-red-400/10 shadow-[0_0_0_1px_rgba(248,113,113,0.12)]">
          <CardContent className="flex flex-col gap-4 p-5 md:flex-row md:items-center md:justify-between">
            <div className="flex gap-3">
              <ShieldAlert className="mt-1 h-5 w-5 shrink-0 text-red-300" />
              <div>
                <div className="font-semibold text-red-100">Предиктивный лимит сработал</div>
                <p className="mt-1 text-sm text-red-100/80">
                  CPU после следующего запуска превысит 90%, поэтому новые стенды временно не выдаются.
                </p>
              </div>
            </div>
            <Badge variant="danger">max utilization {clusterMax}%</Badge>
          </CardContent>
        </Card>
      ) : null}

      <div className="grid gap-6 lg:grid-cols-4">
        <UtilCard label="CPU" value={67} />
        <UtilCard label="RAM" value={54} />
        <UtilCard label="Disk" value={41} />
        <UtilCard label="Prediction" value={clusterMax} danger />
      </div>

      <div className="grid gap-6 xl:grid-cols-[1fr_420px]">
        <Card>
          <CardHeader>
            <CardTitle>Пул проектов</CardTitle>
            <CardDescription>Тепловая карта заранее подготовленных КИ-проектов.</CardDescription>
          </CardHeader>
          <CardContent className="space-y-5">
            {poolGroups.map((group) => <PoolHeatmap key={group.course} group={group} />)}
          </CardContent>
        </Card>

        <Card>
          <CardHeader>
            <CardTitle>Audit log</CardTitle>
            <CardDescription>Фильтруемая лента событий для разбора инцидентов.</CardDescription>
          </CardHeader>
          <CardContent className="space-y-3">
            <div className="flex gap-2">
              <div className="relative flex-1">
                <Search className="absolute left-3 top-2.5 h-4 w-4 text-muted-foreground" />
                <Input className="pl-9" placeholder="Поиск события" />
              </div>
              <Button variant="outline" size="icon" aria-label="Фильтры аудита">
                <Filter className="h-4 w-4" />
              </Button>
            </div>
            {auditEvents.map((event) => (
              <AuditItem
                key={`${event.time}-${event.kind}`}
                icon={event.kind.includes("quota") ? AlertTriangle : event.kind.includes("project") ? Database : ServerCog}
                time={event.time}
                title={event.kind}
                detail={event.detail}
                actor={event.actor}
              />
            ))}
          </CardContent>
        </Card>
      </div>

      <Card>
        <CardHeader>
          <CardTitle>Карантин проектов</CardTitle>
          <CardDescription>Проекты не возвращаются в пул, пока cleanup не будет разобран вручную.</CardDescription>
        </CardHeader>
        <CardContent className="grid gap-4 md:grid-cols-2">
          {quarantineProjects.map((project) => (
            <div key={project.id} className="rounded-lg border border-red-400/25 bg-red-400/8 p-4">
              <div className="mb-3 flex items-start justify-between gap-3">
                <div>
                  <div className="font-mono text-sm font-semibold text-red-100">{project.id}</div>
                  <div className="text-xs text-red-100/70">{project.course} • с {project.since}</div>
                </div>
                <Badge variant="danger">{project.failures} failures</Badge>
              </div>
              <div className="mb-4 rounded-md border border-red-400/20 bg-background/70 p-3 font-mono text-xs text-red-100/85">
                {project.error}
              </div>
              <div className="flex flex-wrap gap-2">
                <Button variant="secondary" size="sm">
                  <RotateCcw className="h-4 w-4" />
                  Повторить cleanup
                </Button>
                <Button variant="ghost" size="sm">
                  <Trash2 className="h-4 w-4" />
                  Decommission
                </Button>
              </div>
            </div>
          ))}
        </CardContent>
      </Card>
    </div>
  );
}

function UtilCard({ label, value, danger = false }: { label: string; value: number; danger?: boolean }): JSX.Element {
  const blocked = danger || value >= 90;
  return (
    <Card className={blocked ? "border-red-400/30" : undefined}>
      <CardHeader>
        <CardTitle>{label}</CardTitle>
        <CardDescription>Порог блокировки 90%</CardDescription>
      </CardHeader>
      <CardContent>
        <div className={`mb-4 text-4xl font-semibold ${blocked ? "text-red-300" : ""}`}>{value}%</div>
        <Progress value={value} />
      </CardContent>
    </Card>
  );
}

function PoolHeatmap({ group }: { group: PoolGroup }): JSX.Element {
  const total = group.free + group.allocated + group.cleaning + group.quarantine;
  return (
    <div className="rounded-lg border border-border bg-background p-4">
      <div className="mb-3 flex items-center justify-between">
        <div>
          <div className="font-medium">{group.course}</div>
          <div className="text-xs text-muted-foreground">{total} проектов в пуле</div>
        </div>
        <Badge variant={group.quarantine > 0 ? "warning" : "success"}>{group.free} free</Badge>
      </div>
      <div className="mb-4 flex flex-wrap gap-1.5">
        {Array.from({ length: group.free }).map((_, index) => (
          <span key={`free-${index}`} className="h-5 w-5 rounded bg-emerald-400/70" title="free" />
        ))}
        {Array.from({ length: group.allocated }).map((_, index) => (
          <span key={`allocated-${index}`} className="h-5 w-5 rounded bg-amber-400/70" title="allocated" />
        ))}
        {Array.from({ length: group.cleaning }).map((_, index) => (
          <span key={`cleaning-${index}`} className="h-5 w-5 rounded bg-slate-400/70" title="cleaning" />
        ))}
        {Array.from({ length: group.quarantine }).map((_, index) => (
          <span key={`quarantine-${index}`} className="h-5 w-5 rounded bg-red-400/70" title="quarantine" />
        ))}
      </div>
      <div className="grid grid-cols-4 gap-2 text-xs text-muted-foreground">
        <span>{group.free} free</span>
        <span>{group.allocated} alloc</span>
        <span>{group.cleaning} clean</span>
        <span>{group.quarantine} quarantine</span>
      </div>
    </div>
  );
}

function AuditItem({
  icon: Icon,
  time,
  title,
  detail,
  actor,
}: {
  icon: React.ComponentType<{ className?: string }>;
  time: string;
  title: string;
  detail: string;
  actor: string;
}): JSX.Element {
  return (
    <div className="flex gap-3 rounded-lg border border-border bg-background p-3">
      <Icon className="mt-0.5 h-4 w-4 text-primary" />
      <div className="min-w-0 flex-1">
        <div className="flex items-center justify-between gap-3">
          <div className="truncate font-mono text-sm">{title}</div>
          <div className="shrink-0 text-xs text-muted-foreground">{time}</div>
        </div>
        <div className="mt-1 text-xs text-muted-foreground">{detail}</div>
        <div className="mt-1 text-xs text-muted-foreground">actor: {actor}</div>
      </div>
    </div>
  );
}
