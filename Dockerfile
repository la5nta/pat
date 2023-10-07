FROM golang:alpine as builder
RUN apk add --no-cache git
WORKDIR /src
ADD go.mod go.sum ./
RUN go mod download
ADD . .
RUN go build -o /src/pat

FROM scratch
LABEL org.opencontainers.image.source=https://github.com/la5nta/pat
LABEL org.opencontainers.image.description="Pat - A portable Winlink client for amateur radio email"
LABEL org.opencontainers.image.licenses=MIT
COPY --from=builder /src/pat /bin/pat
USER 65534:65534
WORKDIR /app
ENV XDG_CONFIG_HOME=/app
ENV XDG_DATA_HOME=/app
ENV XDG_STATE_HOME=/app
EXPOSE 8080
ENTRYPOINT ["/bin/pat", "http", "--addr", ":8080"]
