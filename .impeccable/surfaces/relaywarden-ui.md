# RelayWarden management UI — surface brief

Mode: Operate. Target: `/v0/resource/plugins/relaywarden/ui`.

## Audience and job

A solo developer running CLIProxyAPI on Windows. They need to know whether a
request is being compressed, whether a relay is degrading the response, and which
auth is fastest, then flip the relevant feature without editing YAML.

## Direction contract

THESIS
A working gateway console read as a transit diagram: feature lines, auth
stations, and service states. It refuses the same-size card grid and the
hero-metric template.

OWN-WORLD
Midnight enamel ground, porcelain text, hairline rules, and four line inks:
green (compression), amber (guard), cobalt (routing), red (alerts). Segoe UI
Variable for interface; Cascadia Mono for data. Standard web controls; the world
enters through type, palette, density, and the route strip.

STORY
The operator sees which protection is active, which auth is delayed, and what
changed last, then flips the relevant line without leaving the page.

FIRST VIEWPORT
Top bar: brand, Client -> CPA -> Upstream route strip, live indicator, config
revision, language switch. Left: four-line rail with status and page links;
master switches live on their pages so every off action uses the same inline
confirmation. Main: tabs, readout row, auth table, recent signals. Bottom: event
tape.

FORM
Assigned direction 3; seed:a900d29114ca692c.

FINISH
Unreviewed and undocumented is unfinished; this build ends with the finish
review, the verdict, DESIGN.md, and every shipping raster carrying its
provenance.

## Constraints

- Every feature has a switch; dangerous off-switches confirm inline.
- Chinese and English are first-class; technical identifiers stay verbatim.
- Management key stays in `sessionStorage`; static page carries no data.
- No external fonts, analytics, or CDN; CSP is same-origin only.
- Responsive: 1440 desktop, 390 mobile, 200% zoom, keyboard operable.
