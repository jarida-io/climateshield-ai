# Architecture

ClimateShield AI is a pipeline of four loosely-coupled components.

```
Open-Meteo API
      |
      v
Climate Engine          — Fetches 14-day weather forecasts per county
      |
      v
ML Predictor            — Scores outbreak probability (cholera, malaria, pneumonia, meningitis)
      |
 [HIGH risk?]
      |
      v
Alert Engine            — Queries Child Immunization Tracker for under-vaccinated children
      |                   Sends SMS/USSD to parents and CHWs
      v
County Dashboard        — Real-time risk heatmap for health officers
```

## Data Flow

1. **Climate Engine** calls Open-Meteo API every 6 hours for 5 Kenyan counties (Nairobi, Kisumu, Mombasa, Nakuru, Eldoret).
2. **ML Predictor** receives the 14-day forecast features and outputs a risk probability per disease.
3. **Alert Engine** triggers when any disease risk >= HIGH. It queries the Child Immunization Tracker for children with overdue vaccinations in the affected county, then sends SMS/USSD alerts.
4. **County Dashboard** displays a live risk heatmap and logs all alerts for health officers.

## Offline / Edge AI

The KSL Translator project (`jarida-io/kenyan_sign_language_app`) demonstrates Jarida's on-device ML capability. The CHW Android module for ClimateShield AI will use the same MediaPipe Tasks architecture for offline risk assessment in low-connectivity areas.
