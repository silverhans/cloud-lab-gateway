import { useMemo, useState } from "react";
import { formatDistanceToNowStrict, format } from "date-fns";
import { ru } from "date-fns/locale";
import { AlertTriangle, Database, Filter, RefreshCw, RotateCcw, Search, ServerCog, ShieldAlert } from "lucide-react";
import { toast } from "sonner";
import { ErrorBlock, LoadingBlock } from "@/components/async-state";
import { QuotaBar } from "@/components/quota-bar";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from "@/components/ui/card";
import { Input } from "@/components/ui/input";
import { Table, TableBody, TableCell, TableHead, TableHeader, TableRow } from "@/components/ui/table";
import type { AuditEvent, Project, ProjectState, QuotaSnapshot } from "@/lib/types";
import { useAudit, useProjects, useQuarantineProject, useQuota, useReleaseProject } from "@/hooks/use-admin";

const projectStates: ProjectState[] = ["free", "allocated", "cleaning", "quarantine", "decommissioned"];

type PoolGroup = {
  domain: string;
  projects: Project[];
  counts: Record<ProjectState, number>;
};

export function AdminPage(): JSX.Element {
  const [kindFilter, setKindFilter] = useState("");
  const projectsQuery = useProjects();
  const quotaQuery = useQuota();
  const auditQuery = useAudit({ kind: kindFilter || undefined, limit: 100 });
  const projects = useMemo(() => projectsQuery.data ?? [], [projectsQuery.data]);
  const quota = quotaQuery.data;
  const threshold = quota?.threshold_pct ?? 90;
  const clusterMax = quota?.utilization_pct?.max ?? 0;
  const launchBlocked = clusterMax >= threshold;
  const poolGroups = useMemo(() => groupProjects(projects), [projects]);
  const quarantineProjects = projects.filter((project) => project.state === "quarantine");

  if (projectsQuery.isLoading || quotaQuery.isLoading) return <LoadingBlock label="Загружаем админскую панель..." />;

  return (
    <div className="mx-auto max-w-7xl space-y-6">
      <section className="flex flex-col gap-4 sm:flex-row sm:items-start sm:justify-between">
        <div>
          <div className="mb-3 flex flex-wrap items-center gap-2">
            <Badge variant={launchBlocked ? "danger" : "success"}>{launchBlocked ? "Запуски заблокированы" : "Запуски разрешены"}</Badge>
            <span className="text-sm text-muted-foreground">{quota?.fetched_at ? `обновлено ${formatDistanceToNowStrict(new Date(quota.fetched_at), { locale: ru })} назад` : "квоты ожидаются"}</span>
          </div>
          <h1 className="text-2xl font-semibold tracking-normal sm:text-3xl">Панель администратора</h1>
          <p className="mt-2 text-sm text-muted-foreground">Утилизация кластера, пул проектов и аудит событий в одном месте.</p>
        </div>
        <Button variant="secondary" onClick={() => void quotaQuery.refetch()}>
          <RefreshCw className="h-4 w-4" />
          Обновить квоты
        </Button>
      </section>

      {projectsQuery.error ? <ErrorBlock error={projectsQuery.error} onRetry={() => void projectsQuery.refetch()} /> : null}
      {quotaQuery.error ? <ErrorBlock error={quotaQuery.error} onRetry={() => void quotaQuery.refetch()} /> : null}

      {launchBlocked ? (
        <Card className="border-red-400/35 bg-red-400/10 shadow-[0_0_0_1px_rgba(248,113,113,0.12)]">
          <CardContent className="flex flex-col gap-4 p-5 md:flex-row md:items-center md:justify-between">
            <div className="flex gap-3">
              <ShieldAlert className="mt-1 h-5 w-5 shrink-0 text-red-300" />
              <div>
                <div className="font-semibold text-red-100">Предиктивный лимит сработал</div>
                <p className="mt-1 text-sm text-red-100/80">Максимальная утилизация выше порога {threshold}%, новые стенды временно не выдаются.</p>
              </div>
            </div>
            <Badge variant="danger">max utilization {Math.round(clusterMax)}%</Badge>
          </CardContent>
        </Card>
      ) : null}

      <QuotaGrid quota={quota} />

      <div className="grid gap-6 xl:grid-cols-[1fr_420px]">
        <Card>
          <CardHeader>
            <CardTitle>Пул проектов</CardTitle>
            <CardDescription>Группировка заранее подготовленных КИ-проектов по доменам и состояниям.</CardDescription>
          </CardHeader>
          <CardContent className="space-y-5">
            {poolGroups.length === 0 ? <div className="rounded-lg border border-border bg-background p-4 text-sm text-muted-foreground">Проекты пока не загружены.</div> : null}
            {poolGroups.map((group) => (
              <PoolHeatmap key={group.domain} group={group} />
            ))}
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
                <Input className="pl-9" placeholder="kind, например quota.blocked" value={kindFilter} onChange={(event) => setKindFilter(event.target.value)} />
              </div>
              <Button variant="outline" size="icon" aria-label="Фильтры аудита" onClick={() => void auditQuery.refetch()}>
                <Filter className="h-4 w-4" />
              </Button>
            </div>
            {auditQuery.error ? <ErrorBlock error={auditQuery.error} onRetry={() => void auditQuery.refetch()} /> : null}
            {auditQuery.isLoading ? <LoadingBlock label="Загружаем аудит..." /> : null}
            {(auditQuery.data ?? []).map((event) => (
              <AuditItem key={event.id ?? `${event.occurred_at}-${event.kind}`} event={event} />
            ))}
            {!auditQuery.isLoading && (auditQuery.data ?? []).length === 0 ? <div className="rounded-lg border border-border bg-background p-3 text-sm text-muted-foreground">Событий нет.</div> : null}
          </CardContent>
        </Card>
      </div>

      <Card>
        <CardHeader>
          <CardTitle>Карантин проектов</CardTitle>
          <CardDescription>Проекты не возвращаются в пул, пока cleanup не будет разобран вручную.</CardDescription>
        </CardHeader>
        <CardContent className="grid gap-4 md:grid-cols-2">
          {quarantineProjects.length === 0 ? <div className="rounded-lg border border-border bg-background p-4 text-sm text-muted-foreground">Карантин пуст.</div> : null}
          {quarantineProjects.map((project) => (
            <QuarantineProjectCard key={project.id ?? project.ki_project_id} project={project} />
          ))}
        </CardContent>
      </Card>
    </div>
  );
}

