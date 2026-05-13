"""
ClimateShield AI — Climate Data Ingestion Engine
Pulls weather forecast data from Open-Meteo API for Kenyan counties.
Apache 2.0 License — Jarida Open Source
"""

import requests
import json
from datetime import datetime

KENYA_COUNTIES = {
    "Nairobi": {"lat": -1.2921, "lon": 36.8219},
    "Kisumu": {"lat": -0.1022, "lon": 34.7617},
    "Mombasa": {"lat": -4.0435, "lon": 39.6682},
    "Nakuru": {"lat": -0.3031, "lon": 36.0800},
    "Eldoret": {"lat": 0.5143, "lon": 35.2698},
}

RISK_THRESHOLDS = {
    "cholera": {"rainfall_mm": 50},
    "malaria": {"rainfall_mm": 30},
    "pneumonia": {"temp_max_below": 18},
    "meningitis": {"temp_max_above": 38},
}


def fetch_climate_data(county_name: str, days: int = 14) -> dict:
    coords = KENYA_COUNTIES[county_name]
    url = "https://api.open-meteo.com/v1/forecast"
    params = {
        "latitude": coords["lat"],
        "longitude": coords["lon"],
        "daily": [
            "precipitation_sum",
            "temperature_2m_max",
            "temperature_2m_min",
            "relative_humidity_2m_max",
        ],
        "timezone": "Africa/Nairobi",
        "forecast_days": days,
    }
    r = requests.get(url, params=params)
    r.raise_for_status()
    return {
        "county": county_name,
        "fetched_at": datetime.now().isoformat(),
        "forecast": r.json()["daily"],
    }


def score_outbreak_risk(data: dict) -> dict:
    daily = data["forecast"]
    rain = max(daily.get("precipitation_sum", [0]))
    temp_values = daily.get("temperature_2m_max", [25])
    temp = sum(temp_values) / len(temp_values)
    return {
        "county": data["county"],
        "risk_scores": {
            "cholera": "HIGH" if rain >= 50 else "LOW",
            "malaria": "HIGH" if rain >= 30 else "LOW",
            "pneumonia": "HIGH" if temp <= 18 else "LOW",
            "meningitis": "HIGH" if temp >= 38 else "LOW",
        },
        "scored_at": datetime.now().isoformat(),
    }


if __name__ == "__main__":
    for county in KENYA_COUNTIES:
        data = fetch_climate_data(county)
        print(json.dumps(score_outbreak_risk(data), indent=2))
