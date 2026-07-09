import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { useEffect, useMemo, useState } from "react";
import { PolicyGuard } from "../auth/PolicyGuard";
import { apiFetch, useAuthStore } from "../auth/store";
import { useLocaleStore } from "../i18n/locale";
import type { MessageKey } from "../i18n/messages";

type Folder = "INBOX" | "SENT" | "ARCHIVE";
type Priority = "LOW" | "NORMAL" | "HIGH" | "URGENT";
type Kind = "MESSAGE" | "ANNOUNCEMENT";

type MailMessage = {
  id: string;
  from_sub: string;
  from_operator?: string;
  to_subs?: string[];
  subject: string;
  body: string;
  priority: Priority;
  priority_color: string;
  kind: Kind;
  expires_at?: string | null;
  expired?: boolean;
  created_at: string;
  read_at?: string | null;
  folder?: string;
};

type DirectoryUser = { sub: string; name: string; email: string };

const mailNode = {
  id: "nav.mail",
  label: "Correo",
  path: "/mail",
  require: {
    permissions: ["mail.read", "mail.send", "inventory.balance.read"],
    anyBranch: true,
    minAmrCount: 1,
  },
};

const PRIORITY_KEYS: Record<Priority, MessageKey> = {
  LOW: "mailPriorityLow",
  NORMAL: "mailPriorityNormal",
  HIGH: "mailPriorityHigh",
  URGENT: "mailPriorityUrgent",
};

export function MailPage() {
  const t = useLocaleStore((s) => s.t);
  return (
    <PolicyGuard
      node={mailNode}
      fallback={
        <section className="panel">
          <h1>{t("mailTitle")}</h1>
          <p className="error">{t("mailForbidden")}</p>
        </section>
      }
    >
      <MailPanel />
    </PolicyGuard>
  );
}

