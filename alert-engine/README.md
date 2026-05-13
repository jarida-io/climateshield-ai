# Alert Engine

Sends SMS/USSD reminders to parents of at-risk children when the ML Predictor flags high outbreak probability for their county.

## Status

In development. Integrates with Child Immunization Tracker to query under-vaccinated children by county.

## Flow

1. Receive high-risk alert from ML Predictor
2. Query Child Immunization Tracker for under-vaccinated children in affected county
3. Send SMS/USSD to registered parent phone numbers via mobile gateway
4. Push alert to registered Community Health Workers
