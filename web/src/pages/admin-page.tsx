import { AlertTriangle, Database, ServerCog } from "lucide-react";
import type React from "react";
import { Badge } from "@/components/ui/badge";
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from "@/components/ui/card";
import { Progress } from "@/components/ui/progress";

const poolGroups = [
  { course: "Linux основы", free: 7, allocated: 5, cleaning: 2, quarantine: 1 },
  { course: "Nginx и конфигурация", free: 5, allocated: 4, cleaning: 1, quarantine: 0 },
];

export function AdminPage(): JSX.Element {
  return (
    <div className="mx-auto max-w-7xl space-y-6">
      <section>
        <h1 className="text-2xl font-semibold tracking-normal sm:text-3xl">Панель администратора</h1>
        <p className="mt-2 text-sm text-muted-foreground">
          Утилизация кластера, пул проектов и события аудита в одном месте.
        </p>
      </section>

      <div className="grid gap-6 lg:grid-cols-3">
        <UtilCard label="CPU" value={67} />
        <UtilCard label="RAM" value={54} />
        <UtilCard label="Disk" value={41} />
      </div>

      <div className="grid gap-6 xl:grid-cols-[1fr_420px]">
        <Card>
          <CardHeader>
            <CardTitle>Пул проектов</CardTitle>
            <CardDescription>Тепловая карта заранее подготовленных КИ-проектов.</CardDescription>
          </CardHeader>
          <CardContent className="space-y-5">
            {poolGroups.map((group) => (
              <div key={group.course} className="rounded-lg border border-border bg-background p-4">
                <div className="mb-3 flex items-center justify-between">
                  <div className="font-medium">{group.course}</div>
                  <Badge variant={group.quarantine > 0 ? "warning" : "success"}>
                    {group.free} free
                  </Badge>
                </div>
                <div className="flex flex-wrap gap-1.5">
                  {Array.from({ length: group.free }).map((_, index) => (
                    <span key={`free-${index}`} className="h-5 w-5 rounded bg-emerald-400/70" />
                  ))}
                  {Array.from({ length: group.allocated }).map((_, index) => (
                    <span key={`allocated-${index}`} className="h-5 w-5 rounded bg-amber-400/70" />
                  ))}
                  {Array.from({ length: group.cleaning }).map((_, index) => (
                    <span key={`cleaning-${index}`} className="h-5 w-5 rounded bg-slate-400/70" />
                  ))}
                  {Array.from({ length: group.quarantine }).map((_, index) => (
                    <span key={`quarantine-${index}`} className="h-5 w-5 rounded bg-red-400/70" />
                  ))}
                </div>
              </div>
            ))}
          </CardContent>
        </Card>

        <Card>
          <CardHeader>
            <CardTitle>Audit log</CardTitle>
            <CardDescription>Последние события шлюза.</CardDescription>
          </CardHeader>
          <CardContent className="space-y-3">
            <AuditItem icon={ServerCog} title="lab.state_changed" detail="lab-demo-001 -> ready" />
            <AuditItem icon={Database} title="project.allocated" detail="proj-007 -> lab-demo-001" />
            <AuditItem icon={AlertTriangle} title="quota.checked" detail="prediction 67% < 90%" />
          </CardContent>
        </Card>
      </div>
    </div>
  );
}

function UtilCard({ label, value }: { label: string; value: number }): JSX.Element {
  return (
    <Card>
      <CardHeader>
        <CardTitle>{label}</CardTitle>
        <CardDescription>Порог блокировки 90%</CardDescription>
      </CardHeader>
      <CardContent>
        <div className="mb-4 text-4xl font-semibold">{value}%</div>
        <Progress value={value} />
      </CardContent>
    </Card>
  );
}

function AuditItem({
  icon: Icon,
  title,
  detail,
}: {
  icon: React.ComponentType<{ className?: string }>;
  title: string;
  detail: string;
}): JSX.Element {
  return (
    <div className="flex gap-3 rounded-lg border border-border bg-background p-3">
      <Icon className="mt-0.5 h-4 w-4 text-primary" />
      <div>
        <div className="font-mono text-sm">{title}</div>
        <div className="text-xs text-muted-foreground">{detail}</div>
      </div>
    </div>
  );
}
