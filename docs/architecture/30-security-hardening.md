# 30 — Hardening HTTP y prevención de inyección

## Capas de seguridad (estado actual)

| Capa | Qué hace | Dónde |
|---|---|---|
| 1. AuthN | JWT (dev bypass / Keycloak) | Gateway |
| 2. AuthZ | PEP → OPA (RBAC + ABAC) | Gateway + servicios |
| 3. Datos | RLS por `org_id` / `branch_id` + SQL parametrizado | PostgreSQL / stores |
| 4. **Nueva** | Headers HTTP, límite de body, Content-Type, sanitización anti-XSS/SQLi | `packages/go/secure` + SPA |

La capa 4 **no sustituye** prepared statements ni OPA: es defensa en profundidad (reject-early + headers de navegador).

## Paquete `packages/go/secure`

| API | Uso |
|---|---|
| `PlainText` / `PlainTextMax` | Quita tags HTML, esquemas peligrosos y controles |
| `SafeURL` | Solo `http`/`https` absolutas (bloquea `javascript:`, `data:`, …) |
| `SafeCSSColor` | Solo `#RGB` / `#RRGGBB` / `#RRGGBBAA` |
| `LooksLikeInjection` / `RejectIfInjection` | Rechaza probes XSS/SQLi obvios |
| `SecurityHeadersMiddleware` | `X-Content-Type-Options`, `X-Frame-Options`, CSP API, COOP/CORP, HSTS si HTTPS |
| `LimitBodyMiddleware` | Tope por defecto 1 MiB en POST/PUT/PATCH |
| `RequireJSONMiddleware` | Mutaciones con JSON (permite multipart/imágenes y POST sin body) |

### Cableado

- **Gateway**: `middleware.Hardening(1<<20)` en todas las rutas.
- **Inventory / Messaging**: headers + body limit; sanitización en storefront, clientes/tarjetas, papeletas y mensajes.

## SPA

- `apps/web/src/security/sanitize.ts` — `safeUrl`, `safeCssColor`, `rejectIfInjection`.
- Storefront público: hero `backgroundImage` y colores CSS solo con valores sanitizados; `maps_url` vía `safeUrl`.
- Admin storefront: rechazo client-side antes del PUT.
- `index.html`: CSP de documento (scripts self, fonts Google, `img-src` https/http para héroes externos).

## Qué se rechaza (ejemplos)

- Texto con `<script>`, `onerror=`, `javascript:`, `UNION SELECT`, `; DROP TABLE`.
- URLs de héroe / CTA / mapas que no sean http(s).
- Bodies > 1 MiB o `Content-Type` no soportado en mutaciones del gateway.

Texto benigno (p. ej. «Caja frágil — no apilar») **no** se marca.

## Relación con [02 — Seguridad RBAC/ABAC](./02-security-rbac-abac.md)

El doc 02 describe AuthN/AuthZ y el modelo de amenaza. Este doc detalla la **capa de endurecimiento de transporte e inputs** añadida encima.
