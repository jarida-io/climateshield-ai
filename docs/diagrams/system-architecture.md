# ClimateShield AI — System Architecture

End-to-end view of how climate data, the immunization tracker, and the alert pipeline connect.

```mermaid
flowchart TB
    subgraph DATA["📡 External Data Sources"]
        OM[Open-Meteo API<br/>14-day forecast per county]
        KMD[Kenya Met Department<br/>historical rainfall]
        MOH[Ministry of Health<br/>KEPI vaccine schedule]
    end

    subgraph ENGINE["🧠 ClimateShield AI Engine"]
        ING[ingest.py<br/>fetches forecasts]
        SCORE[Risk Scoring<br/>HIGH / MEDIUM / LOW<br/>cholera · malaria · pneumonia · meningitis]
        ML[ML Predictor<br/>outbreak probability<br/>7-14 day window]
    end

    subgraph TRACKER["💉 Child Immunization Tracker"]
        DB[(MySQL Database<br/>children · vaccines<br/>schedule · users)]
        WEB[PHP Web App<br/>Apache + Bootstrap]
        AUTH[Auth + 2FA<br/>session-based]
    end

    subgraph ALERT["📨 Alert Dispatch"]
        QUERY[Query: under-vaccinated<br/>children in HIGH risk county]
        AT[Africa's Talking<br/>SMS + USSD gateway]
        SMS[SMS to parent phone]
        USSD[USSD on feature phones]
        INAPP[In-app notifications]
    end

    subgraph USERS["👥 End Users"]
        PARENT[Parent / Guardian]
        CHW[Community Health Worker]
        ADMIN[County Health Officer]
    end

    OM --> ING
    KMD --> ING
    MOH --> DB
    ING --> SCORE
    SCORE --> ML
    ML --> QUERY
    QUERY --> DB
    DB --> WEB
    WEB --> AUTH
    AUTH --> PARENT
    AUTH --> CHW
    AUTH --> ADMIN
    QUERY --> AT
    AT --> SMS
    AT --> USSD
    SMS --> PARENT
    USSD --> PARENT
    WEB --> INAPP
    INAPP --> PARENT
    INAPP --> CHW

    classDef external fill:#E3F2FD,stroke:#1976D2,color:#0D47A1
    classDef engine fill:#FFF3E0,stroke:#F57C00,color:#E65100
    classDef tracker fill:#E8F5E9,stroke:#388E3C,color:#1B5E20
    classDef alert fill:#FFEBEE,stroke:#D32F2F,color:#B71C1C
    classDef users fill:#F3E5F5,stroke:#7B1FA2,color:#4A148C

    class OM,KMD,MOH external
    class ING,SCORE,ML engine
    class DB,WEB,AUTH tracker
    class QUERY,AT,SMS,USSD,INAPP alert
    class PARENT,CHW,ADMIN users
```

## Key flows

1. **Daily ingest** — `ingest.py` runs on cron, fetches 14-day forecasts for all 47 counties.
2. **Risk scoring** — peak rainfall and average temperature mapped to three-tier risk per disease.
3. **Cross-reference** — when a county hits HIGH/MEDIUM, query the tracker for under-vaccinated children there.
4. **Dispatch** — Africa's Talking sends SMS (smartphone or feature phone) and the in-app notification fires.
5. **Parent action** — parent visits clinic; healthcare worker marks dose completed; status syncs back.
