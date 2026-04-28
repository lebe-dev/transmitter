# Transmitter

![Captura de pantalla de Transmitter](screenshot.png)
![Detalles del torrent](screenshot2.png)

Transmitter es una alternativa moderna y ligera a la interfaz web estándar de Transmission. Funciona sin dependencias externas. También incluye integración con bot de Telegram.

## Características

- **Lista de torrents** — tabla ordenable: nombre, estado, progreso, tamaño, velocidad, fecha de adición, ETA
- **Filtros por estado** — Todos, Descargando, Sembrando, Pausado, Completado
- **Búsqueda** — filtra torrents por nombre (sin distinción de mayúsculas)
- **Añadir torrents** — enlaces magnet o carga de archivos .torrent
- **Gestión** — pausar, reanudar, eliminar torrents
- **Auto-actualización** — actualizaciones en vivo cada 3–5 segundos
- **Idiomas soportados**: en, ru, es, de
- **Docker images**: linux/amd64, linux/arm/v7, linux/arm64/v8

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
| `CORS_ORIGIN` | No | `http://localhost:8080` |
| `WEBUI_ENABLED` | No | `true` |
| `TELEGRAM_BOT_ENABLED` | No | `false` |
| `TELEGRAM_TOKEN` | Si se usa el bot | — |
| `TELEGRAM_USERS` | Si se usa el bot | — |
| `LOG_LEVEL` | No | `info` |
| `FILE_PRIORITY_ENABLED` | No | `false` |
| `FILE_PRIORITY_HIGH_COUNT` | No | `3` |

Para todas las opciones, consulta [.env.example](.env.example).

## Seguridad

Ver [SECURITY.md](docs/SECURITY.es.md).

## Hoja de ruta

- Autenticación en la interfaz web
- Video plugin
- Soporte de múltiples instancias de Transmission
- Feeds RSS para adición automática de torrents