function MailPanel() {
  const t = useLocaleStore((s) => s.t);
  const claims = useAuthStore((s) => s.claims);
  const qc = useQueryClient();
  const [folder, setFolder] = useState<Folder>("INBOX");
  const [selectedId, setSelectedId] = useState<string | null>(null);
  const [composing, setComposing] = useState(false);
  const canAnnounce =
    claims?.permissions?.includes("mail.announce") ||
    claims?.roles?.some((r) =>
      ["platform_admin", "store_owner", "regional_manager", "warehouse_manager", "payroll_approver"].includes(r),
    );

  const list = useQuery({
    queryKey: ["mail", folder],
    queryFn: async () => {
      const path =
        folder === "INBOX" ? "/mail/inbox" : folder === "SENT" ? "/mail/sent" : "/mail/archive";
      const res = await apiFetch(path);
      if (!res.ok) throw new Error("list_failed");
      const data = (await res.json()) as { items: MailMessage[] };
      return data.items ?? [];
    },
  });

  const unread = useQuery({
    queryKey: ["mail", "unread"],
    queryFn: async () => {
      const res = await apiFetch("/mail/unread-count");
      if (!res.ok) throw new Error("unread_failed");
      return (await res.json()) as { unread: number };
    },
    refetchInterval: 20_000,
  });

  const directory = useQuery({
    queryKey: ["mail", "directory"],
    queryFn: async () => {
      const res = await apiFetch("/mail/directory");
      if (!res.ok) throw new Error("directory_failed");
      const data = (await res.json()) as { items: DirectoryUser[] };
      return data.items ?? [];
    },
  });

  const selected = useMemo(
    () => list.data?.find((m) => m.id === selectedId) ?? null,
    [list.data, selectedId],
  );

  useEffect(() => {
    if (!selectedId && list.data && list.data.length > 0) {
      setSelectedId(list.data[0].id);
    }
  }, [list.data, selectedId]);

  const markRead = useMutation({
    mutationFn: async (id: string) => {
      const res = await apiFetch(`/mail/messages/${id}/read`, { method: "POST" });
      if (!res.ok) throw new Error("mark_read_failed");
      return res.json();
    },
    onSuccess: () => {
      void qc.invalidateQueries({ queryKey: ["mail"] });
    },
  });

  const archive = useMutation({
    mutationFn: async (id: string) => {
      const res = await apiFetch(`/mail/messages/${id}/archive`, { method: "POST" });
      if (!res.ok) throw new Error("archive_failed");
      return res.json();
    },
    onSuccess: () => {
      setSelectedId(null);
      void qc.invalidateQueries({ queryKey: ["mail"] });
    },
  });

  useEffect(() => {
    if (folder === "INBOX" && selected && !selected.read_at) {
      markRead.mutate(selected.id);
    }
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [selected?.id, folder]);

  const nameOf = (sub: string) => {
    const u = directory.data?.find((d) => d.sub === sub);
    return u?.name || sub;
  };

  return (
    <section className="panel mail-page">
      <header className="mail-header">
        <div>
          <h1>{t("mailTitle")}</h1>
          <p className="muted">{t("mailSubtitle")}</p>
        </div>
        <button type="button" className="btn primary" onClick={() => setComposing(true)}>
          {t("mailCompose")}
        </button>
      </header>

      <div className="mail-layout">
        <nav className="mail-folders" aria-label={t("mailFolders")}>
          {(
            [
              ["INBOX", "mailInbox", unread.data?.unread],
              ["SENT", "mailSent", undefined],
              ["ARCHIVE", "mailArchive", undefined],
            ] as const
          ).map(([id, key, badge]) => (
            <button
              key={id}
              type="button"
              className={`mail-folder${folder === id ? " active" : ""}`}
              onClick={() => {
                setFolder(id);
                setSelectedId(null);
                setComposing(false);
              }}
            >
              <span>{t(key)}</span>
              {typeof badge === "number" && badge > 0 ? (
                <span className="mail-badge">{badge}</span>
              ) : null}
            </button>
          ))}
          <div className="mail-legend">
            <p className="muted">{t("mailPriorityLegend")}</p>
            {(["LOW", "NORMAL", "HIGH", "URGENT"] as Priority[]).map((p) => (
              <span key={p} className={`mail-prio-chip prio-${p.toLowerCase()}`}>
                {t(PRIORITY_KEYS[p])}
              </span>
            ))}
          </div>
        </nav>

        <div className="mail-list" role="list">
          {list.isLoading ? <p className="muted">{t("mailLoading")}</p> : null}
          {list.isError ? <p className="error">{t("mailError")}</p> : null}
          {!list.isLoading && (list.data?.length ?? 0) === 0 ? (
            <p className="muted">{t("mailEmpty")}</p>
          ) : null}
          {list.data?.map((m) => (
            <button
              key={m.id}
              type="button"
              role="listitem"
              className={`mail-row prio-${(m.priority_color || "blue").toLowerCase()}${
                selectedId === m.id ? " selected" : ""
              }${!m.read_at && folder === "INBOX" ? " unread" : ""}`}
              onClick={() => {
                setSelectedId(m.id);
                setComposing(false);
              }}
            >
              <span className="mail-row-prio" aria-hidden />
              <span className="mail-row-main">
                <span className="mail-row-top">
                  <strong>{folder === "SENT" ? t("mailTo") : nameOf(m.from_sub)}</strong>
                  <time dateTime={m.created_at}>{formatShort(m.created_at)}</time>
                </span>
                <span className="mail-row-subject">
                  {m.kind === "ANNOUNCEMENT" ? (
                    <span className="mail-ann-tag">{t("mailAnnouncement")}</span>
                  ) : null}
                  {m.subject}
                </span>
                <span className="mail-row-preview">{m.body.slice(0, 80)}</span>
              </span>
            </button>
          ))}
        </div>

        <div className="mail-reading">
          {composing ? (
            <ComposeForm
              canAnnounce={!!canAnnounce}
              directory={directory.data ?? []}
              onCancel={() => setComposing(false)}
              onSent={() => {
                setComposing(false);
                setFolder("SENT");
                void qc.invalidateQueries({ queryKey: ["mail"] });
              }}
            />
          ) : selected ? (
            <article className={`mail-detail prio-${(selected.priority_color || "blue").toLowerCase()}`}>
              <header>
                <div className="mail-detail-meta">
                  <span className={`mail-prio-chip prio-${selected.priority.toLowerCase()}`}>
                    {t(PRIORITY_KEYS[selected.priority])}
                  </span>
                  {selected.kind === "ANNOUNCEMENT" ? (
                    <span className="mail-ann-tag">{t("mailAnnouncement")}</span>
                  ) : null}
                  {selected.expires_at ? (
                    <span className="muted">
                      {t("mailExpires")}: {formatShort(selected.expires_at)}
                      {selected.expired ? ` · ${t("mailExpired")}` : ""}
                    </span>
                  ) : null}
                </div>
                <h2>{selected.subject}</h2>
                <p className="muted">
                  {t("mailFrom")}: {nameOf(selected.from_sub)}
                  {selected.from_operator ? ` (${selected.from_operator})` : ""}
                  {" · "}
                  <time dateTime={selected.created_at}>{new Date(selected.created_at).toLocaleString()}</time>
                </p>
              </header>
              <div className="mail-body">{selected.body}</div>
              {folder === "INBOX" ? (
                <footer className="mail-actions">
                  <button
                    type="button"
                    className="btn"
                    onClick={() => archive.mutate(selected.id)}
                    disabled={archive.isPending}
                  >
                    {t("mailArchiveAction")}
                  </button>
                </footer>
              ) : null}
            </article>
          ) : (
            <p className="muted mail-placeholder">{t("mailSelect")}</p>
          )}
        </div>
      </div>
    </section>
  );
}

function ComposeForm({
  canAnnounce,
  directory,
  onCancel,
  onSent,
}: {
  canAnnounce: boolean;
  directory: DirectoryUser[];
  onCancel: () => void;
  onSent: () => void;
}) {
  const t = useLocaleStore((s) => s.t);
  const me = useAuthStore((s) => s.claims?.sub);
  const [subject, setSubject] = useState("");
  const [body, setBody] = useState("");
  const [priority, setPriority] = useState<Priority>("NORMAL");
  const [kind, setKind] = useState<Kind>("MESSAGE");
  const [toSubs, setToSubs] = useState<string[]>([]);
  const [expiresDays, setExpiresDays] = useState(7);
  const [error, setError] = useState("");

  const send = useMutation({
    mutationFn: async () => {
      const payload: Record<string, unknown> = {
        subject,
        body,
        priority,
        kind,
        to_subs: kind === "ANNOUNCEMENT" && toSubs.length === 0 ? [] : toSubs,
      };
      if (kind === "ANNOUNCEMENT") {
        const exp = new Date();
        exp.setDate(exp.getDate() + expiresDays);
        payload.expires_at = exp.toISOString();
      }
      const res = await apiFetch("/mail/messages", {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify(payload),
      });
      if (!res.ok) {
        const raw = await res.text();
        throw new Error(raw || "send_failed");
      }
      return res.json();
    },
    onSuccess: () => onSent(),
    onError: () => setError(t("mailSendError")),
  });

  const recipients = directory.filter((u) => u.sub !== me);

  return (
    <form
      className="mail-compose"
      onSubmit={(e) => {
        e.preventDefault();
        setError("");
        if (!subject.trim()) {
          setError(t("mailSubjectRequired"));
          return;
        }
        if (kind === "MESSAGE" && toSubs.length === 0) {
          setError(t("mailRecipientsRequired"));
          return;
        }
        send.mutate();
      }}
    >
      <h2>{t("mailCompose")}</h2>
      {canAnnounce ? (
        <label className="mail-field">
          <span>{t("mailKind")}</span>
          <select
            value={kind}
            onChange={(e) => setKind(e.target.value as Kind)}
          >
            <option value="MESSAGE">{t("mailKindMessage")}</option>
            <option value="ANNOUNCEMENT">{t("mailKindAnnouncement")}</option>
          </select>
        </label>
      ) : null}

      {kind === "MESSAGE" || (kind === "ANNOUNCEMENT" && toSubs.length > 0) ? (
        <fieldset className="mail-field">
          <legend>{t("mailTo")}</legend>
          <div className="mail-recipients">
            {recipients.map((u) => {
              const checked = toSubs.includes(u.sub);
              return (
                <label key={u.sub} className="mail-recipient">
                  <input
                    type="checkbox"
                    checked={checked}
                    onChange={() =>
                      setToSubs((prev) =>
                        checked ? prev.filter((s) => s !== u.sub) : [...prev, u.sub],
                      )
                    }
                  />
                  <span>{u.name}</span>
                </label>
              );
            })}
          </div>
          {kind === "ANNOUNCEMENT" ? (
            <p className="muted">{t("mailAnnounceAllHint")}</p>
          ) : null}
        </fieldset>
      ) : kind === "ANNOUNCEMENT" ? (
        <p className="muted">{t("mailAnnounceAllHint")}</p>
      ) : null}

      {kind === "ANNOUNCEMENT" ? (
        <label className="mail-field">
          <span>{t("mailExpiresDays")}</span>
          <input
            type="number"
            min={1}
            max={90}
            value={expiresDays}
            onChange={(e) => setExpiresDays(Number(e.target.value) || 7)}
          />
        </label>
      ) : null}

      <label className="mail-field">
        <span>{t("mailPriority")}</span>
        <select value={priority} onChange={(e) => setPriority(e.target.value as Priority)}>
          {(["LOW", "NORMAL", "HIGH", "URGENT"] as Priority[]).map((p) => (
            <option key={p} value={p}>
              {t(PRIORITY_KEYS[p])}
            </option>
          ))}
        </select>
      </label>

      <label className="mail-field">
        <span>{t("mailSubject")}</span>
        <input value={subject} onChange={(e) => setSubject(e.target.value)} required />
      </label>

      <label className="mail-field">
        <span>{t("mailBody")}</span>
        <textarea rows={8} value={body} onChange={(e) => setBody(e.target.value)} />
      </label>

      {error ? <p className="error">{error}</p> : null}

      <div className="mail-actions">
        <button type="button" className="btn" onClick={onCancel}>
          {t("mailCancel")}
        </button>
        <button type="submit" className="btn primary" disabled={send.isPending}>
          {send.isPending ? t("mailSending") : t("mailSend")}
        </button>
      </div>
    </form>
  );
}

function formatShort(iso: string) {
  try {
    const d = new Date(iso);
    return d.toLocaleDateString(undefined, { month: "short", day: "numeric" });
  } catch {
    return iso;
  }
}
