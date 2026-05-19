import { Link, useRouter } from "@tanstack/react-router";
import type React from "react";
import {
  Activity,
  BookOpenCheck,
  ClipboardList,
  DoorOpen,
  Gauge,
  GraduationCap,
  LifeBuoy,
  LockKeyhole,
  Server,
  Settings,
  Shield,
} from "lucide-react";
import { clearStoredUser } from "@/lib/auth";
import type { CurrentUser, Role } from "@/lib/types";
import { Button } from "@/components/ui/button";
import { Separator } from "@/components/ui/separator";
import { RoleBadge } from "@/components/role-badge";
import { cn } from "@/lib/cn";

type AppShellProps = {
  user: CurrentUser;
  children: React.ReactNode;
};

type NavItem = {
  to: string;
  label: string;
  icon: React.ComponentType<{ className?: string }>;
  roles: Role[];
};

const navItems: NavItem[] = [
  { to: "/student", label: "Моя лаба", icon: BookOpenCheck, roles: ["student"] },
  { to: "/teacher", label: "Лабы курса", icon: GraduationCap, roles: ["teacher"] },
  { to: "/teacher", label: "Настройки", icon: Settings, roles: ["teacher"] },
  { to: "/admin", label: "Утилизация", icon: Gauge, roles: ["admin"] },
  { to: "/admin", label: "Пул проектов", icon: Server, roles: ["admin"] },
  { to: "/admin", label: "Аудит", icon: ClipboardList, roles: ["teacher", "admin"] },
  { to: "/admin", label: "Карантин", icon: Shield, roles: ["admin"] },
];

export function AppShell({ user, children }: AppShellProps): JSX.Element {
  const router = useRouter();
  const visibleItems = navItems.filter((item) => item.roles.includes(user.role));

  function logout(): void {
    clearStoredUser();
    void router.invalidate();
    void router.navigate({ to: "/login" });
  }

  return (
    <div className="min-h-screen bg-background">
      <div className="fixed inset-y-0 left-0 hidden w-64 border-r border-border bg-card/70 backdrop-blur xl:block">
        <div className="flex h-full flex-col">
          <div className="p-5">
            <div className="flex items-center gap-3">
              <div className="flex h-10 w-10 items-center justify-center rounded-lg border border-primary/30 bg-primary/15 text-primary">
                <LockKeyhole className="h-5 w-5" />
              </div>
              <div>
                <div className="text-sm font-semibold">Cloud Lab Gateway</div>
                <div className="text-xs text-muted-foreground">КИ orchestration</div>
              </div>
            </div>
          </div>

          <nav className="flex-1 space-y-1 px-3">
            {visibleItems.map((item) => (
              <Link
                key={`${item.to}-${item.label}`}
                to={item.to}
                className={cn(
                  "flex items-center gap-3 rounded-md px-3 py-2 text-sm text-muted-foreground transition-colors hover:bg-muted hover:text-foreground",
                  "data-[status=active]:bg-primary/10 data-[status=active]:text-primary",
                )}
              >
                <item.icon className="h-4 w-4" />
                {item.label}
              </Link>
            ))}
          </nav>

          <div className="p-4">
            <Separator className="mb-4" />
            <Button variant="ghost" className="w-full justify-start" onClick={logout}>
              <DoorOpen className="h-4 w-4" />
              Выйти
            </Button>
          </div>
        </div>
      </div>

      <main className="xl:pl-64">
        <header className="sticky top-0 z-20 border-b border-border bg-background/90 backdrop-blur">
          <div className="flex h-16 items-center justify-between px-4 sm:px-6 lg:px-8">
            <div className="flex min-w-0 items-center gap-3">
              <Activity className="h-5 w-5 shrink-0 text-primary" />
              <div className="truncate text-sm text-muted-foreground">Лабораторные стенды</div>
            </div>
            <div className="flex items-center gap-3">
              <div className="hidden text-right sm:block">
                <div className="text-sm font-medium">{user.displayName}</div>
                <div className="text-xs text-muted-foreground">{user.email}</div>
              </div>
              <RoleBadge role={user.role} />
            </div>
          </div>
        </header>

        <div className="px-4 py-6 sm:px-6 lg:px-8">{children}</div>
      </main>

      <div className="fixed bottom-4 right-4 xl:hidden">
        <Button variant="secondary" size="sm">
          <LifeBuoy className="h-4 w-4" />
          Помощь
        </Button>
      </div>
    </div>
  );
}
