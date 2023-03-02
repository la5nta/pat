docker run \
    --env MYCALL=N0CALL \
    -p 8080:8080 \
    -p 8774:8774 \
    --mount "type=bind,source=$(pwd)/docker/config/config.json,target=/app/config.json" \
    --mount "type=bind,source=$(pwd)/docker/data/logs,target=/app/logs" \
    --mount "type=bind,source=$(pwd)/docker/data/mailbox,target=/app/mailbox" \
    --mount "type=bind,source=$(pwd)/docker/data/standard_forms,target=/app/standard_forms" \
    --name pat \
    pat:latest