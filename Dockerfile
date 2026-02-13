FROM golang:1.26-alpine AS builder

WORKDIR /app
COPY go.mod go.sum ./
RUN go mod download
COPY . .

RUN CGO_ENABLED=0 GOOS=linux go build -ldflags="-s -w" -o sms-gateway .

FROM gcr.io/distroless/static-debian12:nonroot

WORKDIR /
COPY --from=builder /app/sms-gateway .

USER nonroot:nonroot
ENTRYPOINT ["./sms-gateway"]
