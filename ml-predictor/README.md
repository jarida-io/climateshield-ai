# ML Predictor

Forecasts disease outbreak probability 7–14 days ahead using climate features from the Climate Engine.

## Status

Model in training. Planned approach: scikit-learn gradient boosting on historical climate + outbreak correlation data.

## Planned Input Features

- 14-day precipitation sum
- 14-day max/min temperature
- Relative humidity
- County-level historical outbreak data
