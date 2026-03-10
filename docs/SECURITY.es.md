# Seguridad

## Principios de diseño

Transmitter está diseñado para **implementación en red doméstica** donde todos los usuarios son de confianza. Las funciones de seguridad están implementadas para prevenir la exposición accidental y proteger las credenciales.

## Autenticación y autorización

- **Interfaz web:** sin autenticación de usuario. Accesible solo en la red doméstica. Si más adelante se necesita acceso externo, añade middleware con Basic Auth o sesión por cookie.
- **Bot de Telegram:** lista blanca por ID de usuario. Los mensajes no autorizados se ignoran (sin respuesta, registrado en nivel warn).

## Gestión de credenciales

- **Transmission RPC:** credenciales almacenadas en el archivo `.env` y variables de entorno del contenedor. Nunca se envían al frontend — se proxifican en el servidor mediante Go.
- Las credenciales se inyectan con el encabezado `Authorization: Basic <base64>` (HTTP Basic Auth)
- Las variables de entorno se cargan via `godotenv`, no se exponen en los registros

## Protección CSRF

- **Transmission RPC:** utiliza su propio mecanismo CSRF (encabezado `X-Transmission-Session-Id`)
- El proxy Go gestiona el ciclo de vida del token de forma transparente:
  1. Obtiene el session ID en la primera solicitud (timeout de 10s)
  2. Ante respuesta 409 Conflict, extrae el nuevo ID del encabezado
  3. Reintenta la solicitud con el token fresco (un reintento)
  4. Almacena en caché el token válido en memoria (`atomic.Value` con singleflight)

## Seguridad de red

- **CORS:** comprobación explícita del origen (sin comodines), previene ataques de DNS rebinding
- `network_mode: host` en Docker — simplifica el acceso por localhost a Transmission
- Sin exposición externa — asume red privada

## Validación de solicitudes

- **Lista blanca RPC:** solo se permiten 6 métodos:
  - `torrent-get`, `torrent-add`, `torrent-start`, `torrent-stop`, `torrent-remove`, `session-get`
- **Límite de tamaño de solicitud:** máximo 1 MB (previene DoS)
- Reenvío directo de JSON-RPC (sin entrada de usuario en llamadas RPC excepto datos del torrent)

## Recomendaciones para acceso externo

Si expones Transmitter a internet o redes no confiables:

1. **Añade autenticación** — Basic Auth o session middleware en Go
2. **Usa HTTPS** — proxy inverso con TLS (nginx, Caddy, etc.)
3. **Restringe el rango de IPs** — reglas en firewall o proxy inverso
4. **VPN/túnel** — reenvío de puertos SSH o WireGuard para acceso remoto
5. **Rate limiting** — implementa límites de velocidad en el endpoint `/api/rpc`
6. **Cambia los valores por defecto** — asegúrate de usar credenciales de Transmission seguras

## Privacidad de datos

- Todos los datos de Transmission (archivos torrent, credenciales) se gestionan en el servidor
- El frontend solo recibe: metadatos de torrents, estado, progreso
- Los tokens de sesión no se exponen al cliente
- Las interacciones de Telegram se registran al nivel configurado
