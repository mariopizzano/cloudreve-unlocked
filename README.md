# cloudreve-unlocked

Fork non ufficiale di [Cloudreve](https://github.com/cloudreve/Cloudreve) con funzionalità aggiuntive di gestione delle storage policy.

Questo repository **non contiene Cloudreve Pro** e non include codice proprietario. È basato sul codice sorgente pubblico GPL-3.0.

## Stato attuale

Base:

- Cloudreve backend: `v4.19.1` (commit `6193d04`, upstream `cloudreve/Cloudreve`)
- Cloudreve frontend: `v4.19.1` (upstream `cloudreve/frontend`)

Funzionalità implementate:

- mount di una storage policy su una cartella;
- la policy montata vale per i nuovi upload nella cartella e nelle sottocartelle;
- la cartella padre più vicina con un mount vince;
- copia/spostamento non applicano la policy della destinazione;
- i file esistenti non vengono spostati;
- visualizzazione della policy effettiva e della policy montata nel pannello dettagli;
- endpoint `GET/PATCH /api/v4/file/policy`;
- guardia sull'eliminazione di policy montate su cartelle.

Non ancora implementate:

- relocation fisica dei file tra policy;
- load balancer policy;
- policy multiple per gruppo;
- policy per directory con regole avanzate.

## Organizzazione per versione Cloudreve

Ogni versione reale di Cloudreve ha il proprio branch:

- `cloudreve-4.19.1` → questa versione
- future versioni: `cloudreve-4.x.y`

I tag releases seguono lo schema:

```text
v4.19.1-policy-mount.1
```

I diff rispetto all'upstream sono in `patches/`:

- `patches/backend-4.19.1.patch`
- `patches/frontend-4.19.1.patch`

Per una nuova versione Cloudreve si crea un nuovo branch dal tag upstream e si riapplicano/adattano le patch.

## Struttura

```text
cloudreve/   backend Go (module github.com/cloudreve/Cloudreve/v4)
frontend/    frontend React/TypeScript
patches/     diff rispetto all'upstream
scripts/     build release
```

## Build

Requisiti: Go 1.26+, Node.js 22+, Python 3, `zip`.

```bash
./scripts/build-release.sh 4.19.1
```

Output:

```text
dist/cloudreve-4.19.1-linux-amd64
```

Il frontend viene incorporato nel binario come `application/statics/assets.zip`.

## Deploy

Il binario è self-contained. In produzione:

1. fermare il servizio;
2. fare backup consistente del database SQLite;
3. sostituire il binario;
4. avviare il servizio;
5. verificare `/api/v4/site/ping` e i log.

Nessuna migrazione database è richiesta per la funzione di mount policy: viene riutilizzata la colonna `files.storage_policy_files`.

## Licenza

GPL-3.0. Vedi `cloudreve/LICENSE`.

Questo è un fork indipendente, non affiliato a Cloudreve.
