FROM golang:1.20-buster as builder

WORKDIR /app

COPY . .
RUN go mod download


RUN ls && CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -ldflags '-extldflags "-static"' -v -o reviewReaper

FROM alpine:3.17

COPY --from=builder /app/reviewReaper /app/reviewReaper
COPY --from=builder /app/config.yaml /app/config.yaml

CMD ["/app/reviewReaper"]