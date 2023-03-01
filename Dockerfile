# Builder
#FROM golang:latest as builder
FROM golang:alpine as builder

WORKDIR /build

# Cache Go deps
COPY go.mod go.sum ./
RUN go mod download

# Copy source
COPY . .

# RUN bash make.bash libax25
# RUN bash make.bash
# RUN chmod +x pat
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

COPY --from=builder /build/pat ./bin/pat

ENTRYPOINT [ "./bin/pat", "http" ]
