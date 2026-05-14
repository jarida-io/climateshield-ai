# Outbreak Risk Scoring Logic

How ClimateShield AI decides whether a county is at HIGH, MEDIUM, or LOW outbreak risk for each disease.

```mermaid
flowchart TD
    START([14-day forecast for county]) --> EXTRACT[Extract:<br/>peak rainfall mm<br/>avg max temperature °C]

    EXTRACT --> CHOLERA{Cholera<br/>rainfall ≥ 60mm?}
    EXTRACT --> MALARIA{Malaria<br/>rainfall ≥ 40mm?}
    EXTRACT --> PNEUMONIA{Pneumonia<br/>temp ≤ 16°C?}
    EXTRACT --> MENINGITIS{Meningitis<br/>temp ≥ 39°C?}

    CHOLERA -->|Yes| CH[Cholera = HIGH]
    CHOLERA -->|No, ≥30mm| CM[Cholera = MEDIUM]
    CHOLERA -->|No| CL[Cholera = LOW]

    MALARIA -->|Yes| MH[Malaria = HIGH]
    MALARIA -->|No, ≥20mm| MM[Malaria = MEDIUM]
    MALARIA -->|No| ML[Malaria = LOW]

    PNEUMONIA -->|Yes| PH[Pneumonia = HIGH]
    PNEUMONIA -->|No, ≤19°C| PM[Pneumonia = MEDIUM]
    PNEUMONIA -->|No| PL[Pneumonia = LOW]

    MENINGITIS -->|Yes| MEH[Meningitis = HIGH]
    MENINGITIS -->|No, ≥36°C| MEM[Meningitis = MEDIUM]
    MENINGITIS -->|No| MEL[Meningitis = LOW]

    CH --> DECISION{Any HIGH or MEDIUM?}
    CM --> DECISION
    MH --> DECISION
    MM --> DECISION
    PH --> DECISION
    PM --> DECISION
    MEH --> DECISION
    MEM --> DECISION
    CL --> DECISION
    ML --> DECISION
    PL --> DECISION
    MEL --> DECISION

    DECISION -->|Yes| ALERT[🚨 Query under-vaccinated<br/>children in county]
    DECISION -->|No| LOG[Log scores · no action]

    ALERT --> SMS[Dispatch SMS via<br/>Africa's Talking]
    SMS --> END([End])
    LOG --> END

    classDef threshold fill:#FFF3E0,stroke:#F57C00
    classDef high fill:#FFCDD2,stroke:#C62828,color:#B71C1C
    classDef medium fill:#FFE0B2,stroke:#EF6C00,color:#E65100
    classDef low fill:#C8E6C9,stroke:#2E7D32,color:#1B5E20
    classDef action fill:#E1BEE7,stroke:#6A1B9A,color:#4A148C

    class CHOLERA,MALARIA,PNEUMONIA,MENINGITIS threshold
    class CH,MH,PH,MEH high
    class CM,MM,PM,MEM medium
    class CL,ML,PL,MEL low
    class ALERT,SMS action
```

## Thresholds (v1 — based on Kenya MoH outbreak data 2015–2023)

| Disease | Driver | HIGH | MEDIUM | Rationale |
|---|---|---|---|---|
| **Cholera** | 14-day peak rainfall | ≥ 60 mm | ≥ 30 mm | Heavy rain → contaminated water sources |
| **Malaria** | 14-day peak rainfall | ≥ 40 mm | ≥ 20 mm | Standing water → mosquito breeding 7–14 days later |
| **Pneumonia** | 14-day avg max temp | ≤ 16 °C | ≤ 19 °C | Cold stress → respiratory infection in under-5s |
| **Meningitis** | 14-day avg max temp | ≥ 39 °C | ≥ 36 °C | Heat + dust season → meningococcal spread |

> Thresholds are calibrated against historical KEPI surveillance reports. v2 will replace static thresholds with a gradient-boosted model trained on 2015–2024 county-level outbreak history (see `ml-predictor/` in the climateshield-ai repo).
