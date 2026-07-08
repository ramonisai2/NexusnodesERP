export type SessionClaims = {
  sub: string;
  org_id: string;
  branch_ids: string[];
  roles: string[];
  permissions: string[];
  attrs: Record<string, unknown>;
  amr: string[];
  sid: string;
};

export type NavNode = {
  id: string;
  label: string;
  path: string;
  require: {
    permissions: string[];
    anyBranch?: boolean;
    minAmrCount?: number;
  };
};

export const NAV_NODES: NavNode[] = [
  {
    id: "nav.dashboard",
    label: "Inicio",
    path: "/",
    require: { permissions: [] },
  },
  {
    id: "nav.inventory",
    label: "Inventario",
    path: "/inventory",
    require: {
      permissions: ["inventory.balance.read"],
      anyBranch: true,
      minAmrCount: 1,
    },
  },
  {
    id: "nav.payroll",
    label: "Nómina",
    path: "/payroll",
    require: {
      permissions: ["payroll.run.read"],
      anyBranch: true,
      minAmrCount: 1,
    },
  },
  {
    id: "nav.reports",
    label: "Reportes",
    path: "/reports",
    require: {
      permissions: ["inventory.balance.read", "payroll.run.read", "reporting.read"],
      anyBranch: true,
      minAmrCount: 1,
    },
  },
];

export function canAccess(claims: SessionClaims | null, node: NavNode): boolean {
  if (!claims) return false;
  if (claims.roles.includes("platform_admin")) return true;
  const { permissions, anyBranch, minAmrCount } = node.require;
  if (permissions.length > 0) {
    const ok = permissions.some((p) => claims.permissions.includes(p));
    if (!ok) return false;
  }
  if (anyBranch && claims.branch_ids.length === 0) return false;
  if ((minAmrCount ?? 0) > (claims.amr?.length ?? 0)) return false;
  return true;
}

export function hasPermission(claims: SessionClaims | null, code: string): boolean {
  if (!claims) return false;
  if (claims.roles.includes("platform_admin")) return true;
  return claims.permissions.includes(code);
}
