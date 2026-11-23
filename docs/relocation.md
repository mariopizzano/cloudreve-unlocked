# Relocation tra storage policy

Stato: **Fase 1 implementata** (file singolo + ricorsione cartelle), con dry-run.

## Implementato

- `POST /api/v4/workflow/relocate/plan` — dry-run, nessuna scrittura
- `POST /api/v4/workflow/relocate` — crea il task
- task `relocate` con fasi `index`, `transfer`, `finish`
- copia dell'entità primaria sulla policy di destinazione
- supporto sorgenti cifrate (decrypt in streaming)
- supporto destinazioni cifrate (nuova chiave AES-256-CTR e metadati)
- aggiornamento atomico di file, primary entity e policy
- entità condivise (`reference_count > 1`): saltate e conteggiate come non supportate
- `delete_source` opzionale, **default false**: l'oggetto sorgente viene conservato

## Limiti attuali

- vengono spostate solo le entità **primarie**; versioni e thumbnail restano sulla policy originale
- entità condivise non spostate
- nessuna cancellazione automatica dell'oggetto sorgente salvo `delete_source=true`
- nessun cleanup automatico di oggetti orfani dopo un errore
- nessun lock esplicito del file durante il task

## Strategia

Copia a livello di entità + swap nel database:

1. lettura dell'entità sorgente (decrypt se cifrata);
2. scrittura sulla policy di destinazione (encrypt se richiesto);
3. creazione della nuova entità;
4. aggiornamento del file (`primary_entity`, `storage_policy_files`);
5. rimozione della vecchia entità dal DB;
6. cancellazione fisica della sorgente solo se `delete_source=true`.

## Test eseguiti

- locale → locale, con cancellazione sorgente: OK
- B2 cifrato → locale non cifrato, sorgente conservata: OK, hash identico al contenuto decifrato originale
- locale → locale cifrato: OK, download decifrato con hash identico all'originale

## Prossimi passi

- Fase 2: spostare anche versioni e thumbnail
- Fase 3: cleanup sicuro degli oggetti sorgente e degli orfani
- Fase 4: UI di conferma con opzione `delete_source`, progresso e audit
