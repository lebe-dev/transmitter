# Transmitter

![Transmitter Screenshot](screenshot.png)
![Transmitter Screenshot (Dunkel)](screenshot-dark.png)

Transmitter ist eine moderne, schlanke Alternative zur Standard-Weboberfläche von Transmission. Läuft ohne externe Abhängigkeiten. Beinhaltet außerdem eine Telegram-Bot-Integration.

## Funktionen

- **Torrent-Liste** — sortierbare Tabelle: Name, Status, Fortschritt, Größe, Geschwindigkeit, Hinzugefügt, ETA
- **Statusfilter** — Alle, Lädt herunter, Seeding, Pausiert, Fertig
- **Suche** — Torrents nach Name filtern (Groß-/Kleinschreibung ignoriert)
- **Torrents hinzufügen** — Magnet-Links oder .torrent-Datei-Upload
- **Verwaltung** — Torrents pausieren, fortsetzen, löschen
- **Auto-Aktualisierung** — Live-Updates alle 3–5 Sekunden
- **Unterstützte Sprachen**: en, ru, es, de

## Erste Schritte

```bash
cp .env.example .env

# .env nach Bedarf anpassen

docker-compose up -d
```

Browser öffnen: `http://localhost:8080`

### Konfiguration

Alle Einstellungen über Umgebungsvariablen:

| Variable | Erforderlich | Standard |
|-----------|--------------|---------|
| `TRANSMISSION_USER` | Ja | — |
| `TRANSMISSION_PASS` | Ja | — |
| `TRANSMISSION_URL` | Nein | `http://localhost:9091/transmission/rpc` |
| `LISTEN_ADDR` | Nein | `:8080` |
| `TELEGRAM_TOKEN` | Nein | (Bot deaktiviert wenn leer) |
| `TELEGRAM_USERS` | Bei Bot-Nutzung | — |

Alle Optionen siehe [.env.example](.env.example).

## Sicherheit

Siehe [SECURITY.md](docs/SECURITY.de.md).

## Roadmap

- UX:
  - Pfade vorschlagen
  - Pfade merken
- Funktionen umschalten: Web-UI, Telegram-Bot
- Gruppierung nach Ordnern / Labels (Labels in Transmission 4.0 hinzugefügt)
- Pausieren / Löschen über Telegram-Bot (Befehlserweiterung)
- Telegram-Benachrichtigungen bei Torrent-Abschluss (Polling + Statusdiff)
- WebSocket statt Polling für Echtzeit-UI-Updates
- Web-UI-Authentifizierung (Basic Auth Middleware) für externen Zugriff über VPN
- RSS-Feeds für automatisches Hinzufügen von Torrents
- Unterstützung mehrerer Transmission-Instanzen
