# GutSaga

GutSaga er oppdragsloggen til gutane i World of Warcraft: Forever. Spillet har ingen plass til våre egne oppdrag, så vi har et tillegg i klienten og et lite program på maskinen som snakker med et eget API.

Forever-klienten er 1.60.1 og bruker det moderne tilleggs-API-et (Midnight), ikke 1.12. Grensesnittnummeret står i `addon/INTERFACE` og i `.toc`-filene. Forever bruker `16001` til det blir endret.

## Lokal testing

Dette er veien før Railway finnes. Du trenger Docker, Go, og en WoW-klient på macOS: Classic Era, Classic (Mists) eller retail. API-et kjører på maskinen. Innloggingen går til Smia i nettleseren og tilbake til `127.0.0.1`.

1. Klon repoet og stå i rotmappen.
2. Portene `5432` og `8080` må være ledige.
3. Start Postgres, API og testoppdragene:

```bash
make dev
```

`make dev` bygger API-et, venter på `http://localhost:8080/healthz`, og seeder tre partioppdrag i Elwynn Forest (kart 37). Å kjøre den på nytt oppdaterer de samme titlene. `make down` stopper containerne. `docker compose down -v` sletter databasen.

4. Start følgeprogrammet mot retail og det lokale API-et:

```bash
go run ./cmd/gutsaga --api-url http://localhost:8080 --client retail
```

Samme valg som miljøvariabler: `GUTSAGA_API_URL=http://localhost:8080` og `GUTSAGA_CLIENT=retail`. `--client retail` velger mappen `_retail_` når både Forever og retail er installert. `--wow-path` (eller `GUTSAGA_WOW_PATH`) peker rett på flavor-mappen om letingen bommer.

5. Programmet installerer tillegget i `_retail_/Interface/AddOns/GutSaga` og åpner nettleseren med «Logg inn med Smia». Logg inn som et medlem av hubben **wow**, server **mcp**. Smia sender nettleseren tilbake til `http://127.0.0.1:<port>/callback`. Om Smia avviser den adressen, prøver programmet `localhost`, og til slutt kan du lime inn koden i terminalen.
6. Start retail, huk av GutSaga, logg inn i Elwynn, og skriv `/reload`.

**Forventet resultat**

- Terminalen sier at du er logget inn, og at oppdragene er skrevet. I spillet viser `/gutsaga` tre oppdrag: «En kveld i Goldshire», «Veien fra Northshire» og «Melk til vaktlaget». Det er ingen Fullfør-knapp.
- Chatloggen får en linje `[GutSaga] CHARACTER navn|realm|nivå|rase|klasse|BattleTag#tall|tid`. Karakteren dukker opp i API-et uten at du skriver den inn.
- «En kveld i Goldshire»: stå i eller rett utenfor Lion's Pride Inn (omtrent 43.8, 65.8 på Elwynn-kartet). Første mål blir grønt. Snakk med **Innkeeper Farley** (engelsk klientnavn). Kjøp 1 Refreshing Spring Water (gjenstand 159). Når alle tre er grønne, tar spillet skjermbilde selv og skriver `[GutSaga] COMPLETE …`.
- «Veien fra Northshire»: stå på veien der Falkhaan Isenstrider står (omtrent 45.6, 47.7).
- «Melk til vaktlaget»: stå i Goldshire igjen og ha 1 Ice Cold Milk (gjenstand 1179, selges av Farley).
- Følgeprogrammet plukker opp linjen og skjermbildet og sender dem til `http://localhost:8080`. Oppdraget står som fullført etter `/reload`.

### Classic

Retail trengs ikke. Seed-oppdragene er de samme i Classic Elwynn: kart 37, Lion's Pride Inn, Innkeeper Farley, Refreshing Spring Water (159) og Ice Cold Milk (1179).

Grensesnittnumrene er hentet 22. oktober 2026 (`/dump select(4, GetBuildInfo())` i klienten om du vil sjekke):

| Klient | Mappe | TOC | Interface |
|---|---|---|---|
| Classic Era 1.15.9 | `_classic_era_` | `GutSaga_Vanilla.toc` | 11509 |
| Classic (Mists) 5.5.4 | `_classic_` | `GutSaga_Mists.toc` | 50504 |
| Anniversary (TBC) 2.5.6 | `_anniversary_` | `GutSaga_TBC.toc` | 20506 |

