import { AlertTriangle, Loader2 } from "lucide-react";
import { Button } from "@/components/ui/button";
import { Card, CardContent } from "@/components/ui/card";
import { isNotImplemented, problemMessage } from "@/lib/problem";

export function LoadingBlock({ label = "Загружаем данные..." }: { label?: string }): JSX.Element {
  return (
    <div className="flex items-center justify-center rounded-lg border border-border bg-card p-8 text-sm text-muted-foreground">
      <Loader2 className="mr-2 h-4 w-4 animate-spin" />
      {label}
    </div>
  );
}

export function ErrorBlock({ error, onRetry }: { error: unknown; onRetry?: () => void }): JSX.Element {
  const soon = isNotImplemented(error);
  return (
    <Card className={soon ? "border-amber-400/30 bg-amber-400/10" : "border-red-400/30 bg-red-400/10"}>
      <CardContent className="flex flex-col gap-3 p-5 sm:flex-row sm:items-center sm:justify-between">
        <div className="flex gap-3">
          <AlertTriangle className={soon ? "mt-0.5 h-5 w-5 text-amber-300" : "mt-0.5 h-5 w-5 text-red-300"} />
          <div>
            <div className="font-medium">{soon ? "Функция скоро будет" : "Не удалось загрузить данные"}</div>
            <div className="mt-1 text-sm text-muted-foreground">{problemMessage(error)}</div>
          </div>
        </div>
        {onRetry ? (
          <Button variant="secondary" onClick={onRetry}>
            Повторить
          </Button>
        ) : null}
      </CardContent>
    </Card>
  );
}
