/** Client-side injection / XSS guards (defense in depth; server also sanitizes). */

const INJECTION =
  /<\s*script\b|javascript\s*:|vbscript\s*:|data\s*:\s*text\/html|on(error|load|click|mouse\w*|focus|blur|submit|input)\s*=|expression\s*\(|url\s*\(\s*['"]?\s*javascript|<\s*iframe\b|<\s*object\b|<\s*embed\b|<\s*svg[^>]*onload|<\s*img[^>]+onerror|union\s+select\b|;\s*(drop|alter|truncate|insert|update|delete)\s+/i;

export function looksLikeInjection(value: string): boolean {
  return INJECTION.test(value);
}

export function plainText(value: string, max = 4000): string {
  return value
    .replace(/[\u0000-\u0008\u000B\u000C\u000E-\u001F\u007F]/g, "")
    .replace(/[<>]/g, "")
    .trim()
    .slice(0, max);
}

/** Safe http(s) URL for href / CSS background-image. Rejects javascript:, data:, etc. */
export function safeUrl(raw: string | undefined | null): string | undefined {
  if (!raw) return undefined;
  const t = raw.trim();
  if (!t || looksLikeInjection(t)) return undefined;
  try {
    const u = new URL(t);
    if (u.protocol !== "http:" && u.protocol !== "https:") return undefined;
    return u.toString();
  } catch {
    return undefined;
  }
}

export function safeCssColor(raw: string | undefined | null, fallback: string): string {
  if (!raw) return fallback;
  const t = raw.trim();
  if (/^#([0-9a-fA-F]{3}|[0-9a-fA-F]{6}|[0-9a-fA-F]{8})$/.test(t)) return t;
  if (/^rgba?\(\s*\d{1,3}\s*,\s*\d{1,3}\s*,\s*\d{1,3}(\s*,\s*(0|1|0?\.\d+))?\s*\)$/.test(t)) return t;
  if (/^hsla?\(\s*\d{1,3}(\.\d+)?\s*,\s*\d{1,3}%\s*,\s*\d{1,3}%(\s*,\s*(0|1|0?\.\d+))?\s*\)$/.test(t)) return t;
  return fallback;
}

export function rejectIfInjection(...values: Array<string | undefined | null>): string | null {
  for (const v of values) {
    if (v && looksLikeInjection(v)) return "Contenido rechazado: posible inyección de código";
  }
  return null;
}
