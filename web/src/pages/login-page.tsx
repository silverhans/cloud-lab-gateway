import { useState } from "react";
import type { FormEvent } from "react";
import { useQueryClient } from "@tanstack/react-query";
import { useRouter } from "@tanstack/react-router";
import { ExternalLink, GraduationCap, ShieldCheck, UserRound } from "lucide-react";
import { api } from "@/lib/api";
import { demoAuthEnabled, moodleEmulatorUrl, setDemoUser } from "@/lib/auth";
import { problemFrom } from "@/lib/problem";
import type { Role } from "@/lib/types";
import { Button } from "@/components/ui/button";
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from "@/components/ui/card";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { Separator } from "@/components/ui/separator";

const demoRoles: Array<{ role: Role; label: string; icon: typeof UserRound }> = [
  { role: "student", label: "Dev: студент", icon: UserRound },
  { role: "teacher", label: "Dev: преподаватель", icon: GraduationCap },
  { role: "admin", label: "Dev: админ", icon: ShieldCheck },
];

export function LoginPage(): JSX.Element {
  const router = useRouter();
  const queryClient = useQueryClient();
  const [email, setEmail] = useState("teacher-001@emulator.local");
  const [password, setPassword] = useState("");
  const [message, setMessage] = useState<string | null>(null);
  const [submitting, setSubmitting] = useState(false);
  const showDemoFallback = demoAuthEnabled();
  const moodleUrl = moodleEmulatorUrl();

  async function loginAs(role: Role): Promise<void> {
    setDemoUser(role);
    await queryClient.invalidateQueries({ queryKey: ["auth", "me"] });
    await router.navigate({ to: role === "student" ? "/student" : role === "teacher" ? "/teacher" : "/admin" });
  }

  async function submitLogin(event: FormEvent<HTMLFormElement>): Promise<void> {
    event.preventDefault();
    setSubmitting(true);
    setMessage(null);
    try {
      const { error, response } = await api.POST("/auth/login", {
        body: { email, password },
      });
      if (error) throw problemFrom(error, response, "Не удалось войти");
      await queryClient.invalidateQueries({ queryKey: ["auth", "me"] });
      await router.navigate({ to: "/" });
    } catch (error) {
      const problem = problemFrom(error, undefined, "Не удалось войти");
      setMessage(problem.status === 501 ? "Логин по паролю скоро будет. Для демо используйте Moodle emulator или dev-вход." : problem.problem.detail ?? problem.problem.title);
    } finally {
      setSubmitting(false);
    }
  }

  return (
    <div className="flex min-h-screen items-center justify-center bg-background px-4 py-10">
      <div className="w-full max-w-md">
        <div className="mb-8 text-center">
          <div className="mx-auto mb-4 flex h-12 w-12 items-center justify-center rounded-lg border border-primary/30 bg-primary/15 text-primary">
            <ShieldCheck className="h-6 w-6" />
          </div>
          <h1 className="text-2xl font-semibold tracking-normal">Cloud Lab Gateway</h1>
          <p className="mt-2 text-sm text-muted-foreground">Безопасный доступ к лабораторным стендам КИ</p>
        </div>

        <Card>
          <CardHeader>
            <CardTitle>Вход через Moodle</CardTitle>
            <CardDescription>Moodle подписывает LTI 1.3 launch, шлюз проверяет JWT и выдаёт защищённую сессию.</CardDescription>
          </CardHeader>
          <CardContent className="space-y-5">
            <Button asChild size="lg" className="w-full">
              <a href={moodleUrl}>
                <ExternalLink className="h-4 w-4" />
                Открыть Moodle emulator
              </a>
            </Button>
            <p className="text-xs leading-5 text-muted-foreground">
              В emulator выберите курс, лабораторную и пользователя. После launch браузер вернётся сюда уже с backend-сессией.
            </p>

            <Separator />

            <form className="space-y-4" onSubmit={(event) => void submitLogin(event)}>
              <div>
                <div className="text-sm font-medium">Teacher/admin login</div>
                <p className="mt-1 text-xs text-muted-foreground">Endpoint уже подключён. Если backend вернёт 501, покажем дружелюбное состояние.</p>
              </div>
              <div className="space-y-2">
                <Label htmlFor="email">Email</Label>
                <Input id="email" type="email" value={email} onChange={(event) => setEmail(event.target.value)} autoComplete="email" />
              </div>
              <div className="space-y-2">
                <Label htmlFor="password">Пароль</Label>
                <Input id="password" type="password" value={password} onChange={(event) => setPassword(event.target.value)} autoComplete="current-password" />
              </div>
              {message ? <div className="rounded-md border border-amber-400/30 bg-amber-400/10 p-3 text-sm text-amber-100">{message}</div> : null}
              <Button className="w-full" type="submit" disabled={submitting || password.length < 8}>
                {submitting ? "Входим..." : "Войти по email"}
              </Button>
            </form>

            {showDemoFallback ? (
              <>
                <Separator />
                <div>
                  <div className="mb-2 text-xs font-medium uppercase text-muted-foreground">Dev fallback</div>
                  <div className="grid gap-2">
                    {demoRoles.map((item) => (
                      <Button key={item.role} variant="secondary" onClick={() => void loginAs(item.role)}>
                        <item.icon className="h-4 w-4" />
                        {item.label}
                      </Button>
                    ))}
                  </div>
                </div>
              </>
            ) : null}
          </CardContent>
        </Card>
      </div>
    </div>
  );
}
