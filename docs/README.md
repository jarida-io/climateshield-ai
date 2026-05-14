# ClimateShield AI — Documentation

Architecture diagrams, wireframes, and design artefacts supporting the UNICEF Venture Fund grant application.

## Diagrams (Mermaid — render natively on GitHub)

| File | What it shows |
|---|---|
| [System Architecture](diagrams/system-architecture.md) | End-to-end: weather APIs → climate engine → immunization tracker → SMS/USSD |
| [Parent Alert Journey](diagrams/parent-alert-journey.md) | Sequence diagram of one alert from forecast to clinic visit |
| [Risk Scoring Logic](diagrams/risk-scoring.md) | How HIGH / MEDIUM / LOW are decided per disease |
| [Data Flow](diagrams/data-flow.md) | Data contracts between subsystems |

## Wireframes (SVG)

| File | Screen |
|---|---|
| [Guardian Dashboard](wireframes/01-guardian-dashboard.svg) | Parent-facing home screen |
| [Vaccination Schedule](wireframes/02-child-vaccination-schedule.svg) | Per-child KEPI schedule with missed/pending tracking |
| [Admin Dashboard](wireframes/03-admin-dashboard.svg) | County health officer command centre |
| [SMS & USSD](wireframes/04-sms-and-ussd.svg) | Both parent-facing channels side by side |

## Companion repository

The PHP web app, MySQL schema, and demo seed data live in the **[Child-Immunization-Tracker](https://github.com/jarida-io/Child-Immunization-Tracker)** repo. The video demo seed (`docker/demo_seed.sql`) and recording instructions (`VIDEO_DEMO_INSTRUCTIONS.md`) are there.
