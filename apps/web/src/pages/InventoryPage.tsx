import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { NAV_NODES, hasPermission } from "../auth/policy";
import { PolicyGuard } from "../auth/PolicyGuard";
import { apiFetch, useAuthStore } from "../auth/store";
import { friendlyApiError, labelBranch, labelUser, useLocaleStore } from "../i18n/locale";

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

type Movement = {
  id: string;
  branch_id: string;
  warehouse_id: string;
  sku_id: string;
  movement_type: string;
  quantity: number;
  status: string;
  posted_by: string;
  created_at: string;
  void_reason?: string;
  voided_by?: string;
  reversal_of?: string;
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
  const canReadMovements = hasPermission(claims, "inventory.movement.read");
  const canVoid = hasPermission(claims, "inventory.movement.void");
  const managed = (claims?.attrs?.managed_warehouses as string[] | undefined) ?? [];

  const balances = useQuery({
    queryKey: ["balances", activeBranchId],
    queryFn: async () => {
      const res = await apiFetch("/inventory/balances");
      if (!res.ok) throw new Error("balances_failed");
      return (await res.json()) as Balance[];
    },
  });

  const movements = useQuery({
    queryKey: ["movements", activeBranchId],
    enabled: canReadMovements,
    queryFn: async () => {
      const res = await apiFetch("/inventory/movements?limit=30");
      if (!res.ok) throw new Error("movements_failed");
      return (await res.json()) as Movement[];
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
    onSuccess: () => {
      void qc.invalidateQueries({ queryKey: ["balances"] });
      void qc.invalidateQueries({ queryKey: ["movements"] });
    },
  });

  const voidMove = useMutation({
    mutationFn: async (row: Movement) => {
      const reason =
        locale === "en" ? "Finger error / bad practice correction" : "Error de captura / mala práctica";
      const res = await apiFetch(`/inventory/movements/${row.id}/void`, {
        method: "POST",
        headers: { "Idempotency-Key": crypto.randomUUID() },
        body: JSON.stringify({ reason }),
      });
      if (!res.ok) {
        const body = await res.text();
        throw new Error(body || "void_failed");
      }
      return res.json();
    },
    onSuccess: () => {
      void qc.invalidateQueries({ queryKey: ["balances"] });
      void qc.invalidateQueries({ queryKey: ["movements"] });
    },
  });

  const rows =
    balances.data?.filter((b) => !activeBranchId || b.branch_id === activeBranchId) ?? [];

  const movementRows =
    movements.data?.filter((m) => !activeBranchId || m.branch_id === activeBranchId) ?? [];

  function canVoidRow(m: Movement): boolean {
    if (!canVoid || m.status !== "POSTED" || m.movement_type === "REVERSAL") return false;
    if (claims?.roles.includes("platform_admin")) return true;
    return managed.includes(m.warehouse_id) || managed.includes("*");
  }

  return (
    <section className="panel">
      <h1>{t("invTitle")}</h1>
      <p className="muted">{t("invSubtitle")}</p>
      {canVoid ? <p className="muted tip">{t("invManagerTip")}</p> : null}
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

      {canReadMovements ? (
        <>
          <h2 style={{ marginTop: "2rem" }}>{t("invMovementsTitle")}</h2>
          <p className="muted">{t("invMovementsSubtitle")}</p>
          {movements.isLoading ? <p className="muted">{t("invMovementsLoading")}</p> : null}
          {movements.isError ? <p className="error">{t("invMovementsError")}</p> : null}
          {movements.data && movementRows.length === 0 ? (
            <p className="muted">{t("invMovementsEmpty")}</p>
          ) : null}
          {movementRows.length > 0 ? (
            <table>
              <thead>
                <tr>
                  <th>{t("invColSku")}</th>
                  <th>{t("invColWarehouse")}</th>
                  <th>{t("invColType")}</th>
                  <th>{t("invColQty")}</th>
                  <th>{t("invColStatus")}</th>
                  <th>{t("invColBy")}</th>
                  <th />
                </tr>
              </thead>
              <tbody>
                {movementRows.map((m) => (
                  <tr key={m.id}>
                    <td>{m.sku_id}</td>
                    <td>{m.warehouse_id.replace(/^wh_/, "").replace(/_/g, " ")}</td>
                    <td>{m.movement_type}</td>
                    <td>{m.quantity}</td>
                    <td>{m.status === "VOID" ? t("statusVoid") : t("statusPosted")}</td>
                    <td>{labelUser(m.posted_by)}</td>
                    <td>
                      {canVoidRow(m) ? (
                        <button
                          type="button"
                          className="btn secondary"
                          disabled={voidMove.isPending}
                          onClick={() => voidMove.mutate(m)}
                        >
                          {voidMove.isPending ? t("invVoidPending") : t("invVoid")}
                        </button>
                      ) : null}
                    </td>
                  </tr>
                ))}
              </tbody>
            </table>
          ) : null}
          {voidMove.isError ? (
            <p className="error">{friendlyApiError((voidMove.error as Error).message, locale)}</p>
          ) : null}
          {voidMove.isSuccess ? <p className="muted tip">{t("invVoidSuccess")}</p> : null}
        </>
      ) : null}
    </section>
  );
}
