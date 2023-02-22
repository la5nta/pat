# Builder
FROM golang:alpine as builder

WORKDIR /build

# Prerequisites for make.bash
#RUN apk add git perl

# Cache Go deps
COPY go.mod go.sum ./
RUN go mod download

# Copy source
COPY . .

# Maybe we can make the tests not fail on Alpine eventually?
#RUN sh make.bash libax25
RUN go build

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

COPY --from=builder /build/pat ./bin/

ENTRYPOINT [ "./bin/pat", "http" ]
