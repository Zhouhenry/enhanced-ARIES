# Build only on Linux ECS. build.py supplies verified local contexts and pins.
ARG BASE_IMAGE=aries-local/jiuwen-python:base-6a0e06f5ba0b
FROM ${BASE_IMAGE}
USER root
ENV PYTHONDONTWRITEBYTECODE=1 \
    UV_PYTHON_DOWNLOADS=never \
    UV_LINK_MODE=copy
RUN apt-get update \
    && apt-get install -y --no-install-recommends git passwd tini ca-certificates \
    && rm -rf /var/lib/apt/lists/* \
    && groupadd --gid 60000 aries-agent \
    && useradd --uid 60000 --gid 60000 --create-home --shell /bin/bash aries-agent \
    && install -d -m 0755 /opt/jiuwenswarm /opt/aries-envd /opt/aries-template /workspace/aries-case
COPY --chown=0:0 source/ /opt/jiuwenswarm/
COPY --chown=0:0 --chmod=0555 uv /usr/local/bin/uv
COPY --chown=0:0 --chmod=0555 envd /opt/aries-envd/envd
COPY --chown=0:0 pytest-wheels/ /opt/aries-build/pytest-wheels/
COPY --chown=0:0 --chmod=0444 aries-build.json /opt/jiuwenswarm/aries-build.json
COPY --chown=0:0 --chmod=0555 verify.py /opt/aries-template/verify.py
COPY --chown=0:0 --chmod=0500 start.sh /opt/aries-template/start.sh
WORKDIR /opt/jiuwenswarm
# uv.lock is unchanged. The offline cache includes the exact locked Git commits;
# agent-core came from its verified same-commit file mirror, not a new revision.
RUN --mount=type=bind,from=uv_cache,target=/root/.cache/uv,rw \
    uv sync --frozen --offline --no-default-groups --python /usr/local/bin/python3 \
    && uv pip install --python /usr/local/bin/python3 --no-index --no-deps /opt/aries-build/pytest-wheels/*.whl \
    && rm -rf /opt/aries-build/pytest-wheels \
    && dpkg-query -W > /opt/aries-template/os-packages.txt \
    && chmod -R u+rwX,a+rX,go-w /opt/jiuwenswarm \
    && chmod -R go-w /opt/aries-template /opt/aries-envd /usr/local \
    && python3 -I /opt/aries-template/verify.py > /opt/aries-template/image-evidence.json \
    && chmod 0444 /opt/aries-template/image-evidence.json
WORKDIR /workspace/aries-case
EXPOSE 49983
# Huawei uses this existing ENVD protocol. E2B supplies its own runtime/start command.
# cgroup delegation and populated=0 are checked by the Harness in the real guest.
ENTRYPOINT ["/usr/bin/tini", "-g", "--", "/opt/aries-envd/envd", "-isnotfc", "-port", "49983"]
CMD []
