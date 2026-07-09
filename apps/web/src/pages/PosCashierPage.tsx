import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { useMemo, useState } from "react";
import { hasPermission } from "../auth/policy";
import { PolicyGuard } from "../auth/PolicyGuard";
import { apiFetch, useAuthStore } from "../auth/store";
import { friendlyApiError, useLocaleStore } from "../i18n/locale";

type Label = {
  sku: string;
  barcode?: string;
  public_description?: string;
  effective_price?: number | null;
  currency?: string;
  on_hand?: number | null;
};

type CartLine = {
  sku: string;
  description: string;
  quantity: number;
  unit_price: number;
};

type FiscalSettings = {
  legal_name: string;
  trade_name: string;
  rfc: string;
  tax_regime: string;
  postal_code: string;
  prices_include_tax: boolean;
  default_tax_rate: number;
  receipt_footer: string;
};

type Sale = {
  id: string;
  ticket_number: string;
  status: string;
  subtotal: number;
  tax_total: number;
  grand_total: number;
  tax_rate: number;
  prices_include_tax: boolean;
  customer_name?: string;
  completed_at?: string;
  lines?: Array<{
    sku: string;
    description: string;
    quantity: number;
    unit_price: number;
    line_total: number;
    tax_amount: number;
    base_amount: number;
  }>;
  payments?: Array<{
    method: string;
    method_label?: string;
    amount: number;
    received_amount?: number;
    change_amount: number;
  }>;
  invoice?: {
    status: string;
    status_label?: string;
    series: string;
    folio?: number;
    rfc_receiver: string;
    legal_name_receiver: string;
    uso_cfdi: string;
    notes?: string;
  };
  fiscal?: FiscalSettings;
  receipt_hint?: string;
};

const posNode = {
  id: "nav.pos",
  label: "Caja",
  path: "/caja",
  require: {
    permissions: ["pos.sale.create", "pos.sale.read", "inventory.balance.read"],
    anyBranch: true,
    minAmrCount: 1,
  },
};

function money(n: number, currency = "MXN") {
  return new Intl.NumberFormat("es-MX", { style: "currency", currency }).format(n);
}

function splitInclusive(gross: number, rate: number) {
  if (rate <= 0) return { base: gross, tax: 0 };
  const base = Math.round((gross / (1 + rate)) * 100) / 100;
  const tax = Math.round((gross - base) * 100) / 100;
  return { base, tax };
}

export function PosCashierPage() {
  const t = useLocaleStore((s) => s.t);
  return (
    <PolicyGuard
      node={posNode}
      fallback={
        <section className="panel">
          <h1>{t("posTitle")}</h1>
          <p className="error">{t("posForbidden")}</p>
        </section>
      }
    >
      <PosCashierPanel />
    </PolicyGuard>
  );
}

function PosCashierPanel() {
  const t = useLocaleStore((s) => s.t);
  const locale = useLocaleStore((s) => s.locale);
  const claims = useAuthStore((s) => s.claims);
  const branchId = useAuthStore((s) => s.activeBranchId) || claims?.branch_ids?.[0] || "";
  const canSell = hasPermission(claims, "pos.sale.create") || hasPermission(claims, "inventory.movement.create");
  const qc = useQueryClient();

  const [scan, setScan] = useState("");
  const [cart, setCart] = useState<CartLine[]>([]);
  const [payMethod, setPayMethod] = useState("CASH");
  const [received, setReceived] = useState("");
  const [wantInvoice, setWantInvoice] = useState(false);
  const [invRfc, setInvRfc] = useState("");
  const [invName, setInvName] = useState("");
  const [invEmail, setInvEmail] = useState("");
  const [invUso, setInvUso] = useState("G03");
  const [error, setError] = useState("");
  const [lastSale, setLastSale] = useState<Sale | null>(null);

  const fiscal = useQuery({
    queryKey: ["pos-fiscal", branchId],
    enabled: Boolean(branchId),
    queryFn: async () => {
      const res = await apiFetch(`/pos/fiscal-settings?branch_id=${encodeURIComponent(branchId)}`);
      if (!res.ok) throw new Error(await res.text());
      return (await res.json()) as FiscalSettings;
    },
  });

  const history = useQuery({
    queryKey: ["pos-sales", branchId],
    enabled: Boolean(branchId),
    queryFn: async () => {
      const res = await apiFetch(`/pos/sales?branch_id=${encodeURIComponent(branchId)}&limit=12`);
      if (!res.ok) throw new Error(await res.text());
      return (await res.json()) as { items: Sale[] };
    },
  });

  const rate = fiscal.data?.default_tax_rate ?? 0.16;
  const includeTax = fiscal.data?.prices_include_tax ?? true;

  const totals = useMemo(() => {
    const grand = Math.round(cart.reduce((s, l) => s + l.unit_price * l.quantity, 0) * 100) / 100;
    if (includeTax) {
      const { base, tax } = splitInclusive(grand, rate);
      return { grand, base, tax };
    }
    const tax = Math.round(grand * rate * 100) / 100;
    return { grand: Math.round((grand + tax) * 100) / 100, base: grand, tax };
  }, [cart, includeTax, rate]);

  const receivedNum = Number(received.replace(",", ".")) || 0;
  const change = payMethod === "CASH" && receivedNum > totals.grand ? Math.round((receivedNum - totals.grand) * 100) / 100 : 0;

  const addSku = async (code: string) => {
    const q = code.trim();
    if (!q) return;
    setError("");
    const res = await apiFetch(
      `/inventory/labels?branch_id=${encodeURIComponent(branchId)}&sku=${encodeURIComponent(q)}`,
    );
    if (!res.ok) {
      // try barcode via list filter — labels API uses sku param; also try raw search
      const res2 = await apiFetch(`/inventory/labels?branch_id=${encodeURIComponent(branchId)}`);
      if (!res2.ok) {
        setError(t("posSkuNotFound"));
        return;
      }
      const all = (await res2.json()) as Label[];
      const hit = all.find((l) => l.sku === q || l.barcode === q);
      if (!hit || hit.effective_price == null) {
        setError(t("posSkuNotFound"));
        return;
      }
      pushLine(hit);
      setScan("");
      return;
    }
    const labels = (await res.json()) as Label[];
    const hit = Array.isArray(labels)
      ? labels.find((l) => l.sku === q || l.barcode === q) || labels[0]
      : null;
    if (!hit || hit.effective_price == null) {
      setError(t("posSkuNotFound"));
      return;
    }
    pushLine(hit);
    setScan("");
  };

  const pushLine = (hit: Label) => {
    setCart((prev) => {
      const idx = prev.findIndex((l) => l.sku === hit.sku);
      if (idx >= 0) {
        const next = [...prev];
        next[idx] = { ...next[idx], quantity: next[idx].quantity + 1 };
        return next;
      }
      return [
        ...prev,
        {
          sku: hit.sku,
          description: hit.public_description || hit.sku,
          quantity: 1,
          unit_price: Number(hit.effective_price),
        },
      ];
    });
  };

  const complete = useMutation({
    mutationFn: async () => {
      if (cart.length === 0) throw new Error(t("posEmptyCart"));
      if (wantInvoice && (!invRfc.trim() || !invName.trim())) {
        throw new Error(t("posInvoiceRequired"));
      }
      const amount = totals.grand;
      const payload = {
        branch_id: branchId,
        lines: cart.map((l) => ({ sku: l.sku, quantity: l.quantity })),
        payments: [
          {
            method: payMethod,
            amount,
            received_amount: payMethod === "CASH" ? receivedNum || amount : undefined,
          },
        ],
        request_invoice: wantInvoice,
        invoice_rfc: invRfc,
        invoice_name: invName,
        invoice_email: invEmail,
        invoice_uso_cfdi: invUso,
        idempotency_key: `pos-${branchId}-${Date.now()}-${Math.random().toString(36).slice(2, 8)}`,
      };
      const res = await apiFetch("/pos/sales/complete", {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify(payload),
      });
      if (!res.ok) throw new Error(await res.text());
      return (await res.json()) as Sale;
    },
    onSuccess: (sale) => {
      setLastSale(sale);
      setCart([]);
      setReceived("");
      setWantInvoice(false);
      setInvRfc("");
      setInvName("");
      setInvEmail("");
      setError("");
      void qc.invalidateQueries({ queryKey: ["pos-sales"] });
      setTimeout(() => window.print(), 300);
    },
    onError: (err: Error) => setError(friendlyApiError(err.message, locale) || t("posSaleError")),
  });

  return (
    <section className="panel pos-page">
      <header className="transfer-header">
        <div>
          <h1>{t("posTitle")}</h1>
          <p className="muted">{t("posSubtitle")}</p>
        </div>
        {fiscal.data ? (
          <div className="pos-fiscal-chip muted">
            <div>
              <strong>{fiscal.data.trade_name || fiscal.data.legal_name}</strong>
            </div>
            <div>
              RFC {fiscal.data.rfc || "—"} · IVA {(rate * 100).toFixed(0)}%
              {includeTax ? ` (${t("posTaxIncluded")})` : ""}
            </div>
          </div>
        ) : null}
      </header>

      {error ? <p className="error">{error}</p> : null}

      <div className="pos-layout">
        <div className="pos-cart">
          <form
            className="pos-scan"
            onSubmit={(e) => {
              e.preventDefault();
              void addSku(scan);
            }}
          >
            <label>
              {t("posScan")}
              <input
                autoFocus
                value={scan}
                onChange={(e) => setScan(e.target.value)}
                placeholder={t("posScanPh")}
              />
            </label>
            <button type="submit" className="btn" disabled={!canSell}>
              {t("posAdd")}
            </button>
          </form>

          <table className="data-table">
            <thead>
              <tr>
                <th>{t("posItem")}</th>
                <th>{t("posQty")}</th>
                <th>{t("posPrice")}</th>
                <th>{t("posLine")}</th>
                <th />
              </tr>
            </thead>
            <tbody>
              {cart.map((l) => (
                <tr key={l.sku}>
                  <td>
                    <strong>{l.description}</strong>
                    <div className="muted">
                      <code>{l.sku}</code>
                    </div>
                  </td>
                  <td>
                    <input
                      className="pos-qty"
                      type="number"
                      min={1}
                      step={1}
                      value={l.quantity}
                      onChange={(e) => {
                        const q = Math.max(1, Number(e.target.value) || 1);
                        setCart((prev) => prev.map((x) => (x.sku === l.sku ? { ...x, quantity: q } : x)));
                      }}
                    />
                  </td>
                  <td>{money(l.unit_price)}</td>
                  <td>{money(l.unit_price * l.quantity)}</td>
                  <td>
                    <button
                      type="button"
                      className="btn secondary"
                      onClick={() => setCart((prev) => prev.filter((x) => x.sku !== l.sku))}
                    >
                      {t("posRemove")}
                    </button>
                  </td>
                </tr>
              ))}
              {cart.length === 0 ? (
                <tr>
                  <td colSpan={5} className="muted">
                    {t("posEmptyCart")}
                  </td>
                </tr>
              ) : null}
            </tbody>
          </table>
        </div>

        <aside className="pos-checkout">
          <h2>{t("posPay")}</h2>
          <div className="pos-totals">
            <div>
              <span>{t("posSubtotal")}</span>
              <strong>{money(totals.base)}</strong>
            </div>
            <div>
              <span>
                {t("posIva")} ({(rate * 100).toFixed(0)}%)
              </span>
              <strong>{money(totals.tax)}</strong>
            </div>
            <div className="pos-grand">
              <span>{t("posTotal")}</span>
              <strong>{money(totals.grand)}</strong>
            </div>
          </div>

          <label>
            {t("posMethod")}
            <select value={payMethod} onChange={(e) => setPayMethod(e.target.value)}>
              <option value="CASH">{t("posCash")}</option>
              <option value="CARD">{t("posCard")}</option>
              <option value="TRANSFER">{t("posTransfer")}</option>
            </select>
          </label>

          {payMethod === "CASH" ? (
            <label>
              {t("posReceived")}
              <input value={received} onChange={(e) => setReceived(e.target.value)} placeholder={String(totals.grand)} />
            </label>
          ) : null}
          {payMethod === "CASH" && change > 0 ? (
            <p className="ok">
              {t("posChange")}: {money(change)}
            </p>
          ) : null}

          <label className="pos-check">
            <input type="checkbox" checked={wantInvoice} onChange={(e) => setWantInvoice(e.target.checked)} />
            {t("posWantInvoice")}
          </label>

          {wantInvoice ? (
            <div className="pos-invoice-fields">
              <label>
                RFC
                <input value={invRfc} onChange={(e) => setInvRfc(e.target.value.toUpperCase())} maxLength={13} />
              </label>
              <label>
                {t("posInvoiceName")}
                <input value={invName} onChange={(e) => setInvName(e.target.value)} />
              </label>
              <label>
                {t("posInvoiceEmail")}
                <input value={invEmail} onChange={(e) => setInvEmail(e.target.value)} />
              </label>
              <label>
                {t("posUsoCfdi")}
                <select value={invUso} onChange={(e) => setInvUso(e.target.value)}>
                  <option value="G01">G01 — Adquisición de mercancías</option>
                  <option value="G03">G03 — Gastos en general</option>
                  <option value="S01">S01 — Sin efectos fiscales</option>
                </select>
              </label>
              <p className="muted tip">{t("posInvoiceTip")}</p>
            </div>
          ) : null}

          <button
            type="button"
            className="btn"
            disabled={!canSell || cart.length === 0 || complete.isPending}
            onClick={() => complete.mutate()}
          >
            {complete.isPending ? t("posCharging") : t("posCharge")}
          </button>
        </aside>
      </div>

      {lastSale ? <ReceiptView sale={lastSale} t={t} /> : null}

      <h2 style={{ marginTop: "2rem" }}>{t("posHistory")}</h2>
      <ul className="plain-list">
        {(history.data?.items || []).map((s) => (
          <li key={s.id}>
            <button
              type="button"
              className="btn secondary"
              onClick={async () => {
                const res = await apiFetch(`/pos/sales/${s.id}`);
                if (res.ok) setLastSale((await res.json()) as Sale);
              }}
            >
              {s.ticket_number}
            </button>{" "}
            <strong>{money(s.grand_total)}</strong>{" "}
            <span className="muted">
              IVA {money(s.tax_total)}
              {s.request_invoice ? ` · ${t("posInvoiceTag")}` : ""}
            </span>
          </li>
        ))}
      </ul>
    </section>
  );
}

