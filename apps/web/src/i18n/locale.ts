import { create } from "zustand";
import { messages, type Locale, type MessageKey } from "./messages";

const LOCALE_KEY = "nexus_locale";

function detectLocale(): Locale {
  const saved = localStorage.getItem(LOCALE_KEY);
  if (saved === "es" || saved === "en") return saved;
  const nav = navigator.language?.toLowerCase() ?? "es";
  return nav.startsWith("en") ? "en" : "es";
}

type LocaleState = {
  locale: Locale;
  setLocale: (locale: Locale) => void;
  t: (key: MessageKey) => string;
};

export const useLocaleStore = create<LocaleState>((set, get) => ({
  locale: typeof window === "undefined" ? "es" : detectLocale(),
  setLocale: (locale) => {
    localStorage.setItem(LOCALE_KEY, locale);
    document.documentElement.lang = locale;
    set({ locale });
  },
  t: (key) => messages[get().locale][key] ?? messages.es[key] ?? key,
}));

export function t(key: MessageKey, locale?: Locale): string {
  const loc = locale ?? useLocaleStore.getState().locale;
  return messages[loc][key] ?? messages.es[key] ?? key;
}

const ROLE_KEYS: Record<string, MessageKey> = {
  inventory_clerk: "roleInventoryClerk",
  payroll_analyst: "rolePayrollAnalyst",
  payroll_approver: "rolePayrollApprover",
  platform_admin: "rolePlatformAdmin",
  warehouse_manager: "roleWarehouseManager",
  store_owner: "roleStoreOwner",
  regional_manager: "roleRegionalManager",
};

const PERM_KEYS: Record<string, MessageKey> = {
  "inventory.balance.read": "permInventoryBalanceRead",
  "inventory.movement.create": "permInventoryMovementCreate",
  "inventory.movement.read": "permInventoryMovementRead",
  "inventory.movement.void": "permInventoryMovementVoid",
  "inventory.catalog.read": "permInventoryCatalogRead",
  "inventory.label.read": "permInventoryLabelRead",
  "reporting.read": "permReportingRead",
  "reporting.image.read": "permReportingImageRead",
  "reporting.image.create": "permReportingImageCreate",
  "store.setup.read": "permStoreSetupRead",
  "payroll.run.prepare": "permPayrollRunPrepare",
  "payroll.run.approve": "permPayrollRunApprove",
  "payroll.run.read": "permPayrollRunRead",
};

const STATUS_KEYS: Record<string, MessageKey> = {
  IN_REVIEW: "statusInReview",
  APPROVED: "statusApproved",
  OPEN: "statusOpen",
  CLOSED: "statusClosed",
  PAID: "statusPaid",
  POSTED: "statusPosted",
  VOID: "statusVoid",
};

const BRANCH_KEYS: Record<string, MessageKey> = {
  br_norte: "branchNorte",
  br_sur: "branchSur",
};

const ORG_KEYS: Record<string, MessageKey> = {
  org_demo: "orgDemo",
  DEMO: "orgDemo",
};

export function labelRole(code: string, locale?: Locale): string {
  const key = ROLE_KEYS[code];
  return key ? t(key, locale) : code;
}

export function labelPermission(code: string, locale?: Locale): string {
  const key = PERM_KEYS[code];
  return key ? t(key, locale) : code;
}

export function labelStatus(code: string, locale?: Locale): string {
  const key = STATUS_KEYS[code];
  return key ? t(key, locale) : t("statusUnknown", locale);
}

export function labelBranch(code: string, locale?: Locale): string {
  const key = BRANCH_KEYS[code];
  return key ? t(key, locale) : code;
}

export function labelOrg(code: string, locale?: Locale): string {
  const key = ORG_KEYS[code];
  return key ? t(key, locale) : code;
}

export function labelUser(sub: string): string {
  const map: Record<string, string> = {
    usr_dev_analyst: "Analista",
    usr_dev_approver: "Aprobador",
    usr_dev_dual: "Dual",
    usr_dev_admin: "Admin",
    usr_dev_wh_manager: "Jefe almacén",
    usr_dev_regional: "Jefe regional",
  };
  const en: Record<string, string> = {
    usr_dev_analyst: "Analyst",
    usr_dev_approver: "Approver",
    usr_dev_dual: "Dual",
    usr_dev_admin: "Admin",
    usr_dev_wh_manager: "Warehouse mgr",
    usr_dev_regional: "Regional mgr",
  };
  const locale = useLocaleStore.getState().locale;
  return (locale === "en" ? en[sub] : map[sub]) ?? sub.replace(/^usr_dev_/, "");
}

export function friendlyApiError(raw: string, locale?: Locale): string {
  const loc = locale ?? useLocaleStore.getState().locale;
  try {
    const parsed = JSON.parse(raw) as { error?: string; action?: string };
    if (parsed.error === "sod_violation") return t("paySodError", loc);
    if (parsed.error === "forbidden" && parsed.action === "payroll.run.approve") {
      return t("paySodError", loc);
    }
    if (parsed.error === "forbidden" && parsed.action === "inventory.movement.void") {
      return t("invVoidError", loc);
    }
    if (parsed.error === "already_voided") return t("invAlreadyVoided", loc);
    if (parsed.error === "forbidden") return t("payForbiddenAction", loc);
    if (parsed.error === "version_conflict" || parsed.error === "insufficient_stock") {
      return t("invMoveError", loc);
    }
  } catch {
    // plain text body
  }
  if (raw.includes("sod_violation")) return t("paySodError", loc);
  if (raw.includes("already_voided")) return t("invAlreadyVoided", loc);
  if (raw.includes("version_conflict") || raw.includes("insufficient_stock")) {
    return t("invMoveError", loc);
  }
  if (raw.includes("inventory.movement.void")) return t("invVoidError", loc);
  if (raw.includes("forbidden")) return t("payForbiddenAction", loc);
  return t("payError", loc);
}
