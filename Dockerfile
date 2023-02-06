# Builder
FROM golang:alpine as builder

WORKDIR /build
COPY . /build
RUN go build

# Runner
FROM golang:alpine

ENV MYCALL=N0CALL

WORKDIR /app

EXPOSE 8080
EXPOSE 8774

# Build out directory structure
RUN mkdir mailbox
RUN mkdir standard_forms
RUN mkdir logs

COPY docker/assets/entrypoint.sh .
COPY --from=builder /build/pat .

CMD sh entrypoint.sh
