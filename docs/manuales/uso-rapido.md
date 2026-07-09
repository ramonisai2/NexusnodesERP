# Manual de uso rápido — NexusERP

Guía corta para operar el sistema día a día. No sustituye la documentación técnica.

---

## 0. Instalar en tu PC (con dependencias)

Solo hace falta **Docker**. El instalador trae Postgres, OPA, backend y la web:

```bash
./infra/scripts/install.sh
```

Luego abre **http://localhost:8088**.  
(Si desarrollas con Vite a mano, la URL sigue siendo `http://localhost:5173`.)

## 1. Entrar

1. Abre la aplicación (instalador: `http://localhost:8088` · Vite: `http://localhost:5173`).
2. Elige idioma (ES / EN) arriba a la derecha.
3. Opciones típicas:
   - **Explorar modo demo** — recorre todo con datos de ejemplo (sin instalar tu tienda).
   - **Perfiles avanzados** — cajero, almacén, CEDI, admin, etc.
   - Si es el **primer uso** de una tienda nueva, te lleva a **Instalación** (`/setup`).

En la barra superior elige la **sucursal** activa (Norte, Sur, CEDI…). Cambia de sucursal antes de operar stock o caja.

---

## 2. Primera instalación (tienda nueva)

Ruta: `/setup`

| Paso | Qué haces |
|---|---|
| 1. Tu tienda | Nombre del negocio y sucursal |
| 2. Red | **Solo intranet** o **Con salida a internet** |
| 3. Funciones | Desbloquea solo lo que vas a usar |
| 4. Productos | Presets + productos propios |
| 5. Dueño | Tu nombre y correo |

### Red (importante)

| Modo | Úsalo si… | Efecto |
|---|---|---|
| **Solo intranet** | Solo red local / VPN | ERP interno. Sin vitrina pública ni portal de clientes en internet. |
| **Con salida a internet** | Quieres catálogo público | Además del ERP: `/tienda/{slug}` y registro/login de clientes. |

En intranet, **Tienda en línea** queda bloqueada automáticamente.

---

## 3. Menú principal (qué es cada cosa)

| Menú | Para qué |
|---|---|
| **Inicio** | Resumen y atajos |
| **Buscar** | Encontrar productos / reportes |
| **Inventario** | Existencias, etiquetas, precios |
| **Recepción** | Entrada de mercancía (tienda o CEDI) |
| **Merma / Robo** | Ajustes tipificados de stock |
| **Papeletas / Transporte / Traslados / Paquetería** | Logística entre sucursales y CEDI |
| **Caja** | Cobro con IVA y ticket |
| **Espera tarjeta** | Cola hasta que el terminal bancario autorice |
| **Clientes** | Cuentas y tarjetas de cliente (staff) |
| **Tienda en línea** | Configurar vitrina pública (solo modo internet) |
| **Jefes depto** | Asignar jefes a departamentos |
| **Aprobaciones** | Cola de decisiones de jefes |
| **Correo** | Mensajería interna |
| **Recursos Humanos / Nómina** | Empleados y corridas |
| **Reportes / Webmaster / Seguridad / Fotos** | Consulta, impresión, sellos, evidencias |

Si un ítem aparece apagado: no tienes permiso o el módulo está bloqueado en la instalación.

---

## 4. Operaciones del día

### Caja (cobrador)

1. Entra como **Cobrador / caja** (o demo).
2. Ve a **Caja**.
3. Escanea o escribe el SKU → Enter.
4. Elige forma de pago:
   - **Efectivo** — captura recibido; el sistema calcula cambio.
   - **Tarjeta** — si el módulo de espera está activo, la venta **no** baja stock hasta autorizar en **Espera tarjeta**.
   - **Transferencia** — cobro directo.
5. Opcional: marca **Cliente quiere factura** (RFC + razón social).
6. **Cobrar e imprimir**.

**Espera de tarjeta:** en la cola, captura referencia de terminal y código de autorización → **Aprobar** (completa venta) o **Rechazar / Cancelar** (sin movimiento de inventario).

