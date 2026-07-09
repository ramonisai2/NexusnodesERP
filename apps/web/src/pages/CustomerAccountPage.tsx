import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { Link, useNavigate, useParams } from "react-router-dom";
import { useEffect, useState } from "react";
import { LanguageSwitcher } from "../components/LanguageSwitcher";
import { useLocaleStore } from "../i18n/locale";
import { friendlyApiError } from "../i18n/locale";

const CUSTOMER_TOKEN_KEY = "nexus_customer_token";
const CUSTOMER_CLAIMS_KEY = "nexus_customer_claims";

type CustomerCard = {
  id: string;
  card_kind: string;
  card_kind_label?: string;
  card_code: string;
  label?: string;
  status: string;
  created_at: string;
};

type Customer = {
  id: string;
  email: string;
  display_name: string;
  phone?: string;
  status: string;
  cards?: CustomerCard[];
};

type AuthResponse = {
  access_token: string;
  customer: Customer;
  claims?: { attrs?: { customer_id?: string } };
};

function loadCustomerToken(): string {
  return localStorage.getItem(CUSTOMER_TOKEN_KEY) || "";
}

function saveSession(token: string, customer: Customer) {
  localStorage.setItem(CUSTOMER_TOKEN_KEY, token);
  localStorage.setItem(CUSTOMER_CLAIMS_KEY, JSON.stringify(customer));
}

function clearSession() {
  localStorage.removeItem(CUSTOMER_TOKEN_KEY);
  localStorage.removeItem(CUSTOMER_CLAIMS_KEY);
}

async function customerFetch(path: string, init: RequestInit = {}) {
  const token = loadCustomerToken();
  const headers = new Headers(init.headers);
  headers.set("Content-Type", "application/json");
  if (token) headers.set("Authorization", `Bearer ${token}`);
  return fetch(`/api${path}`, { ...init, headers });
}

