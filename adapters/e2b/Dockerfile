FROM python:3.12-slim-bookworm@sha256:6a0e06f5ba0b373d051aabd3fffb9b873f27f9a2a3a14bcbe37dee6f0e3bbc70
WORKDIR /app
COPY requirements.txt constraints.txt ./
RUN pip install --no-cache-dir -r requirements.txt \
    && useradd --uid 10001 --create-home adapter \
    && mkdir /state && chown 10001:10001 /state && chmod 0700 /state
COPY server.py guest_exec.py smoke.py ./
USER 10001:10001
EXPOSE 8787
ENTRYPOINT ["python", "server.py", "--listen", "0.0.0.0:8787", "--state-file", "/state/adapter.state.json"]
