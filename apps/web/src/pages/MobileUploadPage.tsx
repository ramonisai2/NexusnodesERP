import { useMutation, useQuery } from "@tanstack/react-query";
import { useMemo, useState } from "react";
import { Link, useParams } from "react-router-dom";
import { LanguageSwitcher } from "../components/LanguageSwitcher";
import { useLocaleStore } from "../i18n/locale";

type SessionInfo = {
  ok: boolean;
  expires_at: string;
  max_files: number;
  remaining: number;
  title_hint?: string;
  branch_id: string;
};

export function MobileUploadPage() {
  const { token = "" } = useParams();
  const t = useLocaleStore((s) => s.t);
  const [title, setTitle] = useState("");
  const [notes, setNotes] = useState("");
  const [files, setFiles] = useState<File[]>([]);
  const [done, setDone] = useState(false);
  const [uploaded, setUploaded] = useState(0);

  const session = useQuery({
    queryKey: ["upload-session", token],
    enabled: Boolean(token),
    queryFn: async () => {
      const res = await fetch(`/api/reports/images/upload/${encodeURIComponent(token)}`);
      const body = await res.json().catch(() => ({}));
      if (!res.ok) throw new Error(body.error || "session_invalid");
      return body as SessionInfo;
    },
    retry: false,
  });

  const hint = session.data?.title_hint || "";
  const defaultTitle = useMemo(() => hint || t("mobileDefaultTitle"), [hint, t]);

  const upload = useMutation({
    mutationFn: async () => {
      if (!files.length) throw new Error("no_files");
      const form = new FormData();
      form.append("title", (title.trim() || defaultTitle).slice(0, 120));
      form.append("notes", notes.trim().slice(0, 500));
      for (const f of files) {
        form.append("files", f);
      }
      const res = await fetch(`/api/reports/images/upload/${encodeURIComponent(token)}`, {
        method: "POST",
        body: form,
      });
      const body = await res.json().catch(() => ({}));
      if (!res.ok) throw new Error(body.error || "upload_failed");
      return body as { uploaded: number; remaining: number };
    },
    onSuccess: (data) => {
      setUploaded(data.uploaded);
      setDone(true);
      setFiles([]);
      void session.refetch();
    },
  });

  function onPick(list: FileList | null) {
    if (!list) return;
    const next = Array.from(list).filter((f) => f.type.startsWith("image/") || !f.type);
    const max = session.data?.remaining ?? 8;
    setFiles(next.slice(0, Math.max(1, max)));
  }

  if (session.isLoading) {
    return (
      <main className="mobile-upload">
        <p className="muted">{t("mobileLoading")}</p>
      </main>
    );
  }

  if (session.isError) {
    const code = (session.error as Error).message;
    let msg = t("mobileSessionBad");
    if (code === "session_expired") msg = t("mobileSessionExpired");
    if (code === "session_exhausted") msg = t("mobileSessionExhausted");
    return (
      <main className="mobile-upload">
        <section className="mobile-card">
          <LanguageSwitcher />
          <h1>{t("mobileTitle")}</h1>
          <p className="error">{msg}</p>
          <p className="muted tip">{t("mobileAskNewQr")}</p>
        </section>
      </main>
    );
  }

  const remaining = session.data?.remaining ?? 0;

  return (
    <main className="mobile-upload">
      <section className="mobile-card">
        <div className="mobile-top">
          <LanguageSwitcher />
        </div>
        <p className="setup-kicker">{t("mobileKicker")}</p>
        <h1>{t("mobileTitle")}</h1>
        <p className="muted">{t("mobileSubtitle")}</p>
        <p className="muted tip">
          {t("mobileRemaining")}: <strong>{remaining}</strong> · {t("mobileExpires")}:{" "}
          {session.data ? new Date(session.data.expires_at).toLocaleTimeString() : "—"}
        </p>

        {done ? (
          <div className="mobile-done">
            <p className="ok">
              {t("mobileUploadOk")} ({uploaded})
            </p>
            {remaining > 0 ? (
              <button type="button" className="btn" onClick={() => setDone(false)}>
                {t("mobileUploadMore")}
              </button>
            ) : (
              <p className="muted tip">{t("mobileAllDone")}</p>
            )}
          </div>
        ) : (
          <form
            className="mobile-form"
            onSubmit={(e) => {
              e.preventDefault();
              upload.mutate();
            }}
          >
            <label>
              <span>{t("imgFieldTitle")}</span>
              <input
                value={title}
                onChange={(e) => setTitle(e.target.value)}
                placeholder={defaultTitle}
                maxLength={120}
              />
            </label>
            <label>
              <span>{t("imgFieldNotes")}</span>
              <textarea value={notes} onChange={(e) => setNotes(e.target.value)} rows={2} maxLength={500} />
            </label>

            <label className="mobile-file">
              <input
                type="file"
                accept="image/*"
                multiple
                onChange={(e) => onPick(e.target.files)}
              />
              <span className="mobile-file-cta">{t("mobileChooseFiles")}</span>
              <span className="muted tip">{t("mobileChooseHint")}</span>
            </label>

            {files.length > 0 ? (
              <ul className="mobile-file-list">
                {files.map((f) => (
                  <li key={`${f.name}-${f.size}-${f.lastModified}`}>{f.name}</li>
                ))}
              </ul>
            ) : null}

            {upload.isError ? (
              <p className="error">{friendlyMobileError((upload.error as Error).message, t)}</p>
            ) : null}

            <button
              type="submit"
              className="btn"
              disabled={upload.isPending || files.length === 0 || remaining <= 0}
            >
              {upload.isPending ? t("mobileSubmitting") : t("mobileSubmit")}
            </button>
          </form>
        )}

        <p className="muted secure-note">
          <Link to="/login">{t("mobileBackLogin")}</Link>
        </p>
      </section>
    </main>
  );
}

function friendlyMobileError(code: string, t: (key: import("../i18n/messages").MessageKey) => string): string {
  if (code === "session_expired") return t("mobileSessionExpired");
  if (code === "session_exhausted") return t("mobileSessionExhausted");
  if (code === "too_many_files") return t("mobileTooMany");
  return t("mobileUploadError");
}
