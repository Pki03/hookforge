FROM golang:1.26-alpine AS builder

WORKDIR /app
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 go build -o /hookforge ./cmd/server

FROM alpine:3.19
RUN apk add --no-cache ca-certificates
WORKDIR /app
COPY --from=builder /hookforge /usr/local/bin/hookforge
COPY --from=builder /app/templates ./templates
COPY --from=builder /app/db ./db
COPY --from=builder /app/api ./api
EXPOSE 8080
CMD ["hookforge"]
