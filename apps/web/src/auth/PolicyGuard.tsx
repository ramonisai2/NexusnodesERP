import type { ReactNode } from "react";
import { Navigate, Outlet } from "react-router-dom";
import { canAccess, type NavNode } from "./policy";
import { useAuthStore } from "./store";

export function RequireAuth() {
  const token = useAuthStore((s) => s.token);
  if (!token) return <Navigate to="/login" replace />;
  return <Outlet />;
}

export function PolicyGuard({
  node,
  children,
  fallback = null,
}: {
  node: NavNode;
  children: ReactNode;
  fallback?: ReactNode;
}) {
  const claims = useAuthStore((s) => s.claims);
  if (!canAccess(claims, node)) return <>{fallback}</>;
  return <>{children}</>;
}
