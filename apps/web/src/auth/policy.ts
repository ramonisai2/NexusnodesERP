export type SessionClaims = {
  sub: string;
  org_id: string;
  branch_ids: string[];
  roles: string[];
  permissions: string[];
  attrs: Record<string, unknown>;
  amr: string[];
  sid: string;
  operator_label?: string;
  session_id?: string;
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
    id: "nav.search",
    label: "Buscar",
    path: "/search",
    require: {
      permissions: ["inventory.balance.read", "inventory.catalog.read", "reporting.image.read", "search.query"],
      anyBranch: true,
      minAmrCount: 1,
    },
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
    id: "nav.receiving",
    label: "Recepción",
    path: "/inventory/receiving",
    require: {
      permissions: ["inventory.receipt.create", "inventory.movement.create", "inventory.balance.read"],
      anyBranch: true,
      minAmrCount: 1,
    },
  },
  {
    id: "nav.slips",
    label: "Papeletas",
    path: "/inventory/slips",
    require: {
      permissions: ["inventory.slip.read", "inventory.slip.create", "inventory.balance.read"],
      anyBranch: true,
      minAmrCount: 1,
    },
  },
  {
    id: "nav.transport",
    label: "Hojas de transporte",
    path: "/inventory/transport",
    require: {
      permissions: ["inventory.transport.read", "inventory.transport.create", "inventory.balance.read"],
      anyBranch: true,
      minAmrCount: 1,
    },
  },
  {
    id: "nav.transfers",
    label: "Traslados",
    path: "/inventory/transfers",
    require: {
      permissions: ["inventory.transfer.read", "inventory.balance.read"],
      anyBranch: true,
      minAmrCount: 1,
    },
  },
  {
    id: "nav.parcels",
    label: "Paquetería",
    path: "/inventory/parcels",
    require: {
      permissions: ["inventory.parcel.read", "inventory.slip.read", "inventory.balance.read"],
      anyBranch: true,
      minAmrCount: 1,
    },
  },
  {
    id: "nav.storefront",
    label: "Tienda en línea",
    path: "/settings/tienda",
    require: {
      permissions: ["store.storefront.manage", "store.storefront.read"],
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
  {
    id: "nav.imageReports",
    label: "Fotos",
    path: "/reports/images",
    require: {
      permissions: ["reporting.image.read", "reporting.image.create", "inventory.balance.read"],
      anyBranch: true,
      minAmrCount: 1,
    },
  },
  {
    id: "nav.approvals",
    label: "Aprobaciones",
    path: "/approvals",
    require: {
      permissions: ["approval.read", "approval.decide"],
      anyBranch: true,
      minAmrCount: 1,
    },
  },
  {
    id: "nav.mail",
    label: "Correo",
    path: "/mail",
    require: {
      permissions: ["mail.read", "mail.send", "inventory.balance.read"],
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
