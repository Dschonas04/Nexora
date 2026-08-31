# Grafana

`nexora.json` ist ein fertiges Bild für Prometheus als Datenquelle. Vier Reihen:
auf einen Blick, Verkehr, die Datenbank, Bestand und Anmeldungen.

## Voraussetzung

Ein Losungswort in `config.conf` oder in der Umgebung, sonst gibt es den Weg
nicht:

```
metriken_token = <etwas Langes>
```

Dann in Prometheus:

```yaml
  - job_name: 'nexora'
    metrics_path: /metrics
    authorization:
      credentials: '<dasselbe Losungswort>'
    static_configs:
      - targets: ['10.0.0.1:3000']
```

## Einspielen

In Grafana unter *Dashboards → New → Import* die Datei hochladen und die
Prometheus-Datenquelle wählen. Wer Grafana über Dateien versorgt, legt sie
stattdessen in das bereitgestellte Verzeichnis und ersetzt `${DS_PROMETHEUS}`
durch die Kennung der Datenquelle.

## Wonach man sieht

**Verbindungen zum Vorrat** ist das Feld, das eine Störung erklärt, die sonst
niemand versteht. pgx nimmt ohne Angabe so viele Verbindungen wie Kerne. Sind
alle belegt, wartet jede weitere Anfrage — von außen sieht das aus wie eine
langsame Datenbank, obwohl die Datenbank Langeweile hat. Klebt die Belegung an
der gestrichelten Obergrenze, während daneben die Wartezeit steigt, ist die
Sache erklärt: `pool_max_conns` in `DATABASE_URL` heben.

**Aus dem Speicher beantwortet** beantwortet die andere Frage, die man
zwangsläufig stellt: ob mehr Arbeitsspeicher etwas brächte. Solange die Linie
über 95 Prozent liegt, liest PostgreSQL ohnehin aus dem Speicher, und
`shared_buffers` zu erhöhen bringt nichts.

**Anfragen nach Ausgang** trennt abgewiesen (4xx) von gescheitert (5xx). Eine
Instanz, die fleißig 401 verteilt, tut genau das, was sie soll; nur die rote
Fläche ist ein Fehler.

## Zu den Farben

Die drei Reihenfarben sind mit einem Prüfer gegen Farbfehlsichtigkeit
durchgerechnet, in beiden Grafana-Themen. Die Reihenfolge ist nicht beliebig:
Blau neben Violett fällt bei Rotgrünblindheit zusammen, Bernstein dazwischen
löst das. Die Zustandsfarben grün/gelb/rot sind dafür reserviert und werden nie
als weitere Datenreihe benutzt.
