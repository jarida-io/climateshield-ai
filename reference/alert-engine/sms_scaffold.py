"""
ClimateShield AI — SMS/USSD Alert Engine Scaffold
Apache 2.0 License — Jarida Open Source
"""

import requests
from dataclasses import dataclass


@dataclass
class AlertPayload:
    county: str
    disease: str
    risk_level: str
    parent_phone: str
    child_name: str
    vaccine_due: str


SMS_TEMPLATE_EN = (
    "ClimateShield Alert: {disease} risk is {risk_level} in {county}. "
    "{child_name} is due for {vaccine_due}. Visit your nearest clinic. "
    "Reply STOP to opt out."
)

SMS_TEMPLATE_SW = (
    "Tahadhari ya ClimateShield: Hatari ya {disease} ni {risk_level} katika {county}. "
    "{child_name} anahitaji chanjo ya {vaccine_due}. Tembelea kliniki yako. "
    "Jibu STOP kujiondoa."
)


def send_sms(payload: AlertPayload, gateway_url: str, api_key: str, lang: str = "en") -> dict:
    template = SMS_TEMPLATE_SW if lang == "sw" else SMS_TEMPLATE_EN
    message = template.format(
        disease=payload.disease,
        risk_level=payload.risk_level,
        county=payload.county,
        child_name=payload.child_name,
        vaccine_due=payload.vaccine_due,
    )
    response = requests.post(
        gateway_url,
        json={"to": payload.parent_phone, "message": message},
        headers={"Authorization": f"Bearer {api_key}"},
    )
    return {"status": response.status_code, "phone": payload.parent_phone}
