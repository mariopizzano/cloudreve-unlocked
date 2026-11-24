# cloudreve-unlocked

Fork open source di [Cloudreve](https://github.com/cloudreve/Cloudreve) 4.19.1 che implementa, senza codice proprietario, alcune funzioni della Cloudreve Pro Edition.

L'obiettivo del fork è coprire le aree **gestione storage** e **sistema**, che sono quelle indispensabili per un'istanza pubblica. Le altre aree Pro sono dichiarate esplicitamente come non implementate più sotto.

## Funzioni implementate

### Gestione storage

- **Policy per directory**: una storage policy può essere montata su una cartella; i nuovi upload della cartella e delle sottocartelle usano quella policy. Vince il mount più vicino; le cartelle senza mount ereditano dal genitore. Funziona anche via WebDAV.
- **Policy effettiva e montata** visibili nei dettagli della cartella.
- **Più policy per gruppo utente**: l'admin associa un pool di policy a un gruppo; l'utente sceglie la propria policy preferita tra quelle assegnate. Precedenza per i nuovi upload:
  1. policy del file esistente;
  2. mount della directory più vicina;
  3. preferenza utente, se valida nel pool del gruppo;
  4. default del gruppo;
  5. default di sistema.
- **Load balancing policy**: policy virtuale con più policy membro e **pesi**; gli upload vengono distribuiti sui membri in proporzione al peso. I file e le entità puntano sempre a policy concrete, mai alla virtuale. La load balancing non è selezionabile come target di relocation.
- **Migrazione tra storage policy (relocation)**:
  - dry-run con stima di file, entità, versioni, thumbnail e spazio;
  - sposta entità primaria, versioni e thumbnail;
  - supporta sorgenti e destinazioni cifrate;
  - clona le entità condivise senza toccare gli altri file;
  - scelta tra mantenere o eliminare la copia di origine;
  - audit dettagliato per entità nel task e nella UI;
  - **cleanup automatico** degli oggetti di origine trattenuti (`delete_source=false`) con retention configurabile (default 7 giorni), safety check sui riferimenti vivi e API admin per stato e pulizia forzata.

### Sistema

- **Log attività**: audit persistente di accessi (successi/fallimenti) e operazioni su file e share, con IP e user agent. L'utente vede la propria cronologia file e gli accessi recenti; l'admin ha una pagina dedicata con filtri e dettaglio. Logging best-effort: un errore di audit non blocca l'operazione originale.
- **Pipeline i18n** con cache delle traduzioni versionata per build e controllo delle chiavi richieste in fase di build.

### Correzioni

- velocità di upload corretta al resume;
- SQLite `busy_timeout` per ridurre i lock;
- gestione sicura delle vecchie entità di relocation (hard delete, nessun record orfano).

## Cosa manca

Le seguenti funzioni della Pro Edition **non sono implementate** e sono considerate fuori scope per questo fork:

- **Segnalazioni abuso**: nessun canale di report per utenti/visitatori e nessun pannello admin di revisione.
- **Condivisione avanzata**: modifica/eliminazione via share link, permessi granulari per file/cartella, upload anonimo, default shares, condivisione a pagamento.
- **Monetizzazione**: piani storage, membership, codici di riscatto, crediti, ordini e pagamenti, template email ricevuta/quota superata.
- **Autenticazione**: multi-account switching, SSO Logto, OIDC, QQ Connect, filtro email in registrazione.
- **Altre estensioni di sistema**: annunci sito, node selection, custom property di tipo utente, iOS VOL.

Per le funzioni implementate non ci sono popup "passa a Pro" né upsell: i controlli Pro relativi sono stati rimossi o sbloccati. Le voci delle funzioni non implementate non sono esposte come funzionanti.

## Build

Requisiti: Go 1.26+, Node.js 22+, Python 3, `zip`.

```bash
./scripts/build-release.sh 4.19.1
```

Output: `dist/cloudreve-4.19.1-linux-amd64`, con il frontend incorporato nel binario.

## Licenza

GPL-3.0, vedi `cloudreve/LICENSE`. Fork indipendente, non affiliato a Cloudreve.
