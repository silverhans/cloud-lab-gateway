import { Settings } from "lucide-react";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from "@/components/ui/card";
import { Table, TableBody, TableCell, TableHead, TableHeader, TableRow } from "@/components/ui/table";

const rows = [
  { student: "Иван Петров", state: "Ready", created: "12:43", cleanup: "14:43" },
  { student: "Анна Смирнова", state: "Checking", created: "12:21", cleanup: "14:21" },
  { student: "Олег Иванов", state: "Frozen", created: "11:55", cleanup: "заморожен" },
  { student: "Мария Кузнецова", state: "Deploying", created: "12:48", cleanup: "-" },
];

export function TeacherPage(): JSX.Element {
  return (
    <div className="mx-auto max-w-7xl space-y-6">
      <section className="flex flex-col gap-4 sm:flex-row sm:items-start sm:justify-between">
        <div>
          <h1 className="text-2xl font-semibold tracking-normal sm:text-3xl">Лабы курса</h1>
          <p className="mt-2 text-sm text-muted-foreground">Linux основы • активных стендов 14 / 30</p>
        </div>
        <Button variant="secondary">
          <Settings className="h-4 w-4" />
          Настройки таймаутов
        </Button>
      </section>

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
                {rows.map((row) => (
                  <TableRow key={row.student}>
                    <TableCell className="font-medium">{row.student}</TableCell>
                    <TableCell>
                      <Badge variant={row.state === "Ready" ? "success" : row.state === "Frozen" ? "violet" : "warning"}>
                        {row.state}
                      </Badge>
                    </TableCell>
                    <TableCell>{row.created}</TableCell>
                    <TableCell>{row.cleanup}</TableCell>
                    <TableCell className="text-right">
                      <Button variant="ghost" size="sm">Открыть</Button>
                    </TableCell>
                  </TableRow>
                ))}
              </TableBody>
            </Table>
          </CardContent>
        </Card>

        <Card>
          <CardHeader>
            <CardTitle>Лимиты</CardTitle>
            <CardDescription>Новые стенды курса наследуют эти значения.</CardDescription>
          </CardHeader>
          <CardContent className="space-y-4">
            <Metric label="Время жизни" value="2ч 00м" />
            <Metric label="Заморозка инцидента" value="24ч" />
            <Metric label="Активных стендов" value="14 / 30" />
          </CardContent>
        </Card>
      </div>
    </div>
  );
}

function Metric({ label, value }: { label: string; value: string }): JSX.Element {
  return (
    <div className="rounded-lg border border-border bg-background p-3">
      <div className="text-xs uppercase text-muted-foreground">{label}</div>
      <div className="mt-1 text-xl font-semibold">{value}</div>
    </div>
  );
}