function QuotaGrid({ quota }: { quota?: QuotaSnapshot }): JSX.Element {
  const threshold = quota?.threshold_pct ?? 90;
  return (
    <div className="grid gap-6 lg:grid-cols-4">
      <UtilCard label="CPU" value={quota?.utilization_pct?.vcpus} threshold={threshold} />
      <UtilCard label="RAM" value={quota?.utilization_pct?.ram} threshold={threshold} />
      <UtilCard label="Disk" value={quota?.utilization_pct?.disk} threshold={threshold} />
      <UtilCard label="Max" value={quota?.utilization_pct?.max} threshold={threshold} danger />
    </div>
  );
}

function UtilCard({ label, value = 0, threshold, danger = false }: { label: string; value?: number; threshold: number; danger?: boolean }): JSX.Element {
  const blocked = danger || value >= threshold;
  return (
    <Card className={blocked ? "border-red-400/30" : undefined}>
      <CardHeader>
        <CardTitle>{label}</CardTitle>
        <CardDescription>Порог блокировки {threshold}%</CardDescription>
      </CardHeader>
      <CardContent>
        <div className={`mb-4 text-4xl font-semibold ${blocked ? "text-red-300" : ""}`}>{Math.round(value)}%</div>
        <QuotaBar label={label} value={value} threshold={threshold} />
      </CardContent>
    </Card>
  );
}

function PoolHeatmap({ group }: { group: PoolGroup }): JSX.Element {
  const total = group.projects.length;
  return (
    <div className="rounded-lg border border-border bg-background p-4">
      <div className="mb-3 flex items-center justify-between">
        <div>
          <div className="font-medium">{group.domain}</div>
          <div className="text-xs text-muted-foreground">{total} проектов в пуле</div>
        </div>
        <Badge variant={group.counts.quarantine > 0 ? "warning" : "success"}>{group.counts.free} free</Badge>
      </div>
      <div className="mb-4 flex flex-wrap gap-1.5">
        {group.projects.map((project) => (
          <ProjectDot key={project.id ?? project.ki_project_id ?? project.name} project={project} />
        ))}
      </div>
      <div className="grid grid-cols-5 gap-2 text-xs text-muted-foreground">
        {projectStates.map((state) => (
          <span key={state}>{group.counts[state]} {state}</span>
        ))}
      </div>
      <ProjectTable projects={group.projects} />
    </div>
  );
}

function ProjectDot({ project }: { project: Project }): JSX.Element {
  const color =
    project.state === "free"
      ? "bg-emerald-400/70"
      : project.state === "allocated"
        ? "bg-amber-400/70"
        : project.state === "cleaning"
          ? "bg-slate-400/70"
          : project.state === "quarantine"
            ? "bg-red-400/70"
            : "bg-zinc-500/70";
  return <span className={`h-5 w-5 rounded ${color}`} title={`${project.name ?? project.id}: ${project.state ?? "unknown"}`} />;
}

