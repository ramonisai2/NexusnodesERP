# 17 — Panel de instalación para tienda pequeña

## Para qué sirve (lenguaje de negocio)

Una tienda de abarrotes no necesita “microservicios”, “OPA” ni “Tempo” el primer día.
Necesita: **nombre de la tienda**, **productos del día a día** y **un dueño** que pueda ver existencias.

El panel `/setup` hace exactamente eso en 3 pasos. El ERP completo (nómina, jerarquías, observabilidad) queda disponible después, sin abrumar.

## Cómo encaja Tempo / Grafana

| Capacidad técnica | Valor para la tienda pequeña |
|---|---|
| Trazas en Tempo | Si “no carga el inventario”, soporte ve la petición fallida sin pedirle logs al dueño |
| Dashboard Event Pipeline | Confirma que una salida de stock disparó el correo / notificación |
| Grafana Explore | Diagnóstico rápido cuando algo “se trabó” tras una actualización |

No se muestra en el menú del dueño: es **herramienta de soporte**, no del mostrador.

## Flujo del asistente

```
/setup
  1. Nombre del negocio + sucursal
  2. Funciones a desbloquear / bloquear (ver [38](./38-install-modules.md))
  3. Productos preset (leche, pan, arroz…) o personalizados
  4. Dueño (nombre + correo)
     → POST /setup/complete { enabled_modules }
     → org + branch + warehouse + store_owner + stock + etiquetas
     → auto-login (DEV_AUTH_BYPASS)
```

## API pública (sin JWT)

| Método | Ruta | Uso |
|---|---|---|
| `GET /setup/status` | ¿Hace falta instalar? |
| `GET /setup/presets` | Catálogo sugerido abarrotes |
| `POST /setup/complete` | Crea la tienda (`force: true` para reinstalar en demo) |

## Perfil `store_owner`

- Ve: Inicio (atajos), Inventario, Reportes, Fotos
- No ve: Nómina ni roles de jefe regional
- Permisos: balance/movimientos/catálogo/etiquetas + reporting/fotos

## Migración

`013_store_setup.sql` — `app_install`, `organizations.profile`, rol `store_owner`.
Si ya existe el seed `DEMO`, marca la instalación como completa (el asistente no bloquea el login demo).

## Arranque

```bash
make migrate
make run-gateway
make run-web
# Abrir http://localhost:5173/setup
```
