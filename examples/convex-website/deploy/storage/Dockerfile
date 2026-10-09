FROM chrislusf/seaweedfs:4.48@sha256:4e61d15fd35994cb1e43e1e553dff106794841fd9a99ade2fc8c8bfce4d7872d
USER root
RUN apk add --no-cache python3 py3-boto3 socat \
    && mkdir -p /data \
    && chown -R 10001:10001 /data
COPY storage.py /opt/pp/storage.py
USER 10001:10001
ENTRYPOINT ["python3", "/opt/pp/storage.py"]