function ProjectTable({ projects }: { projects: Project[] }): JSX.Element {
  const quarantineProject = useQuarantineProject();
  const releaseProject = useReleaseProject();

  async function quarantine(project: Project): Promise<void> {
    const id = project.id;
    if (!id) return;
    const reason = window.prompt("Причина карантина", "manual admin quarantine");
    if (!reason) return;
    await quarantineProject.mutateAsync({ id, reason });
    toast.success("Проект переведён в карантин");
  }

  async function release(project: Project): Promise<void> {
    const id = project.id;
    if (!id) return;
    await releaseProject.mutateAsync(id);
    toast.success("Проект возвращён в пул");
  }

  return (
    <div className="mt-4 overflow-hidden rounded-lg border border-border">
      <Table>
        <TableHeader>
          <TableRow>
            <TableHead>Project</TableHead>
            <TableHead>State</TableHead>
            <TableHead>Failures</TableHead>
            <TableHead className="text-right">Action</TableHead>
          </TableRow>
        </TableHeader>
        <TableBody>
          {projects.slice(0, 8).map((project) => (
            <TableRow key={project.id ?? project.ki_project_id ?? project.name}>
              <TableCell>
                <div className="font-mono text-xs">{project.name ?? shortId(project.id)}</div>
                <div className="text-xs text-muted-foreground">{project.ki_project_id ?? "-"}</div>
              </TableCell>
              <TableCell>{project.state ?? "-"}</TableCell>
              <TableCell>{project.cleanup_failures ?? 0}</TableCell>
              <TableCell className="text-right">
                {project.state === "quarantine" ? (
                  <Button variant="secondary" size="sm" onClick={() => void release(project)} disabled={releaseProject.isPending}>
                    Release
                  </Button>
                ) : (
                  <Button variant="ghost" size="sm" onClick={() => void quarantine(project)} disabled={quarantineProject.isPending}>
                    Quarantine
                  </Button>
                )}
              </TableCell>
            </TableRow>
          ))}
        </TableBody>
      </Table>
    </div>
  );
}

function QuarantineProjectCard({ project }: { project: Project }): JSX.Element {
  const releaseProject = useReleaseProject();
  const changedAt = project.last_state_change_at ? format(new Date(project.last_state_change_at), "dd.MM HH:mm", { locale: ru }) : "-";
  return (
    <div className="rounded-lg border border-red-400/25 bg-red-400/8 p-4">
      <div className="mb-3 flex items-start justify-between gap-3">
        <div>
          <div className="font-mono text-sm font-semibold text-red-100">{project.name ?? shortId(project.id)}</div>
          <div className="text-xs text-red-100/70">{project.ki_domain_id ?? "domain unknown"} • с {changedAt}</div>
        </div>
        <Badge variant="danger">{project.cleanup_failures ?? 0} failures</Badge>
      </div>
      <div className="mb-4 rounded-md border border-red-400/20 bg-background/70 p-3 font-mono text-xs text-red-100/85">
        last_error не отдан текущим OpenAPI-контрактом; доступен только cleanup_failures.
      </div>
      <Button variant="secondary" size="sm" onClick={() => project.id && void releaseProject.mutateAsync(project.id)} disabled={releaseProject.isPending || !project.id}>
        <RotateCcw className="h-4 w-4" />
        Release to pool
      </Button>
    </div>
  );
}

function AuditItem({ event }: { event: AuditEvent }): JSX.Element {
  const Icon = event.kind?.includes("quota") ? AlertTriangle : event.kind?.includes("project") ? Database : ServerCog;
  return (
    <div className="flex gap-3 rounded-lg border border-border bg-background p-3">
      <Icon className="mt-0.5 h-4 w-4 text-primary" />
      <div className="min-w-0 flex-1">
        <div className="flex items-center justify-between gap-3">
          <div className="truncate font-mono text-sm">{event.kind ?? "audit.event"}</div>
          <div className="shrink-0 text-xs text-muted-foreground">{event.occurred_at ? format(new Date(event.occurred_at), "HH:mm:ss", { locale: ru }) : "-"}</div>
        </div>
        <div className="mt-1 text-xs text-muted-foreground">{event.subject_type ?? "subject"} {shortId(event.subject_id ?? undefined)}</div>
        <div className="mt-1 text-xs text-muted-foreground">actor: {shortId(event.actor_user_id ?? undefined) || "system"}</div>
        {event.payload ? <pre className="mt-2 max-h-24 overflow-auto rounded bg-muted p-2 text-xs">{stringifyPayload(event.payload)}</pre> : null}
      </div>
    </div>
  );
}

function groupProjects(projects: Project[]): PoolGroup[] {
  const groups = new Map<string, Project[]>();
  for (const project of projects) {
    const domain = project.ki_domain_id ?? "unknown-domain";
    groups.set(domain, [...(groups.get(domain) ?? []), project]);
  }
  return [...groups.entries()].map(([domain, groupProjects]) => ({
    domain,
    projects: groupProjects,
    counts: projectStates.reduce<Record<ProjectState, number>>(
      (acc, state) => {
        acc[state] = groupProjects.filter((project) => project.state === state).length;
        return acc;
      },
      { free: 0, allocated: 0, cleaning: 0, quarantine: 0, decommissioned: 0 },
    ),
  }));
}

function stringifyPayload(payload: unknown): string {
  if (typeof payload === "string") return payload;
  try {
    return JSON.stringify(payload, null, 2);
  } catch {
    return "[unserializable payload]";
  }
}

function shortId(id?: string | null): string {
  if (!id) return "";
  return id.length > 8 ? id.slice(0, 8) : id;
}
