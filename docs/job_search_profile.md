# Job-search profile — Matías Buttari

This is the source-of-truth for **who is looking for a job through Buscalaburos
3000** and **what shape of role to filter for**. It feeds two things:

- the AI persona that personalises outreach messages (`ai_personas`),
- the relevance ranker that scores job postings (front 2B, when it exists).

Edit this file when your situation changes (new skills, salary floor, shift
from junior to mid, etc.) and re-run `scripts/sync-profile.sh` (TODO) to push
it to the DB.

## Summary

Fullstack JS/TS junior-tirando-a-SSR (1 año pro en Powing + 3 años freelance
QA), con un diferencial fuerte y poco saturado: **AI + automation** (n8n,
Make, Unipile SDK, integraciones con LLMs). Remoto demostrado. Argentino, B2
de inglés.

La combinación **Next.js + automation + AI + experiencia real construyendo un
SaaS de 50 clientes** te diferencia de "otro Next.js dev más".

## Hard preferences

| Campo | Valor |
|---|---|
| Modalidad | Remoto o híbrido (sin presencial) |
| Ubicación de empresa | Global |
| Idiomas que aceptás | Español (nativo), Inglés B2 |
| Sueldo mínimo | USD 1000 / mes (~ USD 12k/año floor; piso bajo, la mayoría de roles remotos pagan más) |
| Industrias bloqueadas | Ninguna por ahora |
| Tamaño de empresa | Cualquiera |
| Stack preferido | Next.js / TypeScript / PostgreSQL+Supabase / n8n / Make / integraciones LLM |
| Red flags duros | Ninguno declarado por ahora |

## Roles a apuntar (en orden de fit)

1. **AI Automation Engineer / AI Integrations Engineer** — el rol más "tuyo" y menos competitivo. n8n + Make + LLMs en pipeline real ya lo hiciste en Powing.
2. **Fullstack Engineer (Next.js / TS) con foco AI** — combinación Next + Supabase + LLMs encaja con cualquier startup armando MVP con IA.
3. **Workflow / Automation Specialist** (n8n / Make / Zapier) — mercado chico pero muy pagador; los vendors (n8n.io, Make.com) suelen reclutar directo.
4. **SDR / RevOps Engineer** — sales prospecting + IA tira al lado growth/RevOps engineering, raro pero bien pago cuando existe.
5. **QA Engineer** *(fallback)* — 3 años freelance QA + marcas top (HBO, Airbnb, Meta, Audi, IKEA, MoneyGram); úsalo como red de seguridad si lo anterior se seca.

## Seniority targeting

- **Startups 10-80 personas** → apuntar SSR/Mid. Pesa más "1 año fullstack + build de SaaS real" que el título.
- **Empresas medianas/grandes** → apuntar Junior+/SSR. No te metas a Senior salvo en rol AI-engineer puro.
- En LinkedIn **no filtres por seniority** — los recruiters lo escriben muy inconsistente. Filtrá por keywords del rol.

## Tipos de empresa a apuntar

| Archetipo | Por qué | Ejemplos |
|---|---|---|
| Latam staffing / nearshore | Piso USD 2-5k/mes, sin fricción con Argentina | Howdy.com, Roota, Strider, Crowdbotics, Atomic, Plurawl, Listopro, Andela Latam, Workana (proyectos) |
| Startups US/EU Seed/Series A remote-first | Founder o head of eng entrevista; valoran autonomía | Wellfound + LinkedIn |
| Vendors del stack que usás | Roles directos en empresa que hace el producto | n8n.io, Make.com, Supabase, Vercel, Clerk, Unipile |
| SaaS con foco LATAM | Hablan español + necesitan el perfil | Truora, Vexi, Cobre, Higo, Kuspit, Vincu, NotCo |
| Argentine scaleups remote-friendly | Locales, fácil arrancar charla en español | MercadoLibre, Globant, 10Pines, Aerolab, Mural, Eventbrite BA, Crehana |

