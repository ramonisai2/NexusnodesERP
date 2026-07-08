import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import QRCode from "qrcode";
import { useEffect, useState } from "react";
import { hasPermission, NAV_NODES } from "../auth/policy";
import { PolicyGuard } from "../auth/PolicyGuard";
import { apiBlob, apiFetch, useAuthStore } from "../auth/store";
import { labelUser, useLocaleStore } from "../i18n/locale";

type ImageReport = {
  id: string;
  branch_id: string;
  created_by: string;
  title: string;
  notes?: string;
  mime_type: string;
  width: number;
  height: number;
  byte_size: number;
  original_width?: number;
  original_height?: number;
  original_byte_size?: number;
  created_at: string;
  content_url?: string;
};

type UploadSession = {
  token: string;
  expires_at: string;
  max_files: number;
  remaining: number;
  upload_url: string;
  title_hint?: string;
};

const imageNode = NAV_NODES.find((n) => n.id === "nav.imageReports")!;

export function ImageReportsPage() {
  const t = useLocaleStore((s) => s.t);
  return (
    <PolicyGuard
      node={imageNode}
      fallback={
        <section className="panel">
          <h1>{t("imgTitle")}</h1>
          <p className="error">{t("imgForbidden")}</p>
        </section>
      }
    >
      <ImageReportsPanel />
    </PolicyGuard>
  );
}

function ImageReportsPanel() {
  const t = useLocaleStore((s) => s.t);
  const claims = useAuthStore((s) => s.claims);
  const activeBranchId = useAuthStore((s) => s.activeBranchId);
  const branchId = activeBranchId || "br_norte";
  const canCreate = hasPermission(claims, "reporting.image.create");
  const queryClient = useQueryClient();

  const [title, setTitle] = useState("");
  const [notes, setNotes] = useState("");
  const [file, setFile] = useState<File | null>(null);
  const [message, setMessage] = useState<string | null>(null);
  const [qrSession, setQrSession] = useState<UploadSession | null>(null);
  const [qrDataUrl, setQrDataUrl] = useState<string | null>(null);
  const [qrHint, setQrHint] = useState("");

  const list = useQuery({
    queryKey: ["image-reports", branchId],
    queryFn: async () => {
      const res = await apiFetch(`/reports/images?branch_id=${encodeURIComponent(branchId)}`);
      if (!res.ok) throw new Error("list_failed");
      return (await res.json()) as ImageReport[];
    },
    refetchInterval: qrSession ? 4000 : false,
  });

  const upload = useMutation({
    mutationFn: async () => {
      if (!file || !title.trim()) throw new Error("missing_fields");
      const form = new FormData();
      form.append("title", title.trim());
      form.append("notes", notes.trim());
      form.append("branch_id", branchId);
      form.append("file", file);
      const res = await apiFetch("/reports/images", { method: "POST", body: form });
      if (!res.ok) {
        const body = await res.json().catch(() => ({}));
        throw new Error(body.error || "upload_failed");
      }
      return (await res.json()) as ImageReport;
    },
    onSuccess: () => {
      setTitle("");
      setNotes("");
      setFile(null);
      setMessage(t("imgUploadOk"));
      void queryClient.invalidateQueries({ queryKey: ["image-reports", branchId] });
    },
    onError: () => setMessage(t("imgUploadError")),
  });

  const createQr = useMutation({
    mutationFn: async () => {
      const res = await apiFetch("/reports/images/upload-sessions", {
        method: "POST",
        body: JSON.stringify({
          branch_id: branchId,
          title_hint: qrHint.trim() || title.trim() || undefined,
          max_files: 8,
        }),
      });
      const body = await res.json().catch(() => ({}));
      if (!res.ok) throw new Error(body.error || "session_failed");
      return body as UploadSession;
    },
    onSuccess: async (sess) => {
      // Prefer current browser origin so LAN phones hit the same SPA host.
      const url = `${window.location.origin}/upload/${sess.token}`;
      const dataUrl = await QRCode.toDataURL(url, {
        margin: 1,
        width: 280,
        color: { dark: "#0f1c17", light: "#ffffff" },
      });
      setQrSession({ ...sess, upload_url: url });
      setQrDataUrl(dataUrl);
    },
  });

  useEffect(() => {
    if (!qrSession) return;
    const expires = new Date(qrSession.expires_at).getTime();
    const ms = expires - Date.now();
    if (ms <= 0) {
      setQrSession(null);
      setQrDataUrl(null);
      return;
    }
    const timer = window.setTimeout(() => {
      setQrSession(null);
      setQrDataUrl(null);
    }, ms);
    return () => window.clearTimeout(timer);
  }, [qrSession]);

  return (
    <section className="panel">
      <h1>{t("imgTitle")}</h1>
      <p className="muted">{t("imgSubtitle")}</p>

      {canCreate ? (
        <>
          <div className="qr-upload-panel">
            <h2>{t("qrTitle")}</h2>
            <p className="muted">{t("qrSubtitle")}</p>
            <label>
              <span className="muted">{t("qrHintLabel")}</span>
              <input
                value={qrHint}
                onChange={(e) => setQrHint(e.target.value)}
                placeholder={t("qrHintPh")}
                maxLength={120}
              />
            </label>
            <div className="setup-actions">
              <button
                type="button"
                className="btn"
                disabled={createQr.isPending}
                onClick={() => createQr.mutate()}
              >
                {createQr.isPending ? t("qrGenerating") : t("qrGenerate")}
              </button>
              {qrSession ? (
                <button
                  type="button"
                  className="btn secondary"
                  onClick={() => {
                    setQrSession(null);
                    setQrDataUrl(null);
                  }}
                >
                  {t("qrClose")}
                </button>
              ) : null}
            </div>
            {createQr.isError ? <p className="error">{t("qrError")}</p> : null}
            {qrSession && qrDataUrl ? (
              <div className="qr-box">
                <img src={qrDataUrl} alt={t("qrAlt")} width={280} height={280} />
                <div>
                  <p className="muted tip">{t("qrScanHint")}</p>
                  <p className="muted tip">
                    {t("qrExpires")}: {new Date(qrSession.expires_at).toLocaleTimeString()} ·{" "}
                    {t("qrMaxFiles")}: {qrSession.max_files}
                  </p>
                  <a className="linkish" href={qrSession.upload_url} target="_blank" rel="noreferrer">
                    {t("qrOpenLink")}
                  </a>
                </div>
              </div>
            ) : null}
          </div>

          <form
            className="image-upload"
            onSubmit={(e) => {
              e.preventDefault();
              setMessage(null);
              upload.mutate();
            }}
          >
            <h2>{t("imgUploadTitle")}</h2>
            <label>
              <span className="muted">{t("imgFieldTitle")}</span>
              <input
                value={title}
                onChange={(e) => setTitle(e.target.value)}
                required
                maxLength={120}
                placeholder="Ej. Anaquel dañado — pasillo 3"
              />
            </label>
            <label>
              <span className="muted">{t("imgFieldNotes")}</span>
              <textarea
                value={notes}
                onChange={(e) => setNotes(e.target.value)}
                rows={2}
                maxLength={500}
              />
            </label>
            <label className="file-picker">
              <span className="muted">{t("imgFieldFile")}</span>
              <input
                type="file"
                accept="image/jpeg,image/png,image/webp,image/gif"
                onChange={(e) => setFile(e.target.files?.[0] ?? null)}
                required
              />
              <span className="file-name">{file ? file.name : t("imgChooseFile")}</span>
            </label>
            <button type="submit" className="btn" disabled={upload.isPending || !file || !title.trim()}>
              {upload.isPending ? t("imgSubmitting") : t("imgSubmit")}
            </button>
            {message ? (
              <p className={upload.isError ? "error" : "muted tip"}>{message}</p>
            ) : null}
          </form>
        </>
      ) : null}

      {list.isLoading ? <p className="muted">{t("imgLoading")}</p> : null}
      {list.isError ? <p className="error">{t("imgError")}</p> : null}
      {list.data && list.data.length === 0 ? <p className="muted">{t("imgEmpty")}</p> : null}

      <div className="image-gallery">
        {(list.data ?? []).map((item) => (
          <ImageCard key={item.id} item={item} />
        ))}
      </div>
    </section>
  );
}

