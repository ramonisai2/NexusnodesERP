import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { useState } from "react";
import { hasPermission } from "../auth/policy";
import { PolicyGuard } from "../auth/PolicyGuard";
import { apiFetch, useAuthStore } from "../auth/store";
import { friendlyApiError, useLocaleStore } from "../i18n/locale";

type CustomerCard = {
  id: string;
  card_kind: string;
  card_kind_label?: string;
  card_code: string;
  label?: string;
  status: string;
};

type Customer = {
  id: string;
  email: string;
  display_name: string;
  phone?: string;
  status: string;
  cards?: CustomerCard[];
};

type LookupResult = {
  card: CustomerCard;
  customer: Customer;
};

const customersNode = {
  id: "nav.customers",
  label: "Clientes",
  path: "/customers",
  require: {
    permissions: ["customer.read", "customer.card.read", "inventory.balance.read"],
    anyBranch: true,
    minAmrCount: 1,
  },
};

export function CustomersAdminPage() {
  const t = useLocaleStore((s) => s.t);
  return (
    <PolicyGuard
      node={customersNode}
      fallback={
        <section className="panel">
          <h1>{t("custAdminTitle")}</h1>
          <p className="error">{t("custAdminForbidden")}</p>
        </section>
      }
    >
      <CustomersAdminPanel />
    </PolicyGuard>
  );
}

