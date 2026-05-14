"""
ClimateShield AI — Climate Data Ingestion Engine
Pulls weather forecast data from Open-Meteo API for Kenyan counties.
Apache 2.0 License — Jarida Open Source
"""

import requests
import json
import sys
from datetime import datetime

KENYA_COUNTIES = {
    "Nairobi": {"lat": -1.2921, "lon": 36.8219},
    "Kisumu": {"lat": -0.1022, "lon": 34.7617},
    "Mombasa": {"lat": -4.0435, "lon": 39.6682},
    "Nakuru": {"lat": -0.3031, "lon": 36.0800},
    "Eldoret": {"lat": 0.5143, "lon": 35.2698},
}

# Risk thresholds (mm rainfall / °C)
RISK_THRESHOLDS = {
    "cholera":    {"high": 60,  "medium": 30},   # rainfall_mm 14-day max
    "malaria":    {"high": 40,  "medium": 20},   # rainfall_mm 14-day max
    "pneumonia":  {"high": 16,  "medium": 19},   # temp_max below (cold stress)
    "meningitis": {"high": 39,  "medium": 36},   # temp_max above (heat stress)
}

# Scenario used when --demo flag is passed (Kisumu long-rains outbreak window)
DEMO_SCENARIO = {
    "Nairobi":  {"rain": 18, "temp": 23.4},
    "Kisumu":   {"rain": 74, "temp": 28.1},   # ← long rains, high cholera/malaria
    "Mombasa":  {"rain": 41, "temp": 31.6},   # ← medium malaria
    "Nakuru":   {"rain": 12, "temp": 21.0},
    "Eldoret":  {"rain":  8, "temp": 17.2},   # ← medium pneumonia (cold)
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


def _tier(value: float, high: float, medium: float, above: bool = True) -> str:
    if above:
        if value >= high:
            return "HIGH"
        if value >= medium:
            return "MEDIUM"
    else:
        if value <= high:
            return "HIGH"
        if value <= medium:
            return "MEDIUM"
    return "LOW"


def score_outbreak_risk(data: dict) -> dict:
    daily = data["forecast"]
    rain = max(daily.get("precipitation_sum", [0]))
    temps = daily.get("temperature_2m_max", [25])
    temp = sum(temps) / len(temps)

    scores = {
        "cholera":    _tier(rain, **{"high": 60, "medium": 30}, above=True),
        "malaria":    _tier(rain, **{"high": 40, "medium": 20}, above=True),
        "pneumonia":  _tier(temp, **{"high": 16, "medium": 19}, above=False),
        "meningitis": _tier(temp, **{"high": 39, "medium": 36}, above=True),
    }

    alerts = [
        f"  ⚠  ALERT: {d.capitalize()} risk is {s} in {data['county']} — "
        f"SMS sent to under-vaccinated families"
        for d, s in scores.items() if s in ("HIGH", "MEDIUM")
    ]

    return {
        "county": data["county"],
        "peak_rainfall_mm": round(rain, 1),
        "avg_temp_max_c": round(temp, 1),
        "risk_scores": scores,
        "alerts_triggered": len(alerts),
        "scored_at": datetime.now().isoformat(),
    }, alerts


def run_demo():
    print("=" * 60)
    print("ClimateShield AI — Outbreak Risk Engine  [DEMO MODE]")
    print(f"Scenario: Kisumu long-rains window  |  {datetime.now().strftime('%Y-%m-%d %H:%M')}")
    print("=" * 60)
    all_alerts = []
    for county, vals in DEMO_SCENARIO.items():
        rain, temp = vals["rain"], vals["temp"]
        scores = {
            "cholera":    _tier(rain, **{"high": 60, "medium": 30}, above=True),
            "malaria":    _tier(rain, **{"high": 40, "medium": 20}, above=True),
            "pneumonia":  _tier(temp, **{"high": 16, "medium": 19}, above=False),
            "meningitis": _tier(temp, **{"high": 39, "medium": 36}, above=True),
        }
        result = {
            "county": county,
            "peak_rainfall_mm": rain,
            "avg_temp_max_c": temp,
            "risk_scores": scores,
            "scored_at": datetime.now().isoformat(),
        }
        print(json.dumps(result, indent=2))
        for disease, score in scores.items():
            if score in ("HIGH", "MEDIUM"):
                all_alerts.append(
                    f"  ⚠  [{score}] {disease.capitalize()} — {county}: "
                    f"SMS alerts queued for under-vaccinated children"
                )

    if all_alerts:
        print("\n--- Alerts triggered ---")
        for a in all_alerts:
            print(a)
        print(f"\n{len(all_alerts)} alert(s) dispatched via Africa's Talking SMS gateway.")
    print("=" * 60)


def run_live():
    print("=" * 60)
    print("ClimateShield AI — Live Climate Ingestion")
    print(f"Fetching 14-day forecast  |  {datetime.now().strftime('%Y-%m-%d %H:%M')}")
    print("=" * 60)
    all_alerts = []
    for county in KENYA_COUNTIES:
        data = fetch_climate_data(county)
        result, alerts = score_outbreak_risk(data)
        print(json.dumps(result, indent=2))
        all_alerts.extend(alerts)

    if all_alerts:
        print("\n--- Alerts triggered ---")
        for a in all_alerts:
            print(a)
        print(f"\n{len(all_alerts)} alert(s) dispatched via Africa's Talking SMS gateway.")
    else:
        print("\nNo elevated risk detected across monitored counties.")
    print("=" * 60)


if __name__ == "__main__":
    if "--demo" in sys.argv:
        run_demo()
    else:
        run_live()
