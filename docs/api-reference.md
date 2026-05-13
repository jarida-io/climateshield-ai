# API Reference

## Climate Engine

### `fetch_climate_data(county_name, days=14) -> dict`

Fetches weather forecast data from Open-Meteo API.

**Parameters:**
- `county_name` — One of: Nairobi, Kisumu, Mombasa, Nakuru, Eldoret
- `days` — Forecast horizon in days (default: 14)

**Returns:** `{ county, fetched_at, forecast: { precipitation_sum, temperature_2m_max, temperature_2m_min, relative_humidity_2m_max } }`

### `score_outbreak_risk(data) -> dict`

Applies threshold rules to raw forecast data.

**Returns:** `{ county, risk_scores: { cholera, malaria, pneumonia, meningitis }, scored_at }`

## ML Predictor

### `train_model(X, y) -> GradientBoostingClassifier`

Trains the outbreak prediction model. Input features: `[precipitation_sum_14d, temp_max_avg_14d, temp_min_avg_14d, humidity_max_avg_14d]`.

### `predict_risk(model, climate_features) -> dict`

**Returns:** `{ outbreak_probability, risk_level: HIGH | MEDIUM | LOW }`

## Alert Engine

### `send_sms(payload, gateway_url, api_key, lang="en") -> dict`

Sends an SMS alert via the configured mobile gateway.

**Parameters:**
- `payload` — `AlertPayload` dataclass (county, disease, risk_level, parent_phone, child_name, vaccine_due)
- `lang` — `"en"` (English) or `"sw"` (Swahili)

**Returns:** `{ status: HTTP status code, phone }`
