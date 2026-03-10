# Guía de configuración de Transmission

Esta guía explica cómo configurar Transmission de forma segura para que Transmitter pueda conectarse correctamente.

## Cómo se conecta Transmitter

Transmitter se comunica con Transmission exclusivamente desde el **lado del servidor** — las credenciales nunca llegan al navegador. El backend de Go se autentica mediante `Authorization: Basic <base64>` en cada solicitud RPC y gestiona el token `X-Transmission-Session-Id` de forma transparente.

```
Navegador → POST /api/rpc → Servicio Transmitter (auth + lista blanca) → Transmission RPC
```

El proxy aplica una **lista blanca de métodos**: solo se reenvían `torrent-get`, `torrent-add`, `torrent-start`, `torrent-stop`, `torrent-remove` y `session-get`. El resto de métodos se rechazan con `403`.

## Obligatorio: habilitar autenticación RPC

Transmitter **requiere** que `TRANSMISSION_USER` y `TRANSMISSION_PASS` estén configurados. La configuración correspondiente de Transmission debe coincidir.

En `/etc/transmission-daemon/settings.json` (detén el daemon antes de editar):

```json
{
  "rpc-authentication-required": true,
  "rpc-username": "tu-usuario",
  "rpc-password": "tu-contraseña"
}
```

Transmission hashea la contraseña en el primer arranque — el texto plano que escribas será reemplazado por un hash como `{5d1a45db...`. Es el comportamiento esperado.

Establece las mismas credenciales en `.env`:

```env
TRANSMISSION_USER=tu-usuario
TRANSMISSION_PASS=tu-contraseña
```

## Opcional: lista blanca de IPs

`rpc-bind-address: "0.0.0.0"` es intencional si también usas la interfaz web integrada de Transmission desde otras máquinas de tu red local. La autenticación (descrita arriba) es la protección principal en este caso.

Como capa adicional, puedes restringir qué IPs pueden conectarse:

```json
{
  "rpc-whitelist-enabled": true,
  "rpc-whitelist": "127.0.0.1,192.168.88.*"
}
```

Los comodines se admiten por octeto. Ajusta la subred a tu red LAN.

## Nota sobre Docker

El Docker Compose de Transmitter usa `network_mode: host`, por lo que el contenedor comparte la pila de red del host. `TRANSMISSION_URL=http://localhost:9091/transmission/rpc` funciona sin cambios independientemente de `rpc-bind-address`.

## Diff recomendado de settings.json

Partiendo del ejemplo de configuración, el cambio mínimo requerido:

| Ajuste | Actual | Recomendado |
|---|---|---|
| `rpc-authentication-required` | `false` | `true` |
| `rpc-username` | `""` | tu usuario |
| `rpc-password` | `""` | tu contraseña (texto plano, se hashea al reiniciar) |

## Verificar la conexión

Tras aplicar los cambios, reinicia Transmission y comprueba el endpoint de salud de Transmitter:

```sh
curl http://localhost:8080/api/health
# {"status":"ok"}
```

Una respuesta `502` significa que Transmitter no puede llegar a Transmission — comprueba `TRANSMISSION_URL`, las credenciales y que Transmission esté en ejecución. Un `401` de Transmission indica que las credenciales no coinciden.

## No expongas Transmission RPC externamente

El puerto `9091` nunca debe estar abierto a internet. Transmitter es el único cliente previsto — proxifica, valida y autentica todas las solicitudes. Si necesitas acceso remoto, expón Transmitter (puerto `8080`) detrás de un proxy inverso con TLS, no Transmission directamente.