Anniversary er Burning Crusade nå, så den laster ikke Vanilla-filen. Era gjør det.

```bash
go run ./cmd/gutsaga --api-url http://localhost:8080 --client classic_era
```

Mists-progresjon:

```bash
go run ./cmd/gutsaga --api-url http://localhost:8080 --client classic
```

`classic_era` peker på `_classic_era_`. `classic` peker på `_classic_`. Samme valg finnes som `GUTSAGA_CLIENT`. Uten flagg leter programmet selv, og en maskin som bare har Classic blir funnet. Har du flere klienter, sett flagget så riktig mappe vinner.

**Forventet resultat:** samme tre oppdrag og samme chatlinjer som på retail. Stå på torget i Goldshire, ikke nede i en gruve, så kart-id er 37 og ikke et underkart. Tillegget bruker `C_Map`, `C_Item` og `C_Container` når klienten har dem, og de eldre kallene (`GetPlayerMapPosition`, `GetItemCount`, veske-API) når de mangler. `BNGetInfo`, `Screenshot` og `LoggingChat` kalles bare hvis de finnes. BattleTag kan være tom på en klient uten Battle.net-navn. Da fullfører oppdraget likevel.

NPC-navnene i seed er engelske, fordi `UnitName` følger klientspråket. Kartkoordinatene er de samme på norsk og engelsk. Kjør klienten på engelsk for denne runden, eller endre `npc` i API-et til navnet klienten viser.

Du må være medlem av Smia-hubben wow. Ellers svarer API-et 403. API-containeren må nå `https://smia.tools` for å sjekke tokenet. Den lagrer ikke Smia-tokenet, bare en egen økt (`gss_…`) i 30 dager.

Om Smia er nede kan du midlertidig sette `SMIA_AUTH_MODE=dev` og `SMIA_DEV_TOKEN=dev-token` på API-et, og `GUTSAGA_DEV_TOKEN=dev-token` på følgeprogrammet. Da hoppes nettleseren over. Vanlig `make dev` bruker ekte Smia-innlogging.

### Tilbakestille oppdrag lokalt

`/gutsaga wipe` (eller `clear` / `reset`) tømmer lokale fullføringer og køer en API-sletting. `/gutsaga reset selected` (eller de navngitte aliasene) gjør det for én saga. Etterpå: `/reload` så SavedVariables skrives til disk — følgeprogrammet kaller `DELETE /v1/progress` og synker på nytt. Uten den runden kommer «fullført» tilbake fra API-et.

Mens du utvikler tillegget, sett `GUTSAGA_ADDON_SRC` til `addon/GutSaga` (eller `addon`) før `go run`, så Install kopierer lua fra disk i stedet for den innebygde kopien:

```bash
GUTSAGA_ADDON_SRC=addon/GutSaga go run ./cmd/gutsaga --api-url http://localhost:8080 --client classic_era
```

## Slik får du det (venner)

Du trenger programmet GutSaga og World of Warcraft. Du trenger ikke kildekoden og ikke en API-nøkkel. Du logger inn med Smia.

1. Last ned filen for maskinen din fra utgivelsene (se under om hvor de ligger):
   - Mac med Apple-chip: `gutsaga-darwin-arm64`
   - Mac med Intel: `gutsaga-darwin-amd64`
   - Windows: `gutsaga-windows-amd64.exe`
2. Første gang stopper operativsystemet filen fordi den ikke er signert. Se avsnittene under.
3. Start programmet. Det legger tillegget i `Interface/AddOns/GutSaga` og åpner «Logg inn med Smia».
4. Start spillet, huk av GutSaga på karaktervalg, og skriv `/reload` eller logg inn på nytt.
5. `/gutsaga` eller `/gs` åpner oppdragsloggen. Minikartet har en G-knapp.

Oppdrag har steg: gå et sted (`visit` fester avhukingen, `hold` er bare mens du står der), snakk med noen, ha en gjenstand, et emote (`/salute`), et drap, eller en sekvens med ett tidsvindu («klapp, så /rude innen 30 sekunder»). Sagaen er ferdig første gang alle stegene stemmer samtidig. Et drap kan kreve at hele gruppa er på ett eksakt nivå og inne i instansen. Når alle stegene er grønne, tar tillegget skjermbilde. Programmet på maskinen leser SavedVariables og chatloggen og sender det til API-et. Det trykker aldri på noe i spillet og leser aldri skjermen.

