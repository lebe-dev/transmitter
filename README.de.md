# Transmitter

![Transmitter Screenshot](screenshot.png)
![Torrent-Details](screenshot2.png)

Transmitter ist eine moderne, schlanke Alternative zur Standard-Weboberfläche von Transmission. Läuft ohne externe Abhängigkeiten. Beinhaltet außerdem eine Telegram-Bot-Integration.

## Funktionen

- **Grundfunktionen** — sortierbare Liste (Name, Status, Fortschritt, Größe, Geschwindigkeit, Hinzugefügt, ETA) mit Statusfiltern, Suche nach Name ohne Beachtung der Groß-/Kleinschreibung, Hinzufügen per Magnet-Link oder .torrent-Datei sowie Pausieren/Fortsetzen/Löschen
- **Notizen** — beliebiger Text zu jedem Torrent (serverseitig in SQLite gespeichert, sichtbar in der Liste, im Detailbereich und im Telegram-Bot)
- **Nachtschicht** — markierte Torrents werden nur im konfigurierten Zeitfenster heruntergeladen und geseedet; außerhalb werden sie pausiert
- **Tagschicht** — dasselbe mit eigener Markierung, eigenem Fenster und einem Sonnen-Button; die Schichten sind unabhängig, ein Torrent darf beide Markierungen tragen
- **Schicht-Schalter** — jede konfigurierte Schicht lässt sich unter „Einstellungen → Schichten“ ausschalten; der Zustand liegt serverseitig in SQLite, gilt daher in jedem Browser und bleibt nach einem Neustart erhalten. Solange eine Schicht aus ist, rührt ihr Scheduler nichts an: Torrents behalten ihren Zustand und der Schicht-Button wird ausgegraut
- **Impact** — wie viel ein Torrent von dem zurückgegeben hat, was er genommen hat. Ein Pokal markiert die Torrents, die 100% ihres Downloads hochgeladen haben, der Filter „Nicht zurückgegeben“ zeigt den Rest (der sich sammelweise einer Schicht zuweisen lässt), und ein optionaler Schalter stoppt einen Torrent, sobald er das Ziel erreicht
- **Globale Pause** — ein Button in der Kopfzeile pausiert alle laufenden Torrents und merkt sich, welche es waren; beim Fortsetzen werden nur diese gestartet, von dir gestoppte Torrents bleiben gestoppt. Der Zustand wird serverseitig gespeichert und übersteht einen Neustart; Schichten ruhen, solange die Pause aktiv ist
- **Notizen** — ein freier Text oben auf dem Hauptbildschirm (standardmäßig 3 Zeilen, wächst mit dem Text), automatisch in SQLite gespeichert
- **Unterstützte Sprachen**: en, ru, es, de
- **Docker images**: linux/amd64, linux/arm/v7, linux/arm64/v8

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
| `LISTEN_ADDR` | Nein | `127.0.0.1:8080` (Docker-Image: `0.0.0.0:8080`) |
| `CORS_ORIGIN` | Nein | `http://localhost:8080` |
| `WEBUI_ENABLED` | Nein | `true` |
| `TELEGRAM_BOT_ENABLED` | Nein | `false` |
| `TELEGRAM_TOKEN` | Bei Bot-Nutzung | — |
| `TELEGRAM_USERS` | Bei Bot-Nutzung | — |
| `LOG_LEVEL` | Nein | `info` |
| `FILE_PRIORITY_ENABLED` | Nein | `false` |
| `FILE_PRIORITY_HIGH_COUNT` | Nein | `3` |
| `NIGHT_SHIFT_START` | Nein | — (deaktiviert) |
| `NIGHT_SHIFT_END` | Nein | — (deaktiviert) |
| `NIGHT_SHIFT_INTERVAL` | Nein | `1m` |
| `DAY_SHIFT_START` | Nein | — (deaktiviert) |
| `DAY_SHIFT_END` | Nein | — (deaktiviert) |
| `DAY_SHIFT_INTERVAL` | Nein | `1m` |
| `DB_PATH` | Nein | `data/transmitter.db` (Docker-Image: `/app/data/transmitter.db`) |
| `TORRENT_NOTE_MAX_LENGTH` | Nein | `200` |
| `TORRENT_NOTE_CLEANUP_INTERVAL` | Nein | `1h` |
| `IMPACT_TARGET_RATIO` | Nein | `1` |
| `IMPACT_INTERVAL` | Nein | `5m` |
| `SENTRY_DSN` | Nein | — (deaktiviert) |
| `SENTRY_ENVIRONMENT` | Bei Verwendung von Sentry | — |

Alle Optionen siehe [.env.example](.env.example).

## Sicherheit

Siehe [SECURITY.md](docs/SECURITY.de.md).

## Roadmap

- Web-UI-Authentifizierung
- Video plugin
- Unterstützung mehrerer Transmission-Instanzen
- RSS-Feeds für automatisches Hinzufügen von Torrents
