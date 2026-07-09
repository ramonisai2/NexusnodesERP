import { useMutation, useQuery } from "@tanstack/react-query";
import { useEffect, useMemo, useState } from "react";
import { Link, Navigate, useNavigate } from "react-router-dom";
import { LanguageSwitcher } from "../components/LanguageSwitcher";
import { useLocaleStore } from "../i18n/locale";
import type { MessageKey } from "../i18n/messages";
import { useAuthStore } from "../auth/store";
import type { SessionClaims } from "../auth/policy";

type SetupStatus = {
  needs_setup: boolean;
  profile?: string;
  store_name?: string;
  network_mode?: string;
  can_reinstall?: boolean;
  message?: string;
};

type Preset = {
  id: string;
  name: string;
  barcode: string;
  price: number;
  quantity: number;
  department: string;
  department_name: string;
};

type CustomProduct = {
  name: string;
  price: string;
  quantity: string;
};

type ModuleDef = {
  code: string;
  required: boolean;
  default_on: boolean;
  label_key: string;
  hint_key: string;
};

type NetworkModeDef = {
  code: string;
  default: boolean;
  label_key: string;
  hint_key: string;
  detail_key: string;
  modules_forced_off?: string[];
};

const STEPS = 5;
const INTERNET_MODULES = new Set(["storefront"]);