JSON-formen Smia sender til API-et står i [docs/API.md](docs/API.md) og [docs/sagas-api.md](docs/sagas-api.md). Kort:

- Steg: `go`, `talk`, `item`, `kill`, `emote`, `sequence`. `memory` er `visit` eller `hold`. Kontrakten står i [docs/API.md](docs/API.md).
- `emote`: `"emote": "SALUTE"` (eller `"/salute"`). Valgfritt `npc` (UnitName) og samme stedsfelt som `go`.
- `when`: `"clock": "local"` (standard, `time()` / `date("*t")` på PC-en), `"game"` (`GetGameTime`, ingen dato) eller `"realm"` (`GetServerTime` som UTC). `"start": "22:00", "end": "02:00"` og/eller `starts_at` / `ends_at`.
- Gruppedrap: `"group": { "level": 17, "instance": "The Deadmines" }` på `kill`-steget. Eldre saga-gruppe er `"size"` og `"min_level"` på sagaen.

Classic Era, Mists og TBC har `DoEmote`, `GetGameTime`, `time`/`date` og gruppekallene. Forever kan skjule navn, nivå og `GetGameTime` i kamp (hemmelige verdier); tillegget bruker siste lesbare roster og siste lesbare spillklokke. Skapninger matches på navn, ikke GUID. Inne i en instans sendes gruppekreditten på `PARTY`/`RAID` likevel. Om klienten stopper den, krediterer API-et likevel de karakterene følgeprogrammet kjenner, når den som fullfører har programmet kjørende og vennene har logget inn minst én gang.

Oppdragene ligger i `GutSagaData.lua`, som programmet skriver. Spillet leser filen ved innlogging og `/reload`. Det du fullfører, lagres per karakter i SavedVariables og skrives til disk når du logger ut eller laster inn på nytt. Chatlinjen er det som gjør at fullføringen kommer frem selv om du avslutter spillet brått.

Programmet holder en forbindelse åpen mot API-et. Når en saga blir laget, endret, avsluttet eller fullført, skriver det fila med en gang og viser et varsel på maskinen: «Ny saga: … — skriv /reload i WoW». Spillet ser endringen etter `/reload`. Etter innlasting skriver chatten én linje, for eksempel «Ny saga: … er lastet inn.» Om forbindelsen ryker, prøver programmet igjen selv, og det spør API-et uansett omtrent hvert tiende minutt.

Skriv `/reload` etter at programmet sier at oppdragene er oppdatert.

### macOS og Gatekeeper

Filen er usignert. Første gang sier macOS at den ikke kan åpnes.

1. Legg filen et sted du finner igjen, for eksempel Hjem-mappen.
2. Gi den kjøretillatelse: `chmod +x gutsaga-darwin-arm64`
3. Åpne **Systeminnstillinger → Personvern og sikkerhet**.
4. Under Sikkerhet: «GutSaga ble blokkert» → **Åpne likevel**.
5. Alternativt: høyreklikk filen → **Åpne** → **Åpne**.
6. Hvis den fortsatt er sperret:

```bash
xattr -cr /sti/til/gutsaga-darwin-arm64
```

Kjør den fra Terminal, ikke ved å dobbeltklikke, første gangen. Da kan du svare på oppsettet.

### Windows og SmartScreen

1. Last ned `gutsaga-windows-amd64.exe`.
2. SmartScreen viser «Windows beskyttet PC-en».
3. Velg **Mer informasjon**, deretter **Kjør likevel**.

Kjør `setup` fra PowerShell slik at du får spørsmålene i vinduet.

## Oppdateringer

Programmet sjekker GitHub ved oppstart og omtrent hver fjerde time. Det bytter ut seg selv og tillegget når det finnes en nyere versjon. Versjonsnummeret er `0.1.<byggnummer>`.

Repoet med kildekoden er privat. Ikke bygg en GitHub-token inn i programmet.

**Anbefalt:** et eget offentlig repo som bare inneholder utgivelsene, for eksempel `magnusnilsen/gutsaga-releases`. Venner laster ned uten token, og tokenen deres kan ikke lese kildekoden. Når hemmelighetene `PUBLIC_RELEASE_REPO` og `PUBLIC_RELEASE_TOKEN` er satt på det private repoet, speiler CI utgivelsen dit. I `config.json`:

