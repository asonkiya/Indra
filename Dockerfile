FROM golang:1.25-alpine AS builder

WORKDIR /app

RUN apk add --no-cache git

COPY go.mod go.sum* ./
RUN go mod download

COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -ldflags="-s -w" -o /indra ./cmd/indra

# ---

FROM alpine:3.21

RUN apk add --no-cache ca-certificates

COPY --from=builder /indra /usr/local/bin/indra

EXPOSE 4001/tcp 4001/udp

ENTRYPOINT ["indra"]
CMD ["--data", "/data"]
