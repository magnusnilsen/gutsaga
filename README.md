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

Oppdrag har mål i rekkefølge: sted, snakk med noen, samle en gjenstand, eller (eksperimentelt) et drap. Når alle målene er grønne, tar tillegget skjermbilde og skriver en linje i chatten. Programmet på maskinen leser chatloggen og skjermbildet og sender det til API-et. Det trykker aldri på noe i spillet og leser aldri skjermen.

Oppdragene ligger i `GutSagaData.lua`, som programmet skriver. Spillet leser filen ved innlogging og `/reload`. Det du fullfører, lagres per karakter i SavedVariables og skrives til disk når du logger ut eller laster inn på nytt. Chatlinjen er det som gjør at fullføringen kommer frem selv om du avslutter spillet brått.

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

## For den som driver serveren

API-et ligger i `api/` og kjøres på Railway-prosjektet `gutsaga`.

1. Koble GitHub-repoet til Railway. **Rotmappen til tjenesten skal være repo-roten**, ikke `api/`. `railway.json` peker på `api/Dockerfile`.
2. Legg på Postgres-pluginen. Den setter `DATABASE_URL`.
3. Sett `DISCORD_WEBHOOK_URL` (innkommende webhook) og `ADMIN_TOKEN` (`openssl rand -hex 32`). For forsidebilde i chat, sett også `PUBLIC_BASE_URL` (`https://gutsaga-production.up.railway.app`) og `IMAGE_URL_SECRET` (`openssl rand -hex 32`). `IMAGE_URL_TTL` er valgfri og er `1h` hvis den mangler.
4. Push til `main` bygger og ruller ut. Migreringene kjører ved oppstart. `/healthz` skal svare `{"ok":true}`.
5. Spillere logger inn med Smia. API-et sjekker medlemskap i hubben wow og gir en egen økt. `mint` finnes fortsatt for en manuell nøkkel.

Kontrakten, inkludert admin-endepunktene som Smia kan peke på senere, står i [docs/API.md](docs/API.md) og [docs/openapi.yaml](docs/openapi.yaml).

Fullføring er idempotent per karakter og oppdrag. Versjonen spilleren hadde lastet, blir lagret. Å redigere et oppdrag øker versjonsnummeret og tar et øyeblikksbilde. API-et poster til Discord som **GutSaga**: `**Magni** fullførte *Oppdragsnavn*!`, med skjermbildet vedlagt når vi har ett.

Lokal testing med Postgres, API og seed står i avsnittet [Lokal testing](#lokal-testing). `make dev` er den korte varianten.

Tester (de tømmer en database som har `test` i navnet):

```bash
export DATABASE_URL=postgres://gutsaga:gutsaga@127.0.0.1:5432/gutsaga_test?sslmode=disable
go test ./...
luacheck addon
```