function ReceiptView({ sale, t }: { sale: Sale; t: (k: any) => string }) {
  return (
    <article className="pos-receipt" id="pos-receipt">
      <header>
        <h2>{sale.fiscal?.trade_name || sale.fiscal?.legal_name || "NexusERP"}</h2>
        <p className="muted">
          {sale.fiscal?.legal_name}
          {sale.fiscal?.rfc ? ` · RFC ${sale.fiscal.rfc}` : ""}
        </p>
        <p>
          <strong>{t("posTicket")}</strong> {sale.ticket_number}
        </p>
        <p className="muted">{sale.completed_at ? new Date(sale.completed_at).toLocaleString("es-MX") : ""}</p>
      </header>
      <table>
        <tbody>
          {(sale.lines || []).map((l, i) => (
            <tr key={`${l.sku}-${i}`}>
              <td>
                {l.description}
                <div className="muted">
                  {l.quantity} × {money(l.unit_price)}
                </div>
              </td>
              <td>{money(l.line_total)}</td>
            </tr>
          ))}
        </tbody>
      </table>
      <div className="pos-receipt-totals">
        <div>
          <span>{t("posSubtotal")}</span>
          <span>{money(sale.subtotal)}</span>
        </div>
        <div>
          <span>
            {t("posIva")} ({((sale.tax_rate || 0.16) * 100).toFixed(0)}%)
          </span>
          <span>{money(sale.tax_total)}</span>
        </div>
        <div className="pos-grand">
          <span>{t("posTotal")}</span>
          <span>{money(sale.grand_total)}</span>
        </div>
      </div>
      {(sale.payments || []).map((p) => (
        <p key={p.method + p.amount}>
          {p.method_label || p.method}: {money(p.amount)}
          {p.change_amount > 0 ? ` · ${t("posChange")} ${money(p.change_amount)}` : ""}
        </p>
      ))}
      {sale.invoice ? (
        <div className="pos-receipt-invoice">
          <strong>{t("posInvoiceTag")}</strong>
          <p>
            {sale.invoice.status_label || sale.invoice.status} · {sale.invoice.series}
            {sale.invoice.folio != null ? `-${sale.invoice.folio}` : ""}
          </p>
          <p>
            {sale.invoice.legal_name_receiver} · RFC {sale.invoice.rfc_receiver} · Uso {sale.invoice.uso_cfdi}
          </p>
          {sale.invoice.notes ? <p className="muted">{sale.invoice.notes}</p> : null}
        </div>
      ) : null}
      <footer className="muted">{sale.receipt_hint || sale.fiscal?.receipt_footer}</footer>
      <button type="button" className="btn secondary no-print" onClick={() => window.print()}>
        {t("posPrint")}
      </button>
    </article>
  );
}