## Anti-patrones (no apuntar)

- Big tech US (Google/Meta/Amazon) — competís contra 5000 personas y tu seniority no rinde.
- Empresas cuya descripción dice "fast-paced + ownership + grit + on-call" sin más detalle — código de burnout y sueldo bajo.
- Roles que filtran por "Argentina-based" — el "remote LATAM" es mucho más permeable.

## Búsquedas concretas para LinkedIn (boolean copy-paste)

### A — AI/automation recruiters (sweet spot)
```
("tech recruiter" OR "talent acquisition" OR sourcer OR "hiring manager")
AND ("AI engineer" OR "automation" OR "n8n" OR "LLM" OR "workflow")
AND (remote OR latam OR "latin america")
```

### B — Fullstack/Next.js recruiters
```
("tech recruiter" OR "technical recruiter" OR "talent partner")
AND ("Next.js" OR "Nextjs" OR "Typescript" OR "fullstack")
AND (remote OR latam OR nearshore OR argentina)
```

### C — Founders/CTOs de startups chicas (contratan directo)
```
(founder OR cofounder OR CTO OR "head of engineering")
AND ("AI" OR "GPT" OR "LLM" OR "n8n" OR automation)
AND ("hiring" OR "we're hiring" OR "looking for")
```

### D — Agencias de nearshore Latam
```
("talent acquisition" OR "talent partner" OR sourcer)
AND (nearshore OR "latin america" OR "LATAM")
AND (engineer OR developer OR software)
```

### E — Fallback QA
```
("tech recruiter" OR "QA recruiter") AND (QA OR "quality assurance" OR "test engineer")
AND (remote OR latam)
```

### Filtros que aplicar a cada query

- **Location:** Argentina, México, Colombia, España, United States (TX/FL/CA/NY).
- **Industry:** Computer Software, Internet, IT Services and IT Consulting, Staffing and Recruiting.
- **Network:** 2nd + 3rd grado.
- **"People who posted about ..."** con keywords tipo `"hiring fullstack"` para encontrar recruiters activos AHORA.

### Tip operativo

LinkedIn limita búsquedas anónimas a ~3-5 páginas si no tenés Premium. **Sales
Navigator $80/mes (free trial el primer mes)** multiplica resultados x10.
Cancelar antes del cobro si la prueba alcanza.

## Hooks de outreach (≤200 chars — caben en la nota de invitación)

### Variante ES — humilde y específica
> Hola {nombre}, vi que trabajás con talento técnico {empresa}. Soy fullstack TS+Next con foco en AI/automation (n8n, LLMs, Unipile). Busco rol remoto — ¿charlamos?

### Variante EN — confident, anglo-friendly
> Hi {firstName}, fullstack TS+Next engineer with shipped AI automation work (n8n, LLMs, Unipile SaaS for 50+ clients). Open to remote roles {empresa}. Worth a chat?

### Variante mixed — para recruiters nearshore
> Hola {nombre}, vi que reclutás dev nearshore. 1 año fullstack Next/Supabase + AI integrations (Powing, SaaS LinkedIn/WA). Remoto desde Argentina, B2 EN. Open to chat.

Cuando se arme el primer campaign en Buscalaburos: dejar **Variante ES** como
`A`, **Variante EN** como `B`, A/B-testear con `internal/safety/abtest.go`.

## Errores a evitar al ejecutar la búsqueda

- No mandar el mismo mensaje a recruiter genérico y a founder de startup. Founders responden mejor a hook técnico ("acabás de postear sobre tu pipeline RAG — ¿alguien lo está manteniendo?"). Recruiters responden mejor a hook profesional/CV.
- No empezar por big tech.
- Evitar empresas con descripción "fast-paced + ownership + grit + on-call".
- No filtrar por "Argentina-based" — usar "remote LATAM" o sólo "remote".