```json
"update": { "repo": "magnusnilsen/gutsaga-releases", "token": "" }
```

Alternativet er en finmasket token med lesetilgang i hver venns `config.json` (`update.token`) mot det private repoet. Den tokenen kan også lese kildekoden, så det offentlige utgivelsesrepoet er enklere og tryggere.

Innstillingene ligger i:

- Mac: `~/Library/Application Support/GutSaga/config.json`
- Windows: `%APPDATA%\GutSaga\config.json`

Se `config.example.json`.

## I spillet

Chatlinjene som programmet leser, er:

```
[GutSaga] CHARACTER navn|realm|nivå|rase|klasse|BattleTag|unix-tid
[GutSaga] COMPLETE <oppdragId> <versjon> <karakter> <realm> <nivå> <unix-tid>
[GutSaga] LEVELUP <karakter> <realm> <nivå> <unix-tid>
```

Realm kan ha mellomrom. Tillegget skrur på chatlogging (`LoggingChat`). Loggen er `Logs/WoWChatLog.txt`. Skjermbilder ligger i `Screenshots/`.

Andre som har GutSaga og står i gruppe eller guild, kan få et kort varsel. Inne i instanser sender vi ikke addon-meldinger, fordi klienten begrenser dem der. Vi leser heller ikke hemmelige enhetsverdier i kamp. Navn og nivå huskes fra innlogging.

`## Interface: 16001` i `GutSaga_Forever.toc` er beta-verdien. Endre tallet i `addon/INTERFACE` og i toc-filene samtidig. `GutSaga_Mainline.toc` er retail (`120000`). `GutSaga_Vanilla.toc` er Classic Era (`11509`). `GutSaga_Mists.toc` er Classic-progresjon (`50504`). `GutSaga_TBC.toc` er Anniversary (`20506`). Når programmet installerer for Forever, skriver det Forever-nummeret inn i alle toc-filene, fordi den klienten kan laste Mainline-filen. De andre klientene beholder sin egen toc.

## Saga-kjeder

> **Idé / senere.** Kjeder (`requires` + `chain`) er implementert i API og addon, men ikke en del av det vi robustgjør nå. La dem ligge til grunnleggende flyt (emotes, sync, Discord, karakter-opt-in) er stabil. Ikke bygg nye produktflater oppå dette ennå.

En saga kan kreve at karakteren har fullført andre sagaer først, og kan ligge i en kjede som loggen viser som «Kapittel 2 av 4». Begge feltene er valgfrie. Uten `requires` er sagaen åpen. Uten `chain` står den for seg selv.

`requires` er en liste med saga-id-er. **Alle** må være fullført av den samme karakteren. `chain` er bare visning: `id` er en stabil nøkkel (`[A-Za-z0-9][A-Za-z0-9_-]{0,63}`), `name` er teksten i loggen, `order` er rekkefølgen (unikt innenfor samme `id`, fra 1). Samme `id` deler navn. Setter du et nytt navn, oppdateres navnet på de andre sagaene i kjeden. Versjonen deres økes ikke.

Admin (`POST`/`PATCH /v1/admin/quests`) og spiller-API-et (`/v1/sagas`) tar samme JSON. Smia-verktøyene `create_saga` og `update_saga` sender feltene videre som de er.

```json
{
  "title": "Kapittel 2",
  "description": "Videre fra vertshuset.",
  "objectives": [
    {"id": "road", "type": "location", "text": "Stå på veien", "map_id": 37, "x": 0.456, "y": 0.477}
  ],
  "reward": "",
  "min_level": 1,
  "party_quest": true,
  "active": true,
  "created_by": "Magnus#2231",
  "assigned_character_ids": [],
  "requires": ["11111111-1111-1111-1111-111111111111"],
  "chain": {"id": "goldshire", "name": "Goldshire", "order": 2}
}
```

Svaret legger på plass i kjeden. `index` og `total` regnes ut og skal ikke sendes inn:

```json
"requires": ["11111111-1111-1111-1111-111111111111"],
"chain": {"id": "goldshire", "name": "Goldshire", "order": 2, "index": 2, "total": 4}
```

`requires` kan også sendes som `[{"id": "<uuid>"}]`. Tom liste eller `null` fjerner forutsetninger. `"chain": null` tar sagaen ut av kjeden. Utelatte felt på `PATCH` blir stående.

