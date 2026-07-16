# PIN-SOS 🚨

Alerta ciudadana de emergencia. Una persona en peligro abre una web en su móvil, comparte su ubicación, describe la situación (opcionalmente con una foto) y envía una alerta. La alerta llega al instante a un panel de operaciones y a un chat de Telegram, donde un equipo puede verla, ubicarla en el mapa y marcar su estado (activa → atendida → cerrada).

Ligera, sin dependencias pesadas, desplegable en minutos. Pensada para funcionar en móviles con conexión pobre.

---

## Qué hace

- Formulario público (PWA) con geolocalización, mapa (Leaflet) y foto opcional.
- Panel protegido por token: lista de alertas con mapa, foto y cambio de estado.
- Notificación instantánea a Telegram con nombre, teléfono, descripción, enlace a Google Maps y foto.
- Retención automática: las alertas cerradas se borran (con su foto) pasado un plazo configurable.

## Arquitectura

```
Ciudadano (móvil/PWA)                 Equipo de respuesta
        │                                    │
        ▼                                    ▼
  POST /api/alert  ──► [ PIN-SOS (Go) ] ──► Panel  /api/alerts
                          │      │            (token Bearer)
                          │      └──► Telegram (bot)
                          ▼
                     SQLite + /uploads
```

Un solo binario Go. SQLite con WAL. Archivos subidos en disco. Sin base de datos externa.

---

## Privacidad y datos sensibles

PIN-SOS maneja datos de personas potencialmente en riesgo: nombre, teléfono, ubicación exacta y foto. Trátalo en consecuencia.

- **HTTPS siempre.** El panel usa un token en cabecera; sobre HTTP sería interceptable. `force_https` está activo en `fly.toml` — no lo desactives.
- **El token del panel es una credencial de alto valor.** Quien lo tenga ve todas las alertas. Genéralo con `openssl rand -hex 32`, no lo compartas por canales inseguros y rótalo si sospechas una filtración.
- **Retención mínima.** Por defecto las alertas *cerradas* se eliminan a los 30 días junto con su foto (`RETENTION_DAYS`). Una alerta cerrada guardada para siempre es un pasivo, no un activo.
- **Fotos con nombre aleatorio.** Se sirven en `/uploads/` sin autenticación, así que el nombre del archivo es impredecible (16 bytes aleatorios) y se envían con cabeceras que impiden que se ejecuten como contenido activo en el navegador.
- Considera cifrar el volumen donde vive `/data` si tu proveedor lo permite.

> Este proyecto no ofrece garantías. Si lo despliegas para uso real, revisa la legislación de protección de datos aplicable a tu país y caso de uso.

---

## Configuración

Copia `.env.example` a `.env` y rellena los valores.

| Variable | Requerida | Por defecto | Descripción |
|---|---|---|---|
| `PANEL_TOKEN` | sí | — | Token del panel (mín. 32 chars). `openssl rand -hex 32` |
| `PORT` | no | 8080 | Puerto HTTP |
| `DB_PATH` | no | /data/pinsos.db | Ruta de la base SQLite |
| `UPLOADS_DIR` | no | /data/uploads | Carpeta de fotos |
| `STATIC_DIR` | no | static | Carpeta de archivos web |
| `RETENTION_DAYS` | no | 30 | Días antes de purgar alertas cerradas |
| `RATE_PER_IP` | no | 5 | Alertas/min por IP |
| `RATE_GLOBAL` | no | 60 | Alertas/min en total |
| `TELEGRAM_BOT_TOKEN` | no | — | Token del bot (sin él, no hay notificación) |
| `TELEGRAM_CHAT_ID` | no | — | Chat destino de las alertas |

Sin las variables de Telegram, la app funciona igual pero no envía notificaciones (lo registra en el log).

---

## Uso local

```bash
export PANEL_TOKEN=$(openssl rand -hex 32)
export DB_PATH=./pinsos.db UPLOADS_DIR=./uploads STATIC_DIR=static
go run .
# abre http://localhost:8080  (formulario)
#      http://localhost:8080/panel.html  (panel — pide el token)
```

## Despliegue en Fly.io

```bash
fly volumes create pinsos_data --size 1
fly secrets set PANEL_TOKEN=$(openssl rand -hex 32)
fly secrets set TELEGRAM_BOT_TOKEN=xxxx TELEGRAM_CHAT_ID=xxxx
fly deploy
```

El workflow `.github/workflows/deploy.yml` despliega automáticamente al hacer push a `main` (requiere el secret `FLY_API_TOKEN` en GitHub).

---

## API

| Método | Ruta | Auth | Descripción |
|---|---|---|---|
| POST | /api/alert | pública (rate-limited) | Crea una alerta. Multipart: name, phone, description, lat, lng, photo (opcional) |
| GET | /api/alerts?status=&limit= | Bearer | Lista alertas |
| PATCH | /api/alerts/{id}/status | Bearer | Cambia estado. Body: `{"status":"ATENDIDA"}` (ACTIVA/ATENDIDA/CERRADA) |

---

## Desarrollo

```bash
go vet ./...
go test ./...
```

Los tests cubren creación de alertas, rechazo de archivos que no son imágenes reales, autenticación del panel, validación de estados y el rate limiter.

## Stack

Go · SQLite (WAL) · Leaflet · Telegram Bot API · PWA (service worker) · Fly.io

## Licencia

MIT
