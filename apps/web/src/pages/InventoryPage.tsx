import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { NAV_NODES, hasPermission } from "../auth/policy";
import { PolicyGuard } from "../auth/PolicyGuard";
import { apiFetch, useAuthStore } from "../auth/store";

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
  return (
    <PolicyGuard
      node={inventoryNode}
      fallback={
        <section className="panel">
          <h1>Inventario</h1>
          <p className="error">No tienes permiso para ver inventarios en esta sesión.</p>
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

  return (
    <section className="panel">
      <h1>Inventario</h1>
      <p className="muted">
        Saldos por almacén con bloqueo optimista (`version`). Filtrado por sucursal activa.
      </p>
      {balances.isLoading ? <p className="muted">Cargando…</p> : null}
      {balances.isError ? <p className="error">Error al cargar saldos.</p> : null}
      {balances.data ? (
        <table>
          <thead>
            <tr>
              <th>SKU</th>
              <th>Almacén</th>
              <th>Sucursal</th>
              <th>On hand</th>
              <th>Versión</th>
              <th />
            </tr>
          </thead>
          <tbody>
            {balances.data
              .filter((b) => !activeBranchId || b.branch_id === activeBranchId)
              .map((b) => (
                <tr key={b.id}>
                  <td>{b.sku}</td>
                  <td>{b.warehouse_id}</td>
                  <td>{b.branch_id}</td>
                  <td>{b.on_hand}</td>
                  <td>{b.version}</td>
                  <td>
                    <button
                      type="button"
                      className="btn secondary"
                      disabled={!canMove || move.isPending}
                      onClick={() => move.mutate(b)}
                    >
                      Salida −1
                    </button>
                  </td>
                </tr>
              ))}
          </tbody>
        </table>
      ) : null}
      {move.isError ? <p className="error">{(move.error as Error).message}</p> : null}
    </section>
  );
}
