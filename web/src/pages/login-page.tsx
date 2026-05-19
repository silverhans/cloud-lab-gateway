import { useQueryClient } from "@tanstack/react-query";
import { useRouter } from "@tanstack/react-router";
import { GraduationCap, ShieldCheck, UserRound } from "lucide-react";
import { setDemoUser } from "@/lib/auth";
import type { Role } from "@/lib/types";
import { Button } from "@/components/ui/button";
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from "@/components/ui/card";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";

const demoRoles: Array<{ role: Role; label: string; icon: typeof UserRound }> = [
  { role: "student", label: "Войти как студент", icon: UserRound },
  { role: "teacher", label: "Войти как преподаватель", icon: GraduationCap },
  { role: "admin", label: "Войти как админ", icon: ShieldCheck },
];

export function LoginPage(): JSX.Element {
  const router = useRouter();
  const queryClient = useQueryClient();

  function loginAs(role: Role): void {
    setDemoUser(role);
    void queryClient.invalidateQueries({ queryKey: ["auth", "me"] });
    void router.navigate({ to: role === "student" ? "/student" : role === "teacher" ? "/teacher" : "/admin" });
  }

  return (
    <div className="flex min-h-screen items-center justify-center bg-background px-4 py-10">
      <div className="w-full max-w-md">
        <div className="mb-8 text-center">
          <div className="mx-auto mb-4 flex h-12 w-12 items-center justify-center rounded-lg border border-primary/30 bg-primary/15 text-primary">
            <ShieldCheck className="h-6 w-6" />
          </div>
          <h1 className="text-2xl font-semibold tracking-normal">Cloud Lab Gateway</h1>
          <p className="mt-2 text-sm text-muted-foreground">
            Безопасный доступ к лабораторным стендам КИ
          </p>
        </div>

        <Card>
          <CardHeader>
            <CardTitle>Вход для демо</CardTitle>
            <CardDescription>
              Пока Moodle и КИ не подключены, можно переключаться между ролями локально.
            </CardDescription>
          </CardHeader>
          <CardContent className="space-y-5">
            <div className="space-y-3">
              <div className="space-y-2">
                <Label htmlFor="email">Email</Label>
                <Input id="email" value="student-001@emulator.local" readOnly />
              </div>
              <div className="space-y-2">
                <Label htmlFor="password">Пароль</Label>
                <Input id="password" type="password" value="demo-password" readOnly />
              </div>
            </div>

            <div className="grid gap-2">
              {demoRoles.map((item) => (
                <Button key={item.role} variant="secondary" onClick={() => loginAs(item.role)}>
                  <item.icon className="h-4 w-4" />
                  {item.label}
                </Button>
              ))}
            </div>
          </CardContent>
        </Card>
      </div>
    </div>
  );
}