export function CustomerAccountPage() {
  const { slug = "" } = useParams();
  const t = useLocaleStore((s) => s.t);
  const locale = useLocaleStore((s) => s.locale);
  const navigate = useNavigate();
  const qc = useQueryClient();
  const [mode, setMode] = useState<"login" | "register">("register");
  const [email, setEmail] = useState("");
  const [password, setPassword] = useState("");
  const [name, setName] = useState("");
  const [phone, setPhone] = useState("");
  const [token, setToken] = useState(loadCustomerToken());
  const [cardKind, setCardKind] = useState("BARCODE");
  const [cardCode, setCardCode] = useState("");
  const [cardLabel, setCardLabel] = useState("");

  useEffect(() => {
    setToken(loadCustomerToken());
  }, []);

  const me = useQuery({
    queryKey: ["customer-me", token],
    enabled: Boolean(token),
    queryFn: async () => {
      const res = await customerFetch("/customers/me");
      if (res.status === 401 || res.status === 403) {
        clearSession();
        setToken("");
        throw new Error("session_expired");
      }
      if (!res.ok) throw new Error(await res.text());
      return (await res.json()) as Customer;
    },
    retry: false,
  });

  const auth = useMutation({
    mutationFn: async () => {
      const path = mode === "register" ? "/customers/register" : "/customers/login";
      const body =
        mode === "register"
          ? {
              storefront_slug: slug,
              email: email.trim(),
              password,
              display_name: name.trim(),
              phone: phone.trim(),
            }
          : { storefront_slug: slug, email: email.trim(), password };
      const res = await fetch(`/api${path}`, {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify(body),
      });
      if (!res.ok) throw new Error(await res.text());
      return (await res.json()) as AuthResponse;
    },
    onSuccess: (data) => {
      saveSession(data.access_token, data.customer);
      setToken(data.access_token);
      setPassword("");
      void qc.invalidateQueries({ queryKey: ["customer-me"] });
    },
  });

  const addCard = useMutation({
    mutationFn: async () => {
      const res = await customerFetch("/customers/me/cards", {
        method: "POST",
        body: JSON.stringify({
          card_kind: cardKind,
          card_code: cardCode.trim() || undefined,
          label: cardLabel.trim() || undefined,
        }),
      });
      if (!res.ok) throw new Error(await res.text());
      return res.json();
    },
    onSuccess: () => {
      setCardCode("");
      setCardLabel("");
      void qc.invalidateQueries({ queryKey: ["customer-me"] });
    },
  });

  const blockCard = useMutation({
    mutationFn: async (id: string) => {
      const res = await customerFetch(`/customers/me/cards/${id}/block`, {
        method: "POST",
        body: JSON.stringify({ reason: locale === "en" ? "Blocked by customer" : "Bloqueada por el cliente" }),
      });
      if (!res.ok) throw new Error(await res.text());
      return res.json();
    },
    onSuccess: () => void qc.invalidateQueries({ queryKey: ["customer-me"] }),
  });

  function logout() {
    clearSession();
    setToken("");
    void qc.removeQueries({ queryKey: ["customer-me"] });
  }

  return (
    <main className="customer-account-page">
      <header className="sf-top">
        <div className="sf-brand">
          <Link to={`/tienda/${encodeURIComponent(slug)}`} className="sf-brand-name">
            {t("custBackStore")}
          </Link>
          <p className="sf-tagline">{t("custTitle")}</p>
        </div>
        <LanguageSwitcher />
      </header>

      {!token ? (
        <section className="panel customer-auth">
          <h1>{mode === "register" ? t("custRegister") : t("custLogin")}</h1>
          <p className="muted">{t("custSubtitle")}</p>
          <div className="filter-bar">
            <button type="button" className={`btn ${mode === "register" ? "" : "secondary"}`} onClick={() => setMode("register")}>
              {t("custRegister")}
            </button>
            <button type="button" className={`btn ${mode === "login" ? "" : "secondary"}`} onClick={() => setMode("login")}>
              {t("custLogin")}
            </button>
          </div>
          <form
            className="slip-form"
            onSubmit={(e) => {
              e.preventDefault();
              auth.mutate();
            }}
          >
            {mode === "register" ? (
              <>
                <label className="filter-field block">
                  <span>{t("custName")}</span>
                  <input value={name} onChange={(e) => setName(e.target.value)} required />
                </label>
                <label className="filter-field block">
                  <span>{t("custPhone")}</span>
                  <input value={phone} onChange={(e) => setPhone(e.target.value)} />
                </label>
              </>
            ) : null}
            <label className="filter-field block">
              <span>{t("custEmail")}</span>
              <input type="email" value={email} onChange={(e) => setEmail(e.target.value)} required />
            </label>
            <label className="filter-field block">
              <span>{t("custPassword")}</span>
              <input type="password" value={password} onChange={(e) => setPassword(e.target.value)} minLength={8} required />
            </label>
            <button type="submit" className="btn" disabled={auth.isPending}>
              {auth.isPending ? t("custWorking") : mode === "register" ? t("custRegister") : t("custLogin")}
            </button>
            {auth.isError ? <p className="error">{friendlyApiError((auth.error as Error).message, locale)}</p> : null}
          </form>
        </section>
      ) : (
        <section className="panel customer-dashboard">
          <div className="filter-bar">
            <div>
              <h1>{t("custWelcome")}</h1>
              <p className="muted">
                {me.data?.display_name || "…"} · {me.data?.email}
              </p>
            </div>
            <button type="button" className="btn secondary" onClick={logout}>
              {t("custLogout")}
            </button>
          </div>

          {me.isLoading ? <p className="muted">{t("custLoading")}</p> : null}
          {me.isError ? <p className="error">{t("custSessionError")}</p> : null}

          <h2>{t("custCards")}</h2>
          <p className="muted tip">{t("custCardsTip")}</p>

          <form
            className="slip-form"
            onSubmit={(e) => {
              e.preventDefault();
              addCard.mutate();
            }}
          >
            <div className="filter-bar">
              <label className="filter-field">
                <span>{t("custCardKind")}</span>
                <select value={cardKind} onChange={(e) => setCardKind(e.target.value)}>
                  <option value="BARCODE">{t("custCardBarcode")}</option>
                  <option value="CHIP">{t("custCardChip")}</option>
                </select>
              </label>
              <label className="filter-field grow">
                <span>{t("custCardCode")}</span>
                <input
                  value={cardCode}
                  onChange={(e) => setCardCode(e.target.value)}
                  placeholder={t("custCardCodePh")}
                />
              </label>
              <label className="filter-field">
                <span>{t("custCardLabel")}</span>
                <input value={cardLabel} onChange={(e) => setCardLabel(e.target.value)} />
              </label>
            </div>
            <button type="submit" className="btn" disabled={addCard.isPending}>
              {addCard.isPending ? t("custWorking") : t("custAddCard")}
            </button>
            {addCard.isError ? <p className="error">{friendlyApiError((addCard.error as Error).message, locale)}</p> : null}
            {addCard.isSuccess ? <p className="muted tip">{t("custAddCardOk")}</p> : null}
          </form>

          {(me.data?.cards ?? []).length === 0 ? <p className="muted">{t("custCardsEmpty")}</p> : null}
          {(me.data?.cards ?? []).length > 0 ? (
            <table>
              <thead>
                <tr>
                  <th>{t("custCardKind")}</th>
                  <th>{t("custCardCode")}</th>
                  <th>{t("custCardLabel")}</th>
                  <th>{t("parcelColStatus")}</th>
                  <th />
                </tr>
              </thead>
              <tbody>
                {(me.data?.cards ?? []).map((c) => (
                  <tr key={c.id}>
                    <td>{c.card_kind_label || c.card_kind}</td>
                    <td>
                      <code>{c.card_code}</code>
                    </td>
                    <td>{c.label || "—"}</td>
                    <td>{c.status}</td>
                    <td>
                      {c.status === "ACTIVE" ? (
                        <button type="button" className="btn secondary" onClick={() => blockCard.mutate(c.id)}>
                          {t("custBlockCard")}
                        </button>
                      ) : null}
                    </td>
                  </tr>
                ))}
              </tbody>
            </table>
          ) : null}

          <p className="muted">
            <button type="button" className="btn secondary" onClick={() => navigate(`/tienda/${encodeURIComponent(slug)}`)}>
              {t("custBackStore")}
            </button>
          </p>
        </section>
      )}
    </main>
  );
}
