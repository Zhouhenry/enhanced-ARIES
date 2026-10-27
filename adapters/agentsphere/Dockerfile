FROM python:3.12-slim-bookworm@sha256:6a0e06f5ba0b373d051aabd3fffb9b873f27f9a2a3a14bcbe37dee6f0e3bbc70
WORKDIR /app
COPY requirements.txt e2b-2.42.0-py3-none-any.whl agentsphere_e2b_patch-0.1.0-py3-none-any.whl /tmp/wheels/
RUN echo 'e0bb408fa1ee9675e8ca999e47d9c548dd47e3073a96378532329abb7899d194  /tmp/wheels/e2b-2.42.0-py3-none-any.whl' | sha256sum -c - \
    && echo '9fd380dbccfa8b86e6249722c273600824f42df6974d1fd35e918ca18751ffc4  /tmp/wheels/agentsphere_e2b_patch-0.1.0-py3-none-any.whl' | sha256sum -c - \
    && pip install --no-cache-dir -r /tmp/wheels/requirements.txt \
    && pip check \
    && useradd --uid 10001 --create-home adapter \
    && mkdir /state && chown 10001:10001 /state && chmod 0700 /state
COPY e2b/server.py e2b/guest_exec.py /app/e2b/
COPY agentsphere/agentsphere.py /app/agentsphere/
RUN chmod 0755 /app/e2b /app/agentsphere \
    && chmod 0644 /app/e2b/server.py /app/e2b/guest_exec.py /app/agentsphere/agentsphere.py
USER 10001:10001
EXPOSE 8788
ENTRYPOINT ["python", "/app/agentsphere/agentsphere.py", "--listen", "0.0.0.0:8788", "--state-file", "/state/agentsphere.state.json"]
