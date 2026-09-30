FROM python:3.12-slim

WORKDIR /app
COPY requirements.txt .
RUN pip install --no-cache-dir -r requirements.txt

COPY client.py analysis.py surrogate.py plots.py handlers.py app.py ./

# Roda sem root. O Gradio precisa de uma pasta temporária com escrita.
RUN useradd --system --uid 10001 app && mkdir -p /tmp/gradio && chown app /tmp/gradio
USER app

ENV GRADIO_SERVER_NAME=0.0.0.0 \
    GRADIO_SERVER_PORT=7860 \
    GRADIO_ANALYTICS_ENABLED=False \
    GRADIO_TEMP_DIR=/tmp/gradio \
    BACKEND_API_URL=http://fraud-api:8080

EXPOSE 7860
HEALTHCHECK --interval=15s --timeout=5s --retries=5 --start-period=20s \
  CMD python -c "import urllib.request; urllib.request.urlopen('http://127.0.0.1:7860/', timeout=3)"

CMD ["python", "app.py"]
