ARG BASE_IMAGE=swr.cn-southwest-2.myhuaweicloud.com/benchmark-zhoutao/aries-jiuwenswarm:0.2.3-e2c9d519-image7-process1@sha256:ba27c904098a8e41c3efe297e773d16b208c2e77edf38e206b70eb0ee335ce21
FROM ${BASE_IMAGE}
USER root
ENV PATH="/opt/jiuwenswarm/.venv/bin:${PATH}" \
    NODE_PATH="/opt/aries-presentation/node_modules"
RUN apt-get update \
    && apt-get install -y --no-install-recommends nodejs npm libreoffice-impress poppler-utils fontconfig fonts-noto-cjk fonts-liberation2 \
    && mkdir -p /opt/aries-presentation \
    && dpkg-query -W -f='${binary:Package}\t${Version}\n' > /opt/aries-presentation/debian-packages.tsv \
    && apt-get clean \
    && rm -rf /var/lib/apt/lists/* /var/cache/apt/archives/*
COPY --chown=0:0 package*.json python-requirements.lock /opt/aries-presentation/
RUN cd /opt/aries-presentation \
    && if [ ! -f package-lock.json ]; then npm install --package-lock-only --ignore-scripts --no-audit --no-fund; fi \
    && npm ci --omit=dev --ignore-scripts --no-audit --no-fund --cache=/tmp/aries-npm-cache \
    && uv pip install --python /opt/jiuwenswarm/.venv/bin/python --no-deps --require-hashes -r python-requirements.lock \
    && node --version > node-version.txt \
    && npm --version > npm-version.txt \
    && npm ls --all --json > npm-installed.json \
    && chmod -R a+rX /opt/aries-presentation \
    && rm -rf /tmp/aries-npm-cache /root/.npm /root/.cache/uv
# Debian's font package creates this directory as root:staff. Keep the existing
# native image ownership contract for /usr/local without changing skill files.
RUN chown 0:0 /usr/local/share/fonts && chmod 0755 /usr/local/share/fonts
