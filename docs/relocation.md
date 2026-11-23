# Relocation tra storage policy

Stato: **design, non implementato**

## Obiettivo

Spostare fisicamente file e blob da una storage policy a un'altra e aggiornare il database in modo coerente:

- B2/S3 → locale
- locale → B2/S3
- in futuro tra due policy remote

Non va confuso con il mount per cartella: il mount cambia solo la policy dei **nuovi** upload.

## Vincoli

- Il database SQLite è l'unica fonte di verità.
- Un'entità può essere condivisa da più file (`reference_count`).
- Un file può avere più entità: originale, thumbnail, versioni, live photo.
- Le entità possono essere cifrate con `EncryptMetadata`.
- Il task deve essere interrompibile e riprendibile.
- Non si deve mai aggiornare il DB prima che la copia fisica sia verificata.
- Su B2 ogni lettura genera egress: vanno evitati duplicati e retry inutili.

## Strategia consigliata

**Copia a livello di entità, poi swap.**

Per ogni entità:

1. leggere il blob dalla policy sorgente;
2. creare un nuovo oggetto sulla policy destinazione;
3. verificare dimensione (e hash se disponibile);
4. creare una nuova riga `entities` con la nuova policy;
5. ricollegare i file alla nuova entità;
6. solo alla fine eliminare la vecchia entità/oggetto, se non più referenziata.

Vantaggi:

- le entità condivise non vengono modificate in place;
- rollback naturale: finché il DB non viene aggiornato, i vecchi oggetti restano validi;
- si può fare cleanup separato.

## Fasi

### Fase 0 — dry run

- Endpoint di pianificazione senza scritture.
- Calcolo di file, entità, blob, dimensione per policy.
- Avvisi su entità cifrate, condivise, versioni, thumbnail.
- UI con riepilogo e conferma.

### Fase 1 — relocation file singolo

- Solo entità non cifrate.
- Copia + verifica + swap.
- Nessuna cancellazione automatica: cleanup manuale o task separato.
- Test su policy locale temporanea.

### Fase 2 — cifratura

- Gestione `EncryptMetadata` e master key.
- Stream decrypt/re-encrypt quando la policy di destinazione ha impostazioni di cifratura diverse.
- Test con file piccoli prima di procedere su B2.

### Fase 3 — cartelle e versioni

- Ricorsione su cartelle.
- Tutte le entità: originali, thumbnail, versioni, live photo.
- Ripresa dopo interruzione, retry con backoff.
- Limite di concorrenza basso (1–2 worker).

### Fase 4 — UI e sicurezza

- Selezione multipla, policy di destinazione, progresso, errori per file.
- Permessi dedicati.
- Audit log.

## Punti ancora da verificare nel codice

- `fs.PrepareRelocateRes` e `inventory.RelocateEntityParameter` esistono ma non hanno implementazione.
- Manca un metodo inventory per lo swap atomico delle entità.
- Manca il workflow `relocate` in `pkg/filemanager/workflows`.
- Manca la route `POST /workflow/relocate`.
- Va verificata la compatibilità con policy remote/slave.
- Va verificato il comportamento di thumbnail e versioni durante lo swap.

## Stima

- Fase 0: 1–2 giorni
- Fase 1: 3–5 giorni
- Fase 2: 4–7 giorni
- Fase 3: 1–2 settimane
- Fase 4: 3–5 giorni

La stima non include il tempo di esecuzione sui 22,8 GB attualmente su B2.