function ImageCard({ item }: { item: ImageReport }) {
  const t = useLocaleStore((s) => s.t);
  const [src, setSrc] = useState<string | null>(null);

  useEffect(() => {
    let revoked: string | null = null;
    let cancelled = false;
    const path = item.content_url?.replace(/^\/reports/, "/reports") ?? `/reports/images/${item.id}/content`;
    void apiBlob(path)
      .then((url) => {
        if (cancelled) {
          URL.revokeObjectURL(url);
          return;
        }
        revoked = url;
        setSrc(url);
      })
      .catch(() => {
        if (!cancelled) setSrc(null);
      });
    return () => {
      cancelled = true;
      if (revoked) URL.revokeObjectURL(revoked);
    };
  }, [item.content_url, item.id]);

  function formatBytes(n: number): string {
    if (n < 1024) return `${n} B`;
    if (n < 1024 * 1024) return `${(n / 1024).toFixed(1)} KB`;
    return `${(n / (1024 * 1024)).toFixed(1)} MB`;
  }

  return (
    <article className="image-card">
      <div className="image-card-media" aria-label={t("imgPreview")}>
        {src ? <img src={src} alt={item.title} loading="lazy" /> : <div className="image-card-placeholder" />}
      </div>
      <div className="image-card-body">
        <h3>{item.title}</h3>
        {item.notes ? <p className="muted">{item.notes}</p> : null}
        <p className="muted tip">
          {t("imgDims")}: {item.width}×{item.height}
          {item.original_width && item.original_height
            ? ` · ${t("imgOriginal")}: ${item.original_width}×${item.original_height}`
            : ""}
        </p>
        <p className="muted tip">
          {t("imgSize")}: {formatBytes(item.byte_size)}
          {item.original_byte_size ? ` · ${t("imgOriginal")}: ${formatBytes(item.original_byte_size)}` : ""}
        </p>
        <p className="muted tip">
          {t("imgBy")}: {labelUser(item.created_by)} · {new Date(item.created_at).toLocaleString()}
        </p>
      </div>
    </article>
  );
}
