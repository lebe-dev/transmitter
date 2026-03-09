# Transmission Konfigurationsanleitung

Diese Anleitung erklärt, wie Transmission sicher konfiguriert wird, damit Transmitter sich korrekt verbinden kann.

## Wie Transmitter sich verbindet

Transmitter kommuniziert mit Transmission ausschließlich über die **Serverseite** — Zugangsdaten gelangen nie in den Browser. Das Go-Backend authentifiziert sich über `Authorization: Basic <base64>` bei jeder RPC-Anfrage und verwaltet den `X-Transmission-Session-Id`-Token transparent.

```
Browser → POST /api/rpc → Transmitter-Dienst (Auth + Whitelist) → Transmission RPC
```

Der Proxy erzwingt eine **Methoden-Whitelist**: nur `torrent-get`, `torrent-add`, `torrent-start`, `torrent-stop`, `torrent-remove` und `session-get` werden weitergeleitet. Alle anderen Methoden werden mit `403` abgelehnt.

## Erforderlich: RPC-Authentifizierung aktivieren

Transmitter **erfordert**, dass `TRANSMISSION_USER` und `TRANSMISSION_PASS` gesetzt sind. Die entsprechende Transmission-Einstellung muss übereinstimmen.

In `/etc/transmission-daemon/settings.json` (Daemon vor dem Bearbeiten stoppen):

```json
{
  "rpc-authentication-required": true,
  "rpc-username": "dein-benutzername",
  "rpc-password": "dein-passwort"
}
```

Transmission hasht das Passwort beim ersten Start — der von dir eingegebene Klartext wird durch einen Hash wie `{5d1a45db...` ersetzt. Das ist das erwartete Verhalten.

Passende Zugangsdaten in `.env` setzen:

```env
TRANSMISSION_USER=dein-benutzername
TRANSMISSION_PASS=dein-passwort
```

## Optional: IP-Whitelist

`rpc-bind-address: "0.0.0.0"` ist beabsichtigt, wenn du auch die eingebaute Transmission-Weboberfläche von anderen Maschinen in deinem lokalen Netzwerk nutzt. Authentifizierung (oben beschrieben) ist in diesem Fall der primäre Schutz.

Als zusätzliche Schicht kannst du einschränken, welche IPs sich verbinden dürfen:

```json
{
  "rpc-whitelist-enabled": true,
  "rpc-whitelist": "127.0.0.1,192.168.88.*"
}
```

Wildcards werden pro Oktett unterstützt. Subnetz an dein LAN anpassen.

## Docker-Hinweis

Transmitters Docker Compose verwendet `network_mode: host`, sodass der Container den Netzwerk-Stack des Hosts teilt. `TRANSMISSION_URL=http://localhost:9091/transmission/rpc` funktioniert ohne Änderungen unabhängig von `rpc-bind-address`.

## Empfohlenes settings.json Diff

Ausgehend von der Beispielkonfiguration die minimal erforderliche Änderung:

| Einstellung | Aktuell | Empfohlen |
|---|---|---|
| `rpc-authentication-required` | `false` | `true` |
| `rpc-username` | `""` | dein Benutzername |
| `rpc-password` | `""` | dein Passwort (Klartext, wird beim Neustart gehasht) |

## Verbindung überprüfen

Nach dem Anwenden der Änderungen Transmission neu starten und den Transmitter-Health-Endpunkt prüfen:

```sh
curl http://localhost:8080/api/health
# {"status":"ok"}
```

Eine `502`-Antwort bedeutet, dass Transmitter Transmission nicht erreichen kann — `TRANSMISSION_URL`, Zugangsdaten und ob Transmission läuft prüfen. Ein `401` von Transmission bedeutet falsche Zugangsdaten.

## Transmission RPC nicht extern exponieren

Port `9091` sollte niemals für das Internet geöffnet sein. Transmitter ist der einzige vorgesehene Client — er proxifiziert, validiert und authentifiziert alle Anfragen. Wenn Fernzugriff benötigt wird, Transmitter (Port `8080`) hinter einem Reverse Proxy mit TLS exponieren, nicht Transmission direkt.
