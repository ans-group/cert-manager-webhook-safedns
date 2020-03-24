FROM golang:1.13 as builder

WORKDIR /build

COPY . .
RUN go mod download

RUN CGO_ENABLED=0 go build -o webhook -ldflags '-w -extldflags "-static"' .

FROM alpine:3.9
RUN apk update \
        && apk upgrade \
        && apk add --no-cache \
        ca-certificates \
        && update-ca-certificates 2>/dev/null || true

COPY --from=builder /build/webhook /app/webhook
EXPOSE 443
ENTRYPOINT ["/app/webhook"]