# Sicherheit

## Designprinzipien

Transmitter ist für die **Heimnetzwerk-Bereitstellung** konzipiert, bei der alle Benutzer als vertrauenswürdig gelten. Sicherheitsfunktionen sind implementiert, um versehentliche Offenlegung zu verhindern und Zugangsdaten zu schützen.

## Authentifizierung & Autorisierung

- **Web-UI:** keine Benutzerauthentifizierung. Nur im Heimnetzwerk zugänglich. Falls später externer Zugriff benötigt wird, Middleware mit Basic Auth oder Cookie-Session hinzufügen.
- **Telegram-Bot:** Whitelist nach Benutzer-ID. Unbefugte Nachrichten werden ignoriert (keine Antwort, Protokollierung auf warn-Ebene).

## Zugangsdatenverwaltung

- **Transmission RPC:** Zugangsdaten werden in der `.env`-Datei und Container-Umgebungsvariablen gespeichert. Werden niemals an das Frontend gesendet — serverseitig in Go weitergeleitet.
- Zugangsdaten werden über den Header `Authorization: Basic <base64>` injiziert (HTTP Basic Auth)
- Umgebungsvariablen werden über `godotenv` geladen, nicht in Logs exponiert

## CSRF-Schutz

- **Transmission RPC:** verwendet einen eigenen CSRF-Mechanismus (Header `X-Transmission-Session-Id`)
- Der Go-Proxy verwaltet den Token-Lebenszyklus transparent:
  1. Holt Session-ID bei der ersten Anfrage (10s Timeout)
  2. Bei 409 Conflict extrahiert neue ID aus dem Header
  3. Wiederholt die Anfrage mit frischem Token (ein Wiederholungsversuch)
  4. Gültigen Token im Speicher cachen (`atomic.Value` mit singleflight)

## Netzwerksicherheit

- **CORS:** explizite Origin-Prüfung (keine Wildcards), verhindert DNS-Rebinding-Angriffe
- `network_mode: host` in Docker — vereinfacht localhost-Zugriff auf Transmission
- Keine externe Exposition — setzt privates Netzwerk voraus

## Anfragenvalidierung

- **RPC-Whitelist:** nur 6 Methoden erlaubt:
  - `torrent-get`, `torrent-add`, `torrent-start`, `torrent-stop`, `torrent-remove`, `session-get`
- **Anfragegrößenlimit:** max. 1 MB Payload (verhindert DoS)
- Direktes JSON-RPC-Weiterleiten (kein Benutzereingabe in RPC-Aufrufen außer Torrent-Daten)

## Empfehlungen für externen Zugriff

Wenn Transmitter im Internet oder in nicht vertrauenswürdigen Netzwerken betrieben wird:

1. **Authentifizierung hinzufügen** — Basic Auth oder Session-Middleware in Go
2. **HTTPS verwenden** — Reverse Proxy mit TLS (nginx, Caddy, etc.)
3. **IP-Bereich einschränken** — Firewall- oder Reverse-Proxy-Regeln
4. **VPN/Tunnel** — SSH-Port-Weiterleitung oder WireGuard für Fernzugriff
5. **Rate Limiting** — Ratenbegrenzung auf dem `/api/rpc` Endpunkt implementieren
6. **Standardwerte ändern** — starke Transmission-Zugangsdaten sicherstellen

## Datenschutz

- Alle Transmission-Daten (Torrent-Dateien, Zugangsdaten) werden serverseitig verarbeitet
- Frontend erhält nur: Torrent-Metadaten, Status, Fortschritt
- Session-Tokens werden nicht an den Client weitergegeben
- Telegram-Interaktionen werden auf dem konfigurierten Level protokolliert
