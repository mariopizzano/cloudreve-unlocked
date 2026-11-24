# Relocation tra storage policy

Stato: **Fase 2 implementata** (entità primarie, versioni, thumbnail e blob condivisi), con dry-run.

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
- il plan espone: file, cartelle, entità totali, versioni, thumbnail, condivise, cifrate, dimensione

## Limiti attuali

- nessun cleanup automatico di oggetti orfani dopo un errore (solo best-effort della copia appena creata)
- nessun lock esplicito del file durante il task
- la policy montata sulla cartella non viene modificata: cambia solo la policy effettiva dei file

## Strategia

Copia a livello di entità + swap nel database:

1. lettura dell'entità sorgente (decrypt se cifrata);
2. scrittura sulla policy di destinazione (encrypt se richiesto);
3. creazione della nuova entità;
4. aggiornamento del file (`primary_entity`, `storage_policy_files`) solo per l'entità primaria;
5. rimozione della vecchia entità dal DB, oppure decremento del reference count se condivisa;
6. cancellazione fisica della sorgente solo se `delete_source=true` e non condivisa.

## Test eseguiti

- locale → locale, con cancellazione sorgente: OK
- B2 cifrato → locale non cifrato, sorgente conservata: OK, hash identico al contenuto decifrato originale
- locale → locale cifrato: OK, download decifrato con hash identico all'originale

## Prossimi passi

- Fase 3: cleanup sicuro degli oggetti sorgente e degli orfani
- Fase 4: UI di conferma con opzione `delete_source`, progresso per entità e audit
