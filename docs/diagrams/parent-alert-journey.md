# Parent Alert Journey

What happens, step by step, when ClimateShield AI detects an outbreak risk in a parent's county.

```mermaid
sequenceDiagram
    autonumber
    participant API as Open-Meteo API
    participant CS as ClimateShield Engine
    participant DB as Tracker DB
    participant AT as Africa's Talking
    participant P as Parent (Akinyi · Kisumu)
    participant C as Clinic / CHW

    Note over API,CS: 08:00 EAT — Daily ingest
    CS->>API: GET /forecast?lat=-0.10&lon=34.76&days=14
    API-->>CS: 74mm rainfall · 28.1°C avg

    Note over CS: Score: Cholera HIGH, Malaria HIGH

    CS->>DB: SELECT children WHERE county='Kisumu' AND missed_doses > 0
    DB-->>CS: Zuri Otieno (8 missed), Kofi Otieno (2 missed)

    CS->>AT: Send SMS to +254733000004
    AT->>P: 🚨 "High cholera risk in Kisumu. Zuri has 8 missed doses. Visit Kisumu Hospital."
    CS->>DB: INSERT notifications (climate alert)

    P->>P: Reads SMS
    P->>C: Walks in / dials USSD shortcode

    C->>DB: Mark Measles 1 + Yellow Fever as Completed
    DB-->>P: Confirmation SMS via AT
    AT->>P: ✅ "Zuri received Measles 1 today. Next dose: Vitamin A 2 due Aug 15."

    Note over P,C: Outbreak prevented before peak
```

## Timeline reality check

| Step | Latency in pilot |
|---|---|
| Climate ingest → risk score | < 5 seconds per county |
| Risk score → SMS dispatched | < 8 seconds (sandbox) |
| SMS read → clinic visit | Same day to 7 days (parent decision) |
| Forecast window for action | 7–14 days before predicted outbreak peak |
