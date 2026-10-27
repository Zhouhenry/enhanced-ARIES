ARG BASE_IMAGE=swr.cn-southwest-2.myhuaweicloud.com/benchmark-zhoutao/aries-jiuwenswarm:0.2.3-e2c9d519-image5@sha256:b4078957d488e1cb52e9cc85ef5555208f0281a545c580a740a619f52127c6b1
FROM ${BASE_IMAGE}
ARG BASE_IMAGE
USER root
COPY --chown=0:0 candidate/ /opt/aries-template/process-candidate/candidate/
COPY --chown=0:0 --chmod=0444 verify.py apply_image.py /opt/aries-template/process-candidate/
RUN python3 -B -I /opt/aries-template/process-candidate/apply_image.py --base-image "${BASE_IMAGE}" \
    && python3 -B -I /opt/aries-template/verify.py > /opt/aries-template/image-evidence.json \
    && chmod 0444 /opt/aries-template/image-evidence.json
