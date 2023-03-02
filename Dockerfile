# Builder
FROM golang:alpine as builder

WORKDIR /build

# Build deps
RUN apk add --no-cache bash perl git

# Copy source
COPY . .

# Build without tests due to AX.25 issues in our build container.
ENV SKIP_TESTS=1
ENV NO_AX25=1
RUN ./make.bash

# Runner
FROM gcr.io/distroless/static-debian11

EXPOSE 8080
EXPOSE 8774

ARG BASE_PATH=/app

WORKDIR ${BASE_PATH}

# All paths lead to /app/pat.
ENV XDG_DATA_HOME=${BASE_PATH}
ENV XDG_STATE_HOME=${BASE_PATH}
ENV XDG_CONFIG_HOME=${BASE_PATH}

COPY --from=builder /build/pat ./bin/pat

ENTRYPOINT [ "./bin/pat", "http" ]
