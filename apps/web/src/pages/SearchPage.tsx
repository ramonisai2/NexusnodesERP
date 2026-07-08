import { useQuery } from "@tanstack/react-query";
import { useDeferredValue, useState } from "react";
import { Link } from "react-router-dom";
import { PolicyGuard } from "../auth/PolicyGuard";
import { apiFetch, useAuthStore } from "../auth/store";
import { useLocaleStore } from "../i18n/locale";
import type { MessageKey } from "../i18n/messages";

type SearchHit = {
  id: string;
  kind: "product" | "label" | "photo";
  title: string;
  subtitle?: string;
  snippet?: string;
  sku?: string;
  barcode?: string;
  href?: string;
  score: number;
  branch_id?: string;
};

type SearchResponse = {
  q: string;
  took_ms: number;
  total: number;
  hits: SearchHit[];
  engine: string;
};

const searchNode = {
  id: "nav.search",
  label: "Buscar",
  path: "/search",
  require: {
    permissions: ["inventory.balance.read", "inventory.catalog.read", "reporting.image.read", "search.query"],
    anyBranch: true,
    minAmrCount: 1,
  },
};

export function SearchPage() {
  const t = useLocaleStore((s) => s.t);
  return (
    <PolicyGuard
      node={searchNode}
      fallback={
        <section className="panel">
          <h1>{t("searchTitle")}</h1>
          <p className="error">{t("searchForbidden")}</p>
        </section>
      }
    >
      <SearchPanel />
    </PolicyGuard>
  );
}

function SearchPanel() {
  const t = useLocaleStore((s) => s.t);
  const activeBranchId = useAuthStore((s) => s.activeBranchId);
  const claims = useAuthStore((s) => s.claims);
  const branchId = activeBranchId || claims?.branch_ids?.[0] || "";
  const [q, setQ] = useState("");
  const [kind, setKind] = useState<"all" | "product" | "label" | "photo">("all");
  const deferredQ = useDeferredValue(q.trim());

  const result = useQuery({
    queryKey: ["search", deferredQ, kind, branchId],
    enabled: deferredQ.length >= 2,
    queryFn: async () => {
      const params = new URLSearchParams({ q: deferredQ, limit: "30" });
      if (branchId) params.set("branch_id", branchId);
      if (kind !== "all") params.set("kind", kind);
      const res = await apiFetch(`/search?${params}`);
      if (!res.ok) throw new Error("search_failed");
      return (await res.json()) as SearchResponse;
    },
  });

  return (
    <section className="panel search-page">
      <h1>{t("searchTitle")}</h1>
      <p className="muted">{t("searchSubtitle")}</p>

      <form className="search-box" role="search" onSubmit={(e) => e.preventDefault()}>
        <label className="search-input-wrap">
          <span className="sr-only">{t("searchPlaceholder")}</span>
          <input
            type="search"
            value={q}
            onChange={(e) => setQ(e.target.value)}
            placeholder={t("searchPlaceholder")}
            autoFocus
            autoComplete="off"
            spellCheck={false}
          />
        </label>
        <div className="search-kinds" role="group" aria-label={t("searchFilter")}>
          {(
            [
              ["all", "searchAll"],
              ["product", "searchProducts"],
              ["label", "searchLabels"],
              ["photo", "searchPhotos"],
            ] as const
          ).map(([id, key]) => (
            <button
              key={id}
              type="button"
              className={kind === id ? "kind on" : "kind"}
              onClick={() => setKind(id)}
            >
              {t(key)}
            </button>
          ))}
        </div>
      </form>

      {deferredQ.length > 0 && deferredQ.length < 2 ? (
        <p className="muted tip">{t("searchMinChars")}</p>
      ) : null}
      {result.isFetching ? <p className="muted">{t("searchLoading")}</p> : null}
      {result.isError ? <p className="error">{t("searchError")}</p> : null}
      {result.data && result.data.total === 0 ? <p className="muted">{t("searchEmpty")}</p> : null}

      {result.data && result.data.total > 0 ? (
        <>
          <p className="muted tip">
            {t("searchResults")}: {result.data.total} · {result.data.took_ms} ms · BM25
          </p>
          <ol className="search-hits">
            {result.data.hits.map((hit) => (
              <li key={hit.id} className="search-hit">
                <div className="search-hit-top">
                  <span className={`hit-kind hit-${hit.kind}`}>{labelKind(hit.kind, t)}</span>
                  <span className="muted tip">score {hit.score}</span>
                </div>
                <h2>{hit.href ? <Link to={hit.href}>{hit.title}</Link> : hit.title}</h2>
                {hit.subtitle ? <p className="muted">{hit.subtitle}</p> : null}
                {hit.snippet && hit.snippet !== hit.subtitle ? (
                  <p className="muted tip">{hit.snippet}</p>
                ) : null}
                <p className="muted tip">
                  {[hit.sku, hit.barcode, hit.branch_id].filter(Boolean).join(" · ")}
                </p>
              </li>
            ))}
          </ol>
        </>
      ) : null}
    </section>
  );
}

function labelKind(kind: SearchHit["kind"], t: (key: MessageKey) => string) {
  if (kind === "product") return t("searchProducts");
  if (kind === "label") return t("searchLabels");
  return t("searchPhotos");
}