API-et avviser ukjent saga-id, selvkrav, sykluser og to sagaer med samme `chain.order`. En ny fullføring av en låst saga får `409`:

```json
{"error": "locked", "message": "Låst: fullfør Kapittel 1 først"}
```

`GET /v1/sync` har `locks` for hver av spillerens karakterer. `GET /v1/admin/quests?character_id=` setter `locked`, `unlocked` og `missing` i tillegg til `eligible`. Låsen er uavhengig av nivå og tildeling: en saga kan være synlig og likevel låst. Loggen viser den grå, med «Kapittel 2 av 4 — Låst: fullfør Kapittel 1 først». Når forrige saga fullføres, låses neste opp i klienten med en gang (kjeden ligger i `GutSagaData.lua`). Neste synk bekrefter det samme.

### Versjon

Fullføring lagres én gang per karakter og saga, med `quest_version` de hadde lastet. En forutsetning teller så snart den raden har `status: completed`, uansett versjon. Har du fullført kapittel 1 på versjon 1, er kapittel 2 åpent selv om kapittel 1 siden er redigert til versjon 4.

Låsen leser `requires` på sagaen slik den er nå, ikke på øyeblikksbildet i versjonen spilleren har lastet. Å endre `requires` eller `chain` øker versjonen og lagrer et nytt øyeblikksbilde, men sletter ingen fullføringer og krever ikke at forutsetningen tas om igjen. En saga som allerede er fullført, blir ikke låst igjen om noen senere legger på en forutsetning. Et nytt forsøk på en saga som alt er fullført, svarer som før (`already_completed`), også om den ville vært låst for en som ikke har gjort den.

## For den som driver serveren

API-et ligger i `api/` og kjøres på Railway-prosjektet `gutsaga`.

1. Koble GitHub-repoet til Railway. **Rotmappen til tjenesten skal være repo-roten**, ikke `api/`. `railway.json` peker på `api/Dockerfile`.
2. Legg på Postgres-pluginen. Den setter `DATABASE_URL`.
3. Sett `DISCORD_WEBHOOK_URL` (innkommende webhook) og `ADMIN_TOKEN` (`openssl rand -hex 32`). Det er ingen flere Discord-variabler. Samme webhook brukes til nye sagaer, versjoner, avslutning, milestolpe-nivå og fullføring. For forsidebilde i chat, sett også `PUBLIC_BASE_URL` (`https://gutsaga-production.up.railway.app`) og `IMAGE_URL_SECRET` (`openssl rand -hex 32`). `IMAGE_URL_TTL` er valgfri og er `1h` hvis den mangler.
4. Push til `main` bygger og ruller ut. Migreringene kjører ved oppstart. `/healthz` skal svare `{"ok":true}`.
5. Spillere logger inn med Smia. API-et sjekker medlemskap i hubben wow og gir en egen økt. `mint` finnes fortsatt for en manuell nøkkel.

Kontrakten, inkludert admin-endepunktene som Smia kan peke på senere, står i [docs/API.md](docs/API.md) og [docs/openapi.yaml](docs/openapi.yaml).

Fullføring er idempotent per karakter og oppdrag. Versjonen spilleren hadde lastet, blir lagret. Å redigere et oppdrag øker versjonsnummeret og tar et øyeblikksbilde.

### Discord

Brukernavnet er alltid **GutSaga**. Teksten er norsk. Meldingene er embeds med samme fargevalg: gull for ny saga (`13017435`), grønn for fullføring (`3908957`), blå for ny versjon (`4091802`), grå for avslutning (`7037528`), gul for milestolpe-nivå (`13666602`). Tidsstempel og bunntekst `GutSaga` følger med.

Innlegget skjer i bakgrunnen. API-kallet venter ikke, og en feil fra Discord (også HTTP 429) ruller ikke tilbake sagaen eller fullføringen. 429 venter `retry_after` fra JSON-kroppen, ellers `Retry-After`. Nettverksfeil og 5xx prøves på nytt med økende pause. Etter noen forsøk logges feilen.

`POST /v1/admin/quests` (det Smia-verktøyet `create_saga` kaller) kunngjør en aktiv saga. `announce: false` hopper over. Et inaktivt utkast kunngjøres ikke med mindre `announce` er `true`. Valgfri `image_base64` lagres som cover og legges ved Discord-meldingen som fil (`attachment://cover.png`). Fullføring gjør det samme med skjermbildet (`attachment://screenshot.png`). Chatklienter som trenger en adresse bruker i stedet `image_signed_url` fra `PUBLIC_BASE_URL` og `IMAGE_URL_SECRET`.

