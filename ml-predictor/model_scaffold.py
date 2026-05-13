"""
ClimateShield AI — ML Outbreak Predictor Scaffold
Apache 2.0 License — Jarida Open Source
"""

from sklearn.ensemble import GradientBoostingClassifier
from sklearn.model_selection import train_test_split
from sklearn.metrics import classification_report
import numpy as np


FEATURES = [
    "precipitation_sum_14d",
    "temp_max_avg_14d",
    "temp_min_avg_14d",
    "humidity_max_avg_14d",
]

DISEASES = ["cholera", "malaria", "pneumonia", "meningitis"]


def train_model(X: np.ndarray, y: np.ndarray) -> GradientBoostingClassifier:
    X_train, X_test, y_train, y_test = train_test_split(X, y, test_size=0.2, random_state=42)
    model = GradientBoostingClassifier(n_estimators=100, random_state=42)
    model.fit(X_train, y_train)
    print(classification_report(y_test, model.predict(X_test)))
    return model


def predict_risk(model: GradientBoostingClassifier, climate_features: list) -> dict:
    X = np.array(climate_features).reshape(1, -1)
    probability = model.predict_proba(X)[0][1]
    return {
        "outbreak_probability": round(probability, 3),
        "risk_level": "HIGH" if probability >= 0.6 else "MEDIUM" if probability >= 0.3 else "LOW",
    }
