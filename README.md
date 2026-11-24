# cloudreve-unlocked

Fork open source di [Cloudreve](https://github.com/cloudreve/Cloudreve) che aggiunge, senza codice proprietario, le funzioni di gestione delle storage policy riservate a Cloudreve Pro.

## Cosa fa

- **Mount di una storage policy su una cartella**: i nuovi upload della cartella e delle sottocartelle usano la policy montata; vince la cartella più vicina con un mount. I file esistenti non vengono spostati.
- **Policy effettiva e montata** visibili nei dettagli della cartella.
- **Riposizionamento tra storage policy** (relocation):
  - dry-run con stima di file, entità, versioni, thumbnail e spazio;
  - sposta entità primaria, versioni e thumbnail;
  - supporta sorgenti e destinazioni cifrate;
  - clona le entità condivise senza toccare gli altri file;
  - scelta tra mantenere o eliminare la copia di origine dopo il trasferimento;
  - cleanup automatico periodico degli oggetti di origine trattenuti (`delete_source=false`),
    con retention configurabile e endpoint admin per consultazione e pulizia forzata.
- **Pipeline i18n**: cache delle traduzioni versionata per build e controllo delle chiavi richieste in fase di build.

## Cosa manca

- load balancer policy;
- policy multiple per gruppo;
- regole avanzate di policy per directory;
- audit dettagliato per entità nella UI di relocation.

## Build

Requisiti: Go 1.26+, Node.js 22+, Python 3, `zip`.

```bash
./scripts/build-release.sh 4.19.1
```

Output: `dist/cloudreve-4.19.1-linux-amd64`, con il frontend incorporato nel binario.

## Licenza

GPL-3.0, vedi `cloudreve/LICENSE`. Fork indipendente, non affiliato a Cloudreve.
