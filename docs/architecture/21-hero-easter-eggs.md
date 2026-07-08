# 21 — Easter eggs: héroes de la infancia

Aliases con nombre de héroe sobre funciones reales. El comportamiento no cambia; el nombre documenta la metáfora.

| Héroe | Función | Lógica |
|---|---|---|
| He-Man | `ByThePowerOfGrayskull` | AuthZ: poder para actuar (OPA/local) |
| Spider-Man | `WithGreatPower` | Permiso = poder + responsabilidad |
| Batman | `DetectiveMode` / `BatComputer` | Tokenizar / normalizar texto de búsqueda |
| Goku | `Kamehameha` | Concentrar foto grande → JPEG web |
| Sailor Moon | `MoonPrismPower` | Token QR → hash (identidad secreta) |
| TMNT | `CowabungaDelta` | Signo de movimiento de stock |
| Thundercats | `SightBeyondSight` | Elegir almacén ARRIVAL/CEDI |
| Digimon | `DigivolveReceipt` | DRAFT → POSTED en recepción |

## Dónde viven

- Go: `packages/go/authz`, `apps/search/.../engine`, `apps/reports/internal/imaging`, `apps/reports/internal/uploadsession`, `apps/inventory/internal/store`
- SPA: `apps/web/src/eastereggs/heroes.ts` + panel en `AppShell` (Konami o escribir `digivolve` / `grayskull` / `cowabunga`)
- Índice: `packages/go/heroes/doc.go`

Los nombres públicos estables (`Allow`, `Process`, `HashToken`, `PostReceipt`, `Tokenize`) delegan a los aliases para no romper callers.