### Inventario y recepción

1. **Inventario** — consulta existencias por sucursal.
2. **Recepción** — captura entrada; en CEDI puedes usar flujo de **camión → tarimas → cajas**.
3. **Merma / Robo** — elige dirección (salida/entrada), motivo (MERMA, ROBO, etc.), SKU y cantidad.

### Logística

- **Papeletas** — notas imprimibles entre tiendas (tipo de contenedor).
- **Transporte** — manifiesto / hoja de viaje.
- **Traslados** — sale stock de origen y llega a destino.
- **Paquetería** — hub, garantías, defectuosos.
- **Seguridad** — verificación de sellos / reporte de andén.

### Tienda en línea (solo modo internet)

1. **Tienda en línea** → slug, marca, colores, contacto.
2. Marca **Publicada**.
3. El público ve `/tienda/{tu-slug}` sin login.
4. Los clientes pueden crear cuenta en `/tienda/{slug}/cuenta`.

### Reportes

- **Reportes** — consolidados operativos.
- **Reportes webmaster** — imprimir / exportar CSV-JSON de casi todo lo administrado.
- **Fotos** — reportes con imágenes; QR para subir desde el celular (sirve también en intranet/LAN).

---

## 5. Perfiles rápidos (demo)

| Quieres probar… | Entra como… |
|---|---|
| Todo el sistema | **Explorar modo demo** |
| Cobro | Cobrador / caja |
| Piso de ventas | Vendedor |
| Almacén de tienda | Empleado de almacén |
| CEDI / camiones | Empleado CEDI |
| Sellos / andén | Seguridad |
| Dueño de abarrotes | Dueño (o termina `/setup`) |
| Nómina (SoD) | Analista prepara → Aprobador aprueba |

Cambia de perfil con **Salir** y vuelve a entrar.

---

## 6. Buenas prácticas

1. Confirma la **sucursal** antes de cobrar o mover stock.
2. En intranet no intentes publicar la vitrina: el sistema lo bloquea a propósito.
3. Con tarjeta + módulo de espera: no cierres el turno sin revisar la **cola de terminal**.
4. Merma y robo: usa el motivo correcto; queda trazabilidad para RR.HH. / auditoría.
5. Nómina: quien prepara no debe ser quien aprueba (separación de funciones).
6. Correo interno es solo entre usuarios del ERP (no es Outlook de internet).

---

## 7. Problemas frecuentes

| Síntoma | Qué revisar |
|---|---|
| Menú en gris / “sin permiso” | Perfil o módulo bloqueado en instalación |
| No aparece Tienda en línea | Modo **intranet**, o módulo `storefront` apagado |
| No publica la vitrina | Necesitas modo **internet** |
| Cobro con tarjeta no baja stock | Está en **Espera tarjeta** pendiente de autorizar |
| Producto no se agrega en caja | Sin precio/etiqueta en esa sucursal |
| 403 al aprobar nómina | Mismo usuario preparó y aprueba (SoD) |

---

## 8. Dónde profundizar

| Tema | Documento |
|---|---|
| Arranque técnico local | [05 — Bootstrap](../architecture/05-bootstrap-guide.md) |
| Instalación y módulos | [38 — Módulos](../architecture/38-install-modules.md) |
| Intranet vs internet | [41 — Modo de red](../architecture/41-network-mode.md) |
| Caja e IVA | [32 — POS](../architecture/32-pos-caja-iva.md) |
| Espera de tarjeta | [40 — Pagos tarjeta](../architecture/40-card-payment-wait.md) |
| Perfiles y roles | [36 — Perfiles](../architecture/36-org-profiles.md) |
| Modo demo | [39 — Demo](../architecture/39-demo-mode.md) |
| Vitrina pública | [27 — Storefront](../architecture/27-configurable-storefront.md) |

---

*Última orientación: operación de tienda / CEDI. Para arquitectura y APIs usa `docs/architecture/`.*