En endring av tekst, mål, belønning, minstenivå eller parti legger ut en kort versjonsmelding. `active: false` legger sagaen bort. Bare å bytte hvem den er tildelt, er stille. `announce: false` på `PATCH` demper begge. Samme regler gjelder `PATCH /v1/sagas/{id}`. Nivå 10, 20, 30, … får en kort melding. Andre nivåer gjør det ikke.

Eksempler på `payload_json`. Filen, når den finnes, er multipart-feltet `files[0]`.

Ny saga:

```json
{
  "username": "GutSaga",
  "content": "Magnus opprettet en ny saga",
  "embeds": [{
    "title": "Første vakt",
    "description": "Stå vakt ved porten.",
    "color": 13017435,
    "fields": [
      {"name": "Mål", "value": "1. Møt opp\n2. Hold ut"},
      {"name": "Minste nivå", "value": "10", "inline": true},
      {"name": "Belønning", "value": "Ære", "inline": true},
      {"name": "Laget av", "value": "Magnus#2231", "inline": true},
      {"name": "Hvem", "value": "Parti — Magni (Paladin, nivå 12)"}
    ],
    "timestamp": "2026-10-24T12:00:00Z",
    "image": {"url": "attachment://cover.png"},
    "footer": {"text": "GutSaga"}
  }],
  "allowed_mentions": {"parse": []}
}
```

Fullføring:

```json
{
  "username": "GutSaga",
  "content": "**Magni** fullførte en saga!",
  "embeds": [{
    "title": "Første vakt",
    "description": "Magni, Paladin på nivå 12, fullførte sagaen.",
    "color": 3908957,
    "fields": [
      {"name": "Karakter", "value": "Magni", "inline": true},
      {"name": "Klasse", "value": "Paladin", "inline": true},
      {"name": "Nivå", "value": "12", "inline": true},
      {"name": "Versjon", "value": "3", "inline": true},
      {"name": "Belønning", "value": "Ære", "inline": true}
    ],
    "timestamp": "2026-10-24T18:30:00Z",
    "image": {"url": "attachment://screenshot.png"},
    "footer": {"text": "GutSaga"}
  }],
  "allowed_mentions": {"parse": []}
}
```

Ny versjon:

```json
{
  "username": "GutSaga",
  "content": "Magnus oppdaterte en saga",
  "embeds": [{
    "title": "Første vakt",
    "description": "Ny versjon 2.",
    "color": 4091802,
    "timestamp": "2026-10-24T12:00:00Z",
    "footer": {"text": "GutSaga"}
  }],
  "allowed_mentions": {"parse": []}
}
```

Lagt bort:

```json
{
  "username": "GutSaga",
  "content": "Magnus la bort en saga",
  "embeds": [{
    "title": "Første vakt",
    "description": "Sagaen er ikke lenger aktiv.",
    "color": 7037528,
    "timestamp": "2026-10-24T12:00:00Z",
    "footer": {"text": "GutSaga"}
  }],
  "allowed_mentions": {"parse": []}
}
```

Milestolpe:

```json
{
  "username": "GutSaga",
  "content": "**Magni** nådde et nytt nivå.",
  "embeds": [{
    "title": "Nivå 20",
    "description": "Magni · Paladin",
    "color": 13666602,
    "timestamp": "2026-10-24T08:00:00Z",
    "footer": {"text": "GutSaga"}
  }],
  "allowed_mentions": {"parse": []}
}
```

Følgeprogrammet lytter på `GET /v1/ws` (samme økt som `/v1/sync`) i stedet for å spørre hele tiden. API-et bruker Postgres `LISTEN`/`NOTIFY`, så flere instanser på Railway deler hendelsene. Varsler, ping og omstart står i [docs/realtime.md](docs/realtime.md).

Lokal testing med Postgres, API og seed står i avsnittet [Lokal testing](#lokal-testing). `make dev` er den korte varianten.

Tester (de tømmer en database som har `test` i navnet):

```bash
export DATABASE_URL=postgres://gutsaga:gutsaga@127.0.0.1:5432/gutsaga_test?sslmode=disable
go test ./...
luacheck addon
```
