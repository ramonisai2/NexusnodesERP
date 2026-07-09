import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { useEffect, useMemo, useState } from "react";
import { hasPermission } from "../auth/policy";
import { PolicyGuard } from "../auth/PolicyGuard";
import { apiFetch, useAuthStore } from "../auth/store";
import { friendlyApiError, useLocaleStore } from "../i18n/locale";

type Assignment = {
  id: string;
  user_sub: string;
  display_name: string;
  email?: string;
  branch_code: string;
  department_code: string;
  department_name: string;
};

type DeptOption = { id: string; code: string; name: string };
type UserOption = { id: string; sub: string; display_name: string; email?: string };

const managersNode = {
  id: "nav.deptManagers",
  label: "Jefes de departamento",
  path: "/settings/jefes",
  require: {
    permissions: ["store.department.manager.read", "store.department.manager.assign"],
    anyBranch: true,
    minAmrCount: 1,
  },
};

export function DepartmentManagersPage() {
  const t = useLocaleStore((s) => s.t);
  return (
    <PolicyGuard
      node={managersNode}
      fallback={
        <section className="panel">
          <h1>{t("deptMgrTitle")}</h1>
          <p className="error">{t("deptMgrForbidden")}</p>
        </section>
      }
    >
      <DepartmentManagersPanel />
    </PolicyGuard>
  );
}

function DepartmentManagersPanel() {
  const t = useLocaleStore((s) => s.t);
  const locale = useLocaleStore((s) => s.locale);
  const claims = useAuthStore((s) => s.claims);
  const branchId = useAuthStore((s) => s.activeBranchId) || claims?.branch_ids?.[0] || "";
  const qc = useQueryClient();
  const canAssign = hasPermission(claims, "store.department.manager.assign");

  const [userSub, setUserSub] = useState("");
  const [selected, setSelected] = useState<string[]>([]);
  const [saved, setSaved] = useState(false);
  const [error, setError] = useState("");

  const options = useQuery({
    queryKey: ["dept-mgr-options", branchId],
    enabled: Boolean(branchId),
    queryFn: async () => {
      const res = await apiFetch(`/org/department-managers/options?branch_id=${encodeURIComponent(branchId)}`);
      if (!res.ok) throw new Error(await res.text());
      return (await res.json()) as { departments: DeptOption[]; users: UserOption[] };
    },
  });

  const assignments = useQuery({
    queryKey: ["dept-mgr-list", branchId],
    enabled: Boolean(branchId),
    queryFn: async () => {
      const res = await apiFetch(`/org/department-managers?branch_id=${encodeURIComponent(branchId)}`);
      if (!res.ok) throw new Error(await res.text());
      return (await res.json()) as { items: Assignment[] };
    },
  });

  useEffect(() => {
    if (!userSub && options.data?.users?.length) {
      setUserSub(options.data.users[0].sub);
    }
  }, [options.data, userSub]);

  useEffect(() => {
    if (!userSub || !assignments.data) return;
    const codes = (assignments.data.items || [])
      .filter((a) => a.user_sub === userSub)
      .map((a) => a.department_code);
    setSelected(codes);
    setSaved(false);
  }, [userSub, assignments.data]);

  const byUser = useMemo(() => {
    const map = new Map<string, Assignment[]>();
    for (const a of assignments.data?.items || []) {
      const list = map.get(a.user_sub) || [];
      list.push(a);
      map.set(a.user_sub, list);
    }
    return map;
  }, [assignments.data]);

  const save = useMutation({
    mutationFn: async () => {
      const res = await apiFetch("/org/department-managers", {
        method: "PUT",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({
          user_sub: userSub,
          branch_id: branchId,
          department_codes: selected,
        }),
      });
      if (!res.ok) throw new Error(await res.text());
      return res.json();
    },
    onSuccess: () => {
      setSaved(true);
      setError("");
      void qc.invalidateQueries({ queryKey: ["dept-mgr-list"] });
    },
    onError: (err: Error) => {
      setSaved(false);
      setError(friendlyApiError(err.message, locale) || t("deptMgrSaveError"));
    },
  });

  const toggle = (code: string) => {
    setSelected((prev) => (prev.includes(code) ? prev.filter((c) => c !== code) : [...prev, code]));
    setSaved(false);
  };

  return (
    <section className="panel">
      <header className="transfer-header">
        <div>
          <h1>{t("deptMgrTitle")}</h1>
          <p className="muted">{t("deptMgrSubtitle")}</p>
        </div>
      </header>

      <p className="muted">{t("deptMgrRule")}</p>

      {options.isLoading || assignments.isLoading ? <p className="muted">{t("deptMgrLoading")}</p> : null}
      {error ? <p className="error">{error}</p> : null}
      {saved ? <p className="ok">{t("deptMgrSaved")}</p> : null}

      {canAssign ? (
        <div className="form-grid" style={{ marginTop: "1rem" }}>
          <label>
            {t("deptMgrUser")}
            <select value={userSub} onChange={(e) => setUserSub(e.target.value)}>
              {(options.data?.users || []).map((u) => (
                <option key={u.sub} value={u.sub}>
                  {u.display_name} ({u.sub})
                </option>
              ))}
            </select>
          </label>

          <fieldset>
            <legend>{t("deptMgrDepartments")}</legend>
            <p className="muted">{t("deptMgrPickTip")}</p>
            <div className="chip-list">
              {(options.data?.departments || []).map((d) => {
                const on = selected.includes(d.code);
                return (
                  <label key={d.code} className={on ? "chip on" : "chip"}>
                    <input type="checkbox" checked={on} onChange={() => toggle(d.code)} />
                    <span>
                      {d.name} <code>{d.code}</code>
                    </span>
                  </label>
                );
              })}
            </div>
            {(options.data?.departments || []).length === 0 ? (
              <p className="muted">{t("deptMgrNoDepts")}</p>
            ) : null}
          </fieldset>

          <div className="actions">
            <button
              type="button"
              className="btn"
              disabled={!userSub || save.isPending}
              onClick={() => save.mutate()}
            >
              {save.isPending ? t("deptMgrSaving") : t("deptMgrSave")}
            </button>
          </div>
        </div>
      ) : (
        <p className="muted">{t("deptMgrReadOnly")}</p>
      )}

      <h2 style={{ marginTop: "2rem" }}>{t("deptMgrCurrent")}</h2>
      {byUser.size === 0 ? <p className="muted">{t("deptMgrEmpty")}</p> : null}
      <ul className="plain-list">
        {[...byUser.entries()].map(([sub, items]) => (
          <li key={sub}>
            <strong>{items[0]?.display_name || sub}</strong>{" "}
            <span className="muted">({sub})</span>
            <div className="chip-list">
              {items.map((a) => (
                <span key={a.id} className="chip on">
                  {a.department_name} <code>{a.department_code}</code>
                </span>
              ))}
            </div>
          </li>
        ))}
      </ul>
    </section>
  );
}
