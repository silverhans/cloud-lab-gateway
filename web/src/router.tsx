import {
  createRootRoute,
  createRoute,
  createRouter,
  Navigate,
  Outlet,
} from "@tanstack/react-router";
import { AppShell } from "@/components/app-shell";
import { useCurrentUser } from "@/hooks/use-current-user";
import { LoginPage } from "@/pages/login-page";
import { StudentPage } from "@/pages/student-page";
import { TeacherPage } from "@/pages/teacher-page";
import { AdminPage } from "@/pages/admin-page";

const rootRoute = createRootRoute({
  component: () => <Outlet />,
});

const indexRoute = createRoute({
  getParentRoute: () => rootRoute,
  path: "/",
  component: function Index() {
    const { data: user, isLoading } = useCurrentUser();
    if (isLoading) return null;
    if (!user) return <Navigate to="/login" />;
    switch (user.role) {
      case "student":
        return <Navigate to="/student" />;
      case "teacher":
        return <Navigate to="/teacher" />;
      case "admin":
        return <Navigate to="/admin" />;
    }
  },
});

const loginRoute = createRoute({
  getParentRoute: () => rootRoute,
  path: "/login",
  component: LoginPage,
});

/**
 * AuthenticatedShell is a layout route that wraps role pages with the
 * AppShell (sidebar + topbar) and a basic auth guard. Children render
 * inside <Outlet/>.
 */
const authenticatedRoute = createRoute({
  getParentRoute: () => rootRoute,
  id: "authenticated",
  component: function Authenticated() {
    const { data: user, isLoading } = useCurrentUser();
    if (isLoading) return null;
    if (!user) return <Navigate to="/login" />;
    return (
      <AppShell user={user}>
        <Outlet />
      </AppShell>
    );
  },
});

const studentRoute = createRoute({
  getParentRoute: () => authenticatedRoute,
  path: "/student",
  component: StudentPage,
});

const teacherRoute = createRoute({
  getParentRoute: () => authenticatedRoute,
  path: "/teacher",
  component: TeacherPage,
});

const adminRoute = createRoute({
  getParentRoute: () => authenticatedRoute,
  path: "/admin",
  component: AdminPage,
});

const routeTree = rootRoute.addChildren([
  indexRoute,
  loginRoute,
  authenticatedRoute.addChildren([studentRoute, teacherRoute, adminRoute]),
]);

export const router = createRouter({
  routeTree,
  defaultPreload: "intent",
  defaultPreloadStaleTime: 0,
});
