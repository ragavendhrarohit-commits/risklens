# RiskLens

Independent cybersecurity monitoring UI. Elasticsearch, Kibana, Fleet, and Elastic Agent stay unmodified in the background. This application does not fork or copy Kibana, and it does not use Elastic UI.

## Run locally

1. Copy `.env.example` to `.env`.
2. Create an Elasticsearch API key in Kibana (Stack Management → API keys) for the existing `thiru-search` container at `http://localhost:9200`.
3. Put the key in `.env` as `ELASTICSEARCH_API_KEY`. Do not put credentials in the frontend.
4. Install and start:

```powershell
python -m venv .venv
.\.venv\Scripts\Activate.ps1
pip install -r requirements.txt
uvicorn backend.app.main:app --reload --host 127.0.0.1 --port 8080
```

Open [http://127.0.0.1:8080](http://127.0.0.1:8080).

`DATA_MODE=elastic` queries Elasticsearch. If Elasticsearch fails, the UI shows an error. It does not silently switch to mock data.

`DATA_MODE=mock` is only for layout work without a cluster.

## Layout

`frontend/index.html` is the main product shell: Monitor (Overview, Users, Anomalies, System & Service Accounts, Devices, Locations) and Ingest (Data Sources), plus global search and time range.
