# Transmitter

![Captura de pantalla de Transmitter](screenshot.png)
![Captura de pantalla de Transmitter (modo oscuro)](screenshot-dark.png)

Transmitter es una alternativa moderna y ligera a la interfaz web estándar de Transmission. Funciona sin dependencias externas. También incluye integración con bot de Telegram.

## Características

- **Lista de torrents** — tabla ordenable: nombre, estado, progreso, tamaño, velocidad, fecha de adición, ETA
- **Filtros por estado** — Todos, Descargando, Sembrando, Pausado, Completado
- **Búsqueda** — filtra torrents por nombre (sin distinción de mayúsculas)
- **Añadir torrents** — enlaces magnet o carga de archivos .torrent
- **Gestión** — pausar, reanudar, eliminar torrents
- **Auto-actualización** — actualizaciones en vivo cada 3–5 segundos
- **Idiomas soportados**: en, ru, es, de

## Primeros pasos

```bash
cp .env.example .env

# edita .env según tus necesidades

docker-compose up -d
```

Abre el navegador: `http://localhost:8080`

### Configuración

Todos los ajustes se realizan mediante variables de entorno:

| Variable | Requerida | Por defecto |
|-----------|-----------|-------------|
| `TRANSMISSION_USER` | Sí | — |
| `TRANSMISSION_PASS` | Sí | — |
| `TRANSMISSION_URL` | No | `http://localhost:9091/transmission/rpc` |
| `LISTEN_ADDR` | No | `:8080` |
| `TELEGRAM_TOKEN` | No | (bot desactivado si está vacío) |
| `TELEGRAM_USERS` | Si se usa el bot | — |

Para todas las opciones, consulta [.env.example](.env.example).

## Seguridad

Ver [SECURITY.md](docs/SECURITY.es.md).

## Hoja de ruta

- UX:
  - Sugerir rutas
  - Recordar rutas
- Alternar funciones: interfaz web, bot de Telegram
- Agrupación por carpetas / etiquetas (etiquetas añadidas en Transmission 4.0)
- Pausar / eliminar mediante bot de Telegram (expansión de comandos)
- Notificaciones de Telegram al completar la descarga (sondeo + diferencia de estado)
- WebSocket en lugar de sondeo para actualizaciones en tiempo real
- Autenticación en la interfaz web (middleware Basic Auth) para acceso externo vía VPN
- Feeds RSS para adición automática de torrents
- Soporte de múltiples instancias de Transmission
