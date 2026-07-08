import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { NAV_NODES, hasPermission } from "../auth/policy";
import { PolicyGuard } from "../auth/PolicyGuard";
import { apiFetch, useAuthStore } from "../auth/store";
import { friendlyApiError, labelBranch, useLocaleStore } from "../i18n/locale";

type Balance = {
  id: string;
  warehouse_id: string;
  branch_id: string;
  sku_id: string;
  sku: string;
  on_hand: number;
  reserved: number;
  version: number;
};

const inventoryNode = NAV_NODES.find((n) => n.id === "nav.inventory")!;

export function InventoryPage() {
  const t = useLocaleStore((s) => s.t);
  return (
    <PolicyGuard
      node={inventoryNode}
      fallback={
        <section className="panel">
          <h1>{t("invTitle")}</h1>
          <p className="error">{t("invForbidden")}</p>
        </section>
      }
    >
      <InventoryPanel />
    </PolicyGuard>
  );
}

function InventoryPanel() {
  const claims = useAuthStore((s) => s.claims);
  const activeBranchId = useAuthStore((s) => s.activeBranchId);
  const qc = useQueryClient();
  const t = useLocaleStore((s) => s.t);
  const locale = useLocaleStore((s) => s.locale);
  const canMove = hasPermission(claims, "inventory.movement.create");

  const balances = useQuery({
    queryKey: ["balances", activeBranchId],
    queryFn: async () => {
      const res = await apiFetch("/inventory/balances");
      if (!res.ok) throw new Error("balances_failed");
      return (await res.json()) as Balance[];
    },
  });

  const move = useMutation({
    mutationFn: async (row: Balance) => {
      const res = await apiFetch("/inventory/movements", {
        method: "POST",
        headers: { "Idempotency-Key": crypto.randomUUID() },
        body: JSON.stringify({
          branch_id: row.branch_id,
          warehouse_id: row.warehouse_id,
          sku_id: row.sku_id,
          movement_type: "ISSUE",
          quantity: 1,
          expected_version: row.version,
        }),
      });
      if (!res.ok) {
        const body = await res.text();
        throw new Error(body || "move_failed");
      }
      return res.json();
    },
    onSuccess: () => qc.invalidateQueries({ queryKey: ["balances"] }),
  });

  const rows =
    balances.data?.filter((b) => !activeBranchId || b.branch_id === activeBranchId) ?? [];

  return (
    <section className="panel">
      <h1>{t("invTitle")}</h1>
      <p className="muted">{t("invSubtitle")}</p>
      {balances.isLoading ? <p className="muted">{t("invLoading")}</p> : null}
      {balances.isError ? <p className="error">{t("invError")}</p> : null}
      {balances.data && rows.length === 0 ? <p className="muted">{t("invNoRows")}</p> : null}
      {rows.length > 0 ? (
        <table>
          <thead>
            <tr>
              <th>{t("invColSku")}</th>
              <th>{t("invColWarehouse")}</th>
              <th>{t("invColBranch")}</th>
              <th>{t("invColOnHand")}</th>
              <th />
            </tr>
          </thead>
          <tbody>
            {rows.map((b) => (
              <tr key={b.id}>
                <td>{b.sku}</td>
                <td>{b.warehouse_id.replace(/^wh_/, "").replace(/_/g, " ")}</td>
                <td>{labelBranch(b.branch_id, locale)}</td>
                <td>{b.on_hand}</td>
                <td>
                  <button
                    type="button"
                    className="btn secondary"
                    disabled={!canMove || move.isPending}
                    onClick={() => move.mutate(b)}
                  >
                    {move.isPending ? t("invIssuePending") : t("invIssue")}
                  </button>
                </td>
              </tr>
            ))}
          </tbody>
        </table>
      ) : null}
      {move.isError ? (
        <p className="error">{friendlyApiError((move.error as Error).message, locale)}</p>
      ) : null}
    </section>
  );
}
