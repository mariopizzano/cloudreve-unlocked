# Relocation tra storage policy

Stato: **Fase 4 implementata** (entità primarie, versioni, thumbnail, blob condivisi, gestione sorgente, record orfani e cleanup automatico degli oggetti trattenuti), con dry-run.

## Implementato

- `POST /api/v4/workflow/relocate/plan` — dry-run, nessuna scrittura
- `POST /api/v4/workflow/relocate` — crea il task
- task `relocate` con fasi `index`, `transfer`, `finish`
- spostamento di **tutte le entità** del file: primaria, versioni e thumbnail
- supporto sorgenti cifrate (decrypt in streaming)
- supporto destinazioni cifrate (nuova chiave AES-256-CTR e metadati)
- aggiornamento atomico di file, entità e policy
- **entità condivise** (`reference_count > 1`): il file viene sganciato dal blob condiviso e
  ne riceve una copia dedicata sulla nuova policy; il blob originale resta intatto per gli
  altri file e il suo `reference_count` viene decrementato
- `delete_source` opzionale, **default false**: l'oggetto sorgente viene conservato; le
  entità condivise non vengono mai cancellate
- gli oggetti conservati con `delete_source=false` vengono tracciati nella tabella
  `retained_objects` (policy, chiave fisica, dimensione, file/entità di origine, timestamp,
  stato) e rimossi fisicamente da un cron giornaliero dopo la retention configurata
- il plan espone: file, cartelle, entità totali, versioni, thumbnail, condivise, cifrate, dimensione

## Cleanup degli oggetti trattenuti

- cron: `cron_retained_object_cleanup` (default `@daily`)
- abilitazione: `retained_object_cleanup_enabled` (default `1`)
- retention: `retained_object_retention_days` (default `7` giorni)
- dimensione batch DB: `retained_object_cleanup_batch` (default `100`)
- prima di cancellare, il cleanup verifica che nessuna entità viva (non soft-deleted) referenzi
  la stessa coppia policy+source; in tal caso il record passa allo stato `skipped` e l'oggetto
  non viene toccato
- endpoint admin:
  - `GET /api/v4/admin/retained-objects` — conteggi, dimensione, ultima esecuzione e lista
  - `DELETE /api/v4/admin/retained-objects` — cleanup forzato immediato (ignora la retention,
    ma mantiene il controllo delle reference vive)

## Limiti attuali

- nessun lock esplicito del file durante il task
- la policy montata sulla cartella non viene modificata: cambia solo la policy effettiva dei file
- se un oggetto risulta ancora referenziato da un'entità viva viene marcato `skipped` e non
  ritentato automaticamente: la sua eventuale rimozione resta in carico al collector delle entità

## Strategia

Copia a livello di entità + swap nel database:

1. lettura dell'entità sorgente (decrypt se cifrata);
2. scrittura sulla policy di destinazione (encrypt se richiesto);
3. creazione della nuova entità;
4. aggiornamento del file (`primary_entity`, `storage_policy_files`) solo per l'entità primaria;
5. rimozione della vecchia entità dal DB, oppure decremento del reference count se condivisa;
6. gestione della sorgente:
   - `delete_source=true`: l'oggetto fisico viene eliminato subito dopo il commit;
   - `delete_source=false`: l'oggetto fisico resta sulla policy di origine e viene registrato
     in `retained_objects` nella stessa transazione dell'hard delete dell'entità; il cleanup
     periodico lo eliminerà dopo la retention.
   In entrambi i casi la vecchia riga entità viene eliminata definitivamente (hard delete), così
   non restano record orfani che il collector non sarebbe in grado di raccogliere.

## Test eseguiti

- locale → locale, con cancellazione sorgente: OK
- B2 cifrato → locale non cifrato, sorgente conservata: OK, hash identico al contenuto decifrato originale
- locale → locale cifrato: OK, download decifrato con hash identico all'originale
- **B2 cifrato → locale → B2** (Fase 2): versione + thumbnail spostate entrambe, entità primaria aggiornata,
  download condiviso HTTP 200 (109.555 byte) in entrambe le direzioni
- **delete_source=true in entrambe le direzioni**: oggetti B2 e locali di origine rimossi, nessun orfano
  locale residuo, download sempre HTTP 200
- **Fase 3, delete_source=false**: la vecchia entità viene rimossa definitivamente dal DB (hard delete),
  l'oggetto locale resta su disco dopo il passaggio del collector, download HTTP 200
- **Fase 4, cleanup automatico**: unit test su scadenza/retention, safety check su entità vive e
  soft-deleted (SQLite in-memory), `go build ./...` completo
- cleanup manuale degli oggetti locali trattenuti e delle vecchie righe di test

## Prossimi passi

- audit dettagliato per entità nella UI; la UI chiede già se mantenere o eliminare la copia di origine
- pagina admin dedicata per il cleanup (oggi disponibile via API)
