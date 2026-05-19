import { Badge } from "@/components/ui/badge";
import type { Role } from "@/lib/types";

const labels: Record<Role, string> = {
  student: "Студент",
  teacher: "Преподаватель",
  admin: "Администратор",
};

export function RoleBadge({ role }: { role: Role }): JSX.Element {
  return <Badge variant={role === "admin" ? "warning" : role === "teacher" ? "violet" : "secondary"}>{labels[role]}</Badge>;
}
