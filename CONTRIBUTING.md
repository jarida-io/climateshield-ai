# Contributing to ClimateShield AI

Thank you for your interest in contributing. ClimateShield AI is an open-source platform that uses climate data to predict disease outbreaks and alert parents of under-vaccinated children in Kenya. Contributions across all components — data science, backend, mobile, frontend, and documentation — are welcome.

## Table of Contents

- [How to Contribute](#how-to-contribute)
- [Development Setup](#development-setup)
- [Project Structure](#project-structure)
- [Coding Standards](#coding-standards)
- [Pull Request Process](#pull-request-process)
- [Issue Reporting](#issue-reporting)
- [Community Guidelines](#community-guidelines)

## How to Contribute

We welcome:

- **ML model improvement** (`ml-predictor/`): Training data, feature engineering, model evaluation
- **Climate data expansion**: Adding more Kenyan counties or integrating Kenya Met Department data
- **SMS/USSD gateway integration** (`alert-engine/`): Connect a live mobile gateway (Africa's Talking, Twilio, or equivalent)
- **Dashboard development** (`dashboard/`): React + Leaflet county heatmap implementation
- **Immunization integration** (`immunization-integration/`): Open health-data standards integration with the Child Immunization Tracker
- **Swahili translations**: Review and expand Swahili SMS templates in `alert-engine/sms_scaffold.py`
- **Documentation**: Architecture docs, API reference, deployment guides

## Development Setup

### Climate Engine

The only dependency is Python 3.x and `requests`.

```bash
git clone https://github.com/jarida-io/climateshield-ai.git
cd climateshield-ai/climate-engine
pip install -r requirements.txt
python ingest.py
```

No API key is required. The climate engine uses the [Open-Meteo API](https://open-meteo.com/), which is free and open.

### ML Predictor

```bash
cd ml-predictor
pip install -r requirements.txt
```

Training data is needed to fit the model — see `ml-predictor/README.md` for the expected feature format.

### Alert Engine

```bash
cd alert-engine
pip install requests  # or add to a requirements.txt
```

To test SMS delivery, you will need credentials for a mobile gateway (Africa's Talking sandbox is free for testing).

### Dashboard (planned)

```bash
cd dashboard
# React + Leaflet — setup instructions coming as implementation progresses
```

## Project Structure

```
climateshield-ai/
├── climate-engine/
│   ├── ingest.py           # Working: Open-Meteo API fetch + risk scoring
│   └── requirements.txt
├── ml-predictor/
│   ├── model_scaffold.py   # GradientBoosting scaffold
│   └── requirements.txt
├── alert-engine/
│   └── sms_scaffold.py     # SMS/USSD templates (EN + Swahili)
├── dashboard/              # County heatmap (in development)
├── immunization-integration/  # Child Immunization Tracker integration spec
└── docs/
    ├── architecture.md     # Full data-flow documentation
    └── api-reference.md    # Function-level API reference
```

## Coding Standards

### Python

- Format with [Black](https://black.readthedocs.io/) (`black .`)
- Type hints on all function signatures
- Docstrings on all public functions (one-line summary + Args/Returns)
- No hardcoded credentials — use environment variables or a config file excluded from version control

```python
def fetch_climate_data(county_name: str, days: int = 14) -> dict:
    """Fetch weather forecast from Open-Meteo for a Kenyan county.

    Args:
        county_name: One of the keys in KENYA_COUNTIES.
        days: Forecast horizon in days (max 16).

    Returns:
        Dict with 'county', 'fetched_at', and 'forecast' keys.
    """
```

### Commit Message Format

Use [Conventional Commits](https://www.conventionalcommits.org/):

```
feat(climate-engine): add Kisii and Garissa counties
fix(ml-predictor): correct feature normalisation before training
docs: update architecture diagram with dashboard component
test(alert-engine): add unit tests for Swahili template rendering
```

## Pull Request Process

1. Fork the repository and create a branch from `main`:
   ```bash
   git checkout -b feat/your-description
   ```
2. Make your changes following the coding standards above
3. Add or update tests where applicable
4. Describe in the PR:
   - What you changed and why
   - How you tested it
   - Any known limitations
5. A maintainer will review and merge

### PR Checklist

- [ ] No credentials or API keys committed
- [ ] Type hints and docstrings on new Python functions
- [ ] Formatted with Black
- [ ] PR description explains what was tested and how

## Issue Reporting

### Bug Reports

Please include:
- Which component (`climate-engine`, `ml-predictor`, `alert-engine`, `dashboard`)
- What you expected to happen
- What actually happened
- Python version and OS
- Full traceback if applicable

### Feature Requests

Please describe:
- The problem you are solving
- Your proposed solution
- How it fits into the overall ClimateShield AI pipeline

### Security Issues

Do not open a public issue for security vulnerabilities. Email **hello@jarida.io** with details.

## Community Guidelines

- Be respectful and constructive
- This platform handles child health data — accuracy and correctness matter above all else
- For significant architectural changes, open an issue to discuss the approach before writing code
- Public health domain knowledge is as valuable as coding skill — contributions from epidemiologists, CHWs, and health data specialists are very welcome

## Contact

- **Issues and feature requests**: [GitHub Issues](https://github.com/jarida-io/climateshield-ai/issues)
- **General questions**: [GitHub Discussions](https://github.com/jarida-io/climateshield-ai/discussions)
- **Security concerns**: hello@jarida.io

## License

By contributing, you agree that your contributions will be licensed under the Apache 2.0 License.