function CustomersAdminPanel() {
  const t = useLocaleStore((s) => s.t);
  const locale = useLocaleStore((s) => s.locale);
  const claims = useAuthStore((s) => s.claims);
  const qc = useQueryClient();
  const canManage =
    hasPermission(claims, "customer.card.manage") ||
    hasPermission(claims, "customer.manage") ||
    hasPermission(claims, "inventory.movement.create");

  const [q, setQ] = useState("");
  const [selectedId, setSelectedId] = useState("");
  const [lookupCode, setLookupCode] = useState("");
  const [lookup, setLookup] = useState<LookupResult | null>(null);
  const [cardKind, setCardKind] = useState("BARCODE");
  const [cardCode, setCardCode] = useState("");
  const [cardLabel, setCardLabel] = useState("");

  const list = useQuery({
    queryKey: ["customers-admin", q],
    queryFn: async () => {
      const params = new URLSearchParams();
      if (q.trim()) params.set("q", q.trim());
      const res = await apiFetch(`/customers?${params}`);
      if (!res.ok) throw new Error("list_failed");
      return (await res.json()) as Customer[];
    },
  });

  const detail = useQuery({
    queryKey: ["customer-admin", selectedId],
    enabled: Boolean(selectedId),
    queryFn: async () => {
      const res = await apiFetch(`/customers/${selectedId}`);
      if (!res.ok) throw new Error("detail_failed");
      return (await res.json()) as Customer;
    },
  });

  const issueCard = useMutation({
    mutationFn: async () => {
      const res = await apiFetch(`/customers/${selectedId}/cards`, {
        method: "POST",
        headers: { "Content-Type": "application/json" },
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
      void qc.invalidateQueries({ queryKey: ["customer-admin", selectedId] });
      void qc.invalidateQueries({ queryKey: ["customers-admin"] });
    },
  });

  const blockCard = useMutation({
    mutationFn: async (id: string) => {
      const res = await apiFetch(`/customers/cards/${id}/block`, {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({ reason: "Bloqueada en tienda" }),
      });
      if (!res.ok) throw new Error(await res.text());
      return res.json();
    },
    onSuccess: () => {
      void qc.invalidateQueries({ queryKey: ["customer-admin", selectedId] });
    },
  });

  const doLookup = useMutation({
    mutationFn: async () => {
      const res = await apiFetch(`/customers/cards/lookup?code=${encodeURIComponent(lookupCode.trim())}`);
      if (!res.ok) throw new Error(await res.text());
      return (await res.json()) as LookupResult;
    },
    onSuccess: (data) => {
      setLookup(data);
      setSelectedId(data.customer.id);
    },
  });

  return (
    <section className="panel customers-admin-page">
      <h1>{t("custAdminTitle")}</h1>
      <p className="muted">{t("custAdminSubtitle")}</p>

      <form
        className="filter-bar"
        onSubmit={(e) => {
          e.preventDefault();
          doLookup.mutate();
        }}
      >
        <label className="filter-field grow">
          <span>{t("custLookup")}</span>
          <input
            value={lookupCode}
            onChange={(e) => setLookupCode(e.target.value)}
            placeholder={t("custLookupPh")}
          />
        </label>
        <button type="submit" className="btn" disabled={doLookup.isPending || !lookupCode.trim()}>
          {t("custLookupBtn")}
        </button>
      </form>
      {doLookup.isError ? <p className="error">{friendlyApiError((doLookup.error as Error).message, locale)}</p> : null}
      {lookup ? (
        <p className="muted tip">
          {lookup.customer.display_name} · {lookup.card.card_kind_label || lookup.card.card_kind} ·{" "}
          <code>{lookup.card.card_code}</code> · {lookup.card.status}
        </p>
      ) : null}

      <div className="filter-bar">
        <label className="filter-field grow">
          <span>{t("custSearch")}</span>
          <input value={q} onChange={(e) => setQ(e.target.value)} placeholder={t("custSearchPh")} />
        </label>
      </div>

      {list.isLoading ? <p className="muted">{t("custLoading")}</p> : null}
      {(list.data ?? []).length === 0 && !list.isLoading ? <p className="muted">{t("custAdminEmpty")}</p> : null}

      <div className="transfer-layout">
        <div className="transfer-list">
          {(list.data ?? []).map((c) => (
            <button
              key={c.id}
              type="button"
              className={`transfer-list-item${selectedId === c.id ? " active" : ""}`}
              onClick={() => setSelectedId(c.id)}
            >
              <strong>{c.display_name}</strong>
              <span className="muted">{c.email}</span>
            </button>
          ))}
        </div>

        <div>
          {detail.data ? (
            <article className="transfer-detail">
              <h2>{detail.data.display_name}</h2>
              <p className="muted">
                {detail.data.email}
                {detail.data.phone ? ` · ${detail.data.phone}` : ""}
              </p>
              <p>
                {t("parcelColStatus")}: {detail.data.status}
              </p>

              <h3>{t("custCards")}</h3>
              {(detail.data.cards ?? []).length === 0 ? <p className="muted">{t("custCardsEmpty")}</p> : null}
              {(detail.data.cards ?? []).map((card) => (
                <div key={card.id} className="filter-bar">
                  <span>
                    {card.card_kind_label || card.card_kind} · <code>{card.card_code}</code> · {card.status}
                  </span>
                  {canManage && card.status === "ACTIVE" ? (
                    <button type="button" className="btn secondary" onClick={() => blockCard.mutate(card.id)}>
                      {t("custBlockCard")}
                    </button>
                  ) : null}
                </div>
              ))}

              {canManage ? (
                <form
                  className="slip-form"
                  onSubmit={(e) => {
                    e.preventDefault();
                    issueCard.mutate();
                  }}
                >
                  <h3>{t("custIssueCard")}</h3>
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
                      <input value={cardCode} onChange={(e) => setCardCode(e.target.value)} placeholder={t("custCardCodePh")} />
                    </label>
                    <label className="filter-field">
                      <span>{t("custCardLabel")}</span>
                      <input value={cardLabel} onChange={(e) => setCardLabel(e.target.value)} />
                    </label>
                  </div>
                  <button type="submit" className="btn" disabled={issueCard.isPending}>
                    {issueCard.isPending ? t("custWorking") : t("custIssueCard")}
                  </button>
                  {issueCard.isError ? (
                    <p className="error">{friendlyApiError((issueCard.error as Error).message, locale)}</p>
                  ) : null}
                </form>
              ) : null}
            </article>
          ) : (
            <p className="muted">{t("custAdminSelect")}</p>
          )}
        </div>
      </div>
    </section>
  );
}
