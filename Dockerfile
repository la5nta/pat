FROM golang:alpine as builder

WORKDIR /app
COPY . /app
RUN go build


FROM golang:alpine
ENV MYCALL=N0CALL

WORKDIR /app

EXPOSE 8080
EXPOSE 8774

RUN mkdir /app/mailbox
RUN mkdir /app/standard_forms
RUN mkdir /app/logs
COPY docker/assets/entrypoint.sh .

COPY --from=builder /app/pat /app/pat

CMD sh entrypoint.sh
