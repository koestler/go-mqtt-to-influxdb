FROM golang:1.13 as builder

COPY . /go/src/github.com/koestler/go-mqtt-to-influx

WORKDIR /go/src/github.com/koestler/go-mqtt-to-influx

RUN go get -d -v ./...

RUN CGO_ENABLED=0 GOOS=linux go build -o /go-mqtt-to-influx

FROM alpine:latest

RUN  addgroup -S app && adduser -S app -G app

USER app
WORKDIR /app
COPY --from=builder /go-mqtt-to-influx .
ENTRYPOINT ["./go-mqtt-to-influx", "-c", "/app/config.yml"]