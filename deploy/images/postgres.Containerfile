FROM docker.io/pgvector/pgvector@sha256:74688072cb3566bff8c052b382af31f397de6ae8547cfca754becab91db3c907
ADD --checksum=sha256:ac2348d57b62f8ae02bb012d7b776c7aff10aa9d798b03dcde0e05ad6f53ae4e https://apt.postgresql.org/pub/repos/apt/pool/main/p/pgbackrest/pgbackrest_2.60.0-1.pgdg13+1_arm64.deb /tmp/pgbackrest.deb
RUN test "$(dpkg --print-architecture)" = arm64 \
    && printf '%s  %s\n' ac2348d57b62f8ae02bb012d7b776c7aff10aa9d798b03dcde0e05ad6f53ae4e /tmp/pgbackrest.deb | sha256sum --check --strict \
    && apt-get update \
    && apt-get install -y --no-install-recommends /tmp/pgbackrest.deb \
    && rm -rf /var/lib/apt/lists/* /tmp/pgbackrest.deb
USER 999:999
