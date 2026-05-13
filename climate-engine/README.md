# Climate Engine

Ingests real-time weather forecast data from the Open-Meteo API for 5 Kenyan counties and scores outbreak risk per disease category.

## Usage

```bash
pip install -r requirements.txt
python ingest.py
```

## Output

JSON risk scores per county, per disease (cholera, malaria, pneumonia, meningitis).