export function SetupPage() {
  const t = useLocaleStore((s) => s.t);
  const token = useAuthStore((s) => s.token);
  const setSession = useAuthStore((s) => s.setSession);
  const navigate = useNavigate();
  const [step, setStep] = useState(0);
  const [storeName, setStoreName] = useState("");
  const [branchName, setBranchName] = useState("Sucursal principal");
  const [ownerName, setOwnerName] = useState("");
  const [ownerEmail, setOwnerEmail] = useState("");
  const [networkMode, setNetworkMode] = useState<"intranet" | "internet">("intranet");
  const [selected, setSelected] = useState<Record<string, boolean>>({
    leche: true,
    pan: true,
    agua: true,
    arroz: true,
    jabon: true,
  });
  const [modulesOn, setModulesOn] = useState<Record<string, boolean>>({});
  const [modulesReady, setModulesReady] = useState(false);
  const [custom, setCustom] = useState<CustomProduct[]>([{ name: "", price: "", quantity: "" }]);
  const [error, setError] = useState<string | null>(null);
  const [reinstalling, setReinstalling] = useState(false);

  const status = useQuery({
    queryKey: ["setup-status"],
    queryFn: async () => {
      const res = await fetch("/api/setup/status");
      if (!res.ok) throw new Error("status_failed");
      return (await res.json()) as SetupStatus;
    },
  });

  const presets = useQuery({
    queryKey: ["setup-presets"],
    queryFn: async () => {
      const res = await fetch("/api/setup/presets");
      if (!res.ok) throw new Error("presets_failed");
      return (await res.json()) as Preset[];
    },
  });

  const modules = useQuery({
    queryKey: ["setup-modules"],
    queryFn: async () => {
      const res = await fetch("/api/setup/modules");
      if (!res.ok) throw new Error("modules_failed");
      return (await res.json()) as {
        modules: ModuleDef[];
        default_enabled: string[];
        network_modes: NetworkModeDef[];
        default_network: string;
      };
    },
  });

  useEffect(() => {
    if (!modules.data || modulesReady) return;
    const next: Record<string, boolean> = {};
    for (const m of modules.data.modules) {
      next[m.code] = m.required || m.default_on;
    }
    if (modules.data.default_network === "internet") {
      setNetworkMode("internet");
    }
    setModulesOn(next);
    setModulesReady(true);
  }, [modules.data, modulesReady]);

  useEffect(() => {
    if (networkMode !== "intranet") return;
    setModulesOn((prev) => {
      let changed = false;
      const next = { ...prev };
      for (const code of INTERNET_MODULES) {
        if (next[code]) {
          next[code] = false;
          changed = true;
        }
      }
      return changed ? next : prev;
    });
  }, [networkMode]);

  const enabledModuleCodes = useMemo(
    () =>
      Object.entries(modulesOn)
        .filter(([code, on]) => on && !(networkMode === "intranet" && INTERNET_MODULES.has(code)))
        .map(([code]) => code),
    [modulesOn, networkMode],
  );

  const complete = useMutation({
    mutationFn: async (force: boolean) => {
      const preset_ids = Object.entries(selected)
        .filter(([, on]) => on)
        .map(([id]) => id);
      const products = custom
        .filter((p) => p.name.trim())
        .map((p) => ({
          name: p.name.trim(),
          price: Number(p.price) || 0,
          quantity: Number(p.quantity) || 0,
          department: "abarrotes",
        }));
      const res = await fetch("/api/setup/complete", {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({
          store_name: storeName.trim(),
          branch_name: branchName.trim() || "Sucursal principal",
          owner_name: ownerName.trim(),
          owner_email: ownerEmail.trim(),
          currency: "MXN",
          preset_ids,
          products,
          enabled_modules: enabledModuleCodes,
          network_mode: networkMode,
          force,
        }),
      });
      const body = await res.json().catch(() => ({}));
      if (!res.ok) throw new Error(body.error || "setup_failed");
      return body as {
        setup: {
          store_name: string;
          products_created: number;
          owner_sub: string;
          enabled_modules?: string[];
          network_mode?: string;
        };
        access_token?: string;
        claims?: SessionClaims;
      };
    },
    onSuccess: (data) => {
      if (data.access_token && data.claims) {
        setSession(data.access_token, data.claims);
        navigate("/", { replace: true });
      } else {
        navigate("/login", { replace: true });
      }
    },
    onError: (err: Error) => setError(err.message === "already_configured" ? t("setupAlready") : t("setupError")),
  });

  if (status.isLoading) {
    return (
      <main className="login-page setup-page">
        <p className="muted">{t("setupLoading")}</p>
      </main>
    );
  }

  const already = Boolean(status.data && !status.data.needs_setup);
  const showWizard = !already || reinstalling;
  const stepLabels = [
    t("setupStepStore"),
    t("setupStepNetwork"),
    t("setupStepModules"),
    t("setupStepProducts"),
    t("setupStepOwner"),
  ];
  const networkModes = modules.data?.network_modes ?? [
    {
      code: "intranet",
      default: true,
      label_key: "netModeIntranet",
      hint_key: "netModeIntranetHint",
      detail_key: "netModeIntranetDetail",
    },
    {
      code: "internet",
      default: false,
      label_key: "netModeInternet",
      hint_key: "netModeInternetHint",
      detail_key: "netModeInternetDetail",
    },
  ];

  return (
    <main className="login-page setup-page">
      <section className="login-card setup-card">
        <div className="login-card-top">
          <LanguageSwitcher />
        </div>
        <p className="setup-kicker">{t("setupKicker")}</p>
        <h1>{t("setupTitle")}</h1>
        <p className="muted">{t("setupSubtitle")}</p>

        {already && !reinstalling ? (
          <div className="setup-already">
            <p>
              {t("setupAlreadyNamed")} <strong>{status.data?.store_name || "—"}</strong>
            </p>
            <p className="muted tip">{status.data?.message}</p>
            {status.data?.network_mode ? (
              <p className="muted tip">
                {t("setupNetworkCurrent")}:{" "}
                <strong>
                  {status.data.network_mode === "internet" ? t("netModeInternet") : t("netModeIntranet")}
                </strong>
              </p>
            ) : null}
            <div className="setup-actions">
              <Link className="btn secondary" to="/login">
                {t("setupGoLogin")}
              </Link>
              <button
                type="button"
                className="btn"
                onClick={() => {
                  setError(null);
                  setStep(0);
                  setReinstalling(true);
                }}
              >
                {t("setupReinstall")}
              </button>
            </div>
          </div>
        ) : null}

        {showWizard ? (
          <>
            <ol className="setup-steps" aria-label={t("setupStepsLabel")}>
              {stepLabels.map((label, i) => (
                <li key={label} className={i === step ? "active" : i < step ? "done" : ""}>
                  <span>{i + 1}</span>
                  {label}
                </li>
              ))}
            </ol>

            {step === 0 ? (
              <div className="setup-panel">
                <label>
                  <span>{t("setupStoreName")}</span>
                  <input
                    value={storeName}
                    onChange={(e) => setStoreName(e.target.value)}
                    placeholder={t("setupStoreNamePh")}
                    autoFocus
                    required
                  />
                </label>
                <label>
                  <span>{t("setupBranchName")}</span>
                  <input
                    value={branchName}
                    onChange={(e) => setBranchName(e.target.value)}
                    placeholder={t("setupBranchNamePh")}
                  />
                </label>
                <p className="muted tip">{t("setupStoreTip")}</p>
              </div>
            ) : null}

            {step === 1 ? (
              <div className="setup-panel">
                <p className="muted">{t("setupNetworkHint")}</p>
                <div className="network-mode-grid" role="radiogroup" aria-label={t("setupStepNetwork")}>
                  {networkModes.map((mode) => {
                    const selectedMode = networkMode === mode.code;
                    return (
                      <button
                        key={mode.code}
                        type="button"
                        className={`network-mode-card ${mode.code} ${selectedMode ? "on" : ""}`}
                        onClick={() => setNetworkMode(mode.code === "internet" ? "internet" : "intranet")}
                        aria-pressed={selectedMode}
                      >
                        <strong>{t(mode.label_key as MessageKey)}</strong>
                        <span className="muted">{t(mode.hint_key as MessageKey)}</span>
                        <p>{t(mode.detail_key as MessageKey)}</p>
                        <em className="module-state">
                          {selectedMode ? t("setupNetworkSelected") : t("setupNetworkSelect")}
                        </em>
                      </button>
                    );
                  })}
                </div>
                <p className="muted tip">{t("setupNetworkTip")}</p>
              </div>
            ) : null}

            {step === 2 ? (
              <div className="setup-panel">
                <p className="muted">{t("setupModulesHint")}</p>
                {networkMode === "intranet" ? (
                  <p className="muted tip">{t("setupModulesIntranetTip")}</p>
                ) : (
                  <p className="muted tip">{t("setupModulesInternetTip")}</p>
                )}
                <div className="module-grid">
                  {(modules.data?.modules ?? []).map((m) => {
                    const blocked = networkMode === "intranet" && INTERNET_MODULES.has(m.code);
                    const on = blocked ? false : !!modulesOn[m.code];
                    return (
                      <label
                        key={m.code}
                        className={`module-chip ${on ? "on" : "off"} ${m.required ? "required" : ""} ${blocked ? "blocked" : ""}`}
                      >
                        <input
                          type="checkbox"
                          checked={on}
                          disabled={m.required || blocked}
                          onChange={(e) =>
                            setModulesOn((prev) => ({ ...prev, [m.code]: e.target.checked }))
                          }
                        />
                        <div>
                          <strong>{t(m.label_key as MessageKey)}</strong>
                          <span className="muted">{t(m.hint_key as MessageKey)}</span>
                          <em className="module-state">
                            {blocked
                              ? t("setupModuleNeedsInternet")
                              : m.required
                                ? t("setupModuleRequired")
                                : on
                                  ? t("setupModuleOn")
                                  : t("setupModuleOff")}
                          </em>
                        </div>
                      </label>
                    );
                  })}
                </div>
                <p className="muted tip">{t("setupModulesTip")}</p>
              </div>
            ) : null}

            {step === 3 ? (
              <div className="setup-panel">
                <p className="muted">{t("setupProductsHint")}</p>
                <div className="preset-grid">
                  {(presets.data ?? []).map((p) => (
                    <label key={p.id} className={`preset-chip ${selected[p.id] ? "on" : ""}`}>
                      <input
                        type="checkbox"
                        checked={!!selected[p.id]}
                        onChange={(e) => setSelected((s) => ({ ...s, [p.id]: e.target.checked }))}
                      />
                      <strong>{p.name}</strong>
                      <span className="muted">
                        ${p.price.toFixed(2)} · {p.department_name}
                      </span>
                    </label>
                  ))}
                </div>
                <h3>{t("setupCustomProducts")}</h3>
                {custom.map((row, idx) => (
                  <div className="custom-row" key={idx}>
                    <input
                      placeholder={t("setupProductName")}
                      value={row.name}
                      onChange={(e) => {
                        const next = [...custom];
                        next[idx] = { ...row, name: e.target.value };
                        setCustom(next);
                      }}
                    />
                    <input
                      placeholder={t("setupProductPrice")}
                      inputMode="decimal"
                      value={row.price}
                      onChange={(e) => {
                        const next = [...custom];
                        next[idx] = { ...row, price: e.target.value };
                        setCustom(next);
                      }}
                    />
                    <input
                      placeholder={t("setupProductQty")}
                      inputMode="numeric"
                      value={row.quantity}
                      onChange={(e) => {
                        const next = [...custom];
                        next[idx] = { ...row, quantity: e.target.value };
                        setCustom(next);
                      }}
                    />
                  </div>
                ))}
                <button
                  type="button"
                  className="btn secondary"
                  onClick={() => setCustom((c) => [...c, { name: "", price: "", quantity: "" }])}
                >
                  {t("setupAddProduct")}
                </button>
              </div>
            ) : null}

            {step === 4 ? (
              <div className="setup-panel">
                <label>
                  <span>{t("setupOwnerName")}</span>
                  <input value={ownerName} onChange={(e) => setOwnerName(e.target.value)} required />
                </label>
                <label>
                  <span>{t("setupOwnerEmail")}</span>
                  <input
                    type="email"
                    value={ownerEmail}
                    onChange={(e) => setOwnerEmail(e.target.value)}
                    placeholder="dueno@mitienda.mx"
                    required
                  />
                </label>
                <p className="muted tip">{t("setupOwnerTip")}</p>
                <p className="muted tip">
                  {t("setupNetworkSummary")}:{" "}
                  <strong>{networkMode === "internet" ? t("netModeInternet") : t("netModeIntranet")}</strong>
                  {" · "}
                  {t("setupModulesSummary")}: <strong>{enabledModuleCodes.length}</strong>
                </p>
              </div>
            ) : null}

            {error ? <p className="error">{error}</p> : null}

            <div className="setup-actions">
              {step > 0 ? (
                <button type="button" className="btn secondary" onClick={() => setStep((s) => s - 1)}>
                  {t("setupBack")}
                </button>
              ) : (
                <Link className="btn secondary" to="/login">
                  {t("setupGoLogin")}
                </Link>
              )}
              {step < STEPS - 1 ? (
                <button
                  type="button"
                  className="btn"
                  disabled={step === 0 && !storeName.trim()}
                  onClick={() => setStep((s) => s + 1)}
                >
                  {t("setupNext")}
                </button>
              ) : (
                <button
                  type="button"
                  className="btn"
                  disabled={complete.isPending || !ownerName.trim() || !ownerEmail.trim()}
                  onClick={() => {
                    setError(null);
                    complete.mutate(already || reinstalling);
                  }}
                >
                  {complete.isPending ? t("setupSaving") : t("setupFinish")}
                </button>
              )}
            </div>
          </>
        ) : null}
      </section>
    </main>
  );
}

export function SetupRedirect({ children }: { children: React.ReactNode }) {
  const status = useQuery({
    queryKey: ["setup-status"],
    queryFn: async () => {
      const res = await fetch("/api/setup/status");
      if (!res.ok) return { needs_setup: false } as SetupStatus;
      return (await res.json()) as SetupStatus;
    },
    staleTime: 30_000,
  });
  if (status.isLoading) return null;
  if (status.data?.needs_setup) return <Navigate to="/setup" replace />;
  return <>{children}</>;
}
