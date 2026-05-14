# Data Flow

How a single record moves through the system, from weather forecast to parent's phone.

```mermaid
flowchart LR
    subgraph IN[Input]
        W[Weather API<br/>JSON]
        K[KEPI schedule<br/>SQL seed]
        R[Child registry<br/>via guardian signup]
    end

    subgraph PROC[Processing]
        N[Normalize<br/>per-county]
        S[Score<br/>HIGH/MED/LOW]
        J[Join: county risk<br/>× under-vaccinated kids]
        F[Format SMS<br/>+254 E.164]
    end

    subgraph OUT[Output]
        SM[SMS via AT]
        UD[USSD callback]
        NF[In-app notification]
        AU[Audit log]
    end

    W --> N
    K --> R
    R --> J
    N --> S
    S --> J
    J --> F
    F --> SM
    F --> NF
    F --> AU
    UD <--> R

    classDef in fill:#E3F2FD,stroke:#1976D2
    classDef proc fill:#FFF3E0,stroke:#F57C00
    classDef out fill:#E8F5E9,stroke:#388E3C
    class W,K,R in
    class N,S,J,F proc
    class SM,UD,NF,AU out
```

## Data contracts

### Climate ingest output (one record per county)
```json
{
  "county": "Kisumu",
  "peak_rainfall_mm": 74.0,
  "avg_temp_max_c": 28.1,
  "risk_scores": {
    "cholera":    "HIGH",
    "malaria":    "HIGH",
    "pneumonia":  "LOW",
    "meningitis": "LOW"
  },
  "scored_at": "2026-05-14T08:00:00+03:00"
}
```

### Tracker query (children at risk in HIGH county)
```sql
SELECT c.child_id, c.name, c.health_id,
       u.name AS guardian_name, u.phone AS guardian_phone,
       COUNT(vs.schedule_id) AS missed_doses
FROM children c
JOIN users u ON c.guardian_id = u.user_id
JOIN vaccination_schedule vs ON vs.child_id = c.child_id
WHERE u.location = 'Kisumu'
  AND vs.status IN ('Missed', 'Pending')
GROUP BY c.child_id
HAVING missed_doses > 0;
```

### SMS payload (Africa's Talking)
```json
{
  "to": "+254733000004",
  "from": "Jarida",
  "message": "🚨 High cholera risk in Kisumu (next 7-14 days). Your child Zuri has 8 missed doses. Visit Kisumu County Hospital. — ClimateShield AI"
}
```
