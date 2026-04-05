FROM golang:1.25-alpine AS builder
WORKDIR /app
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 go build -o /parley-api ./cmd/api

FROM alpine:3.21
RUN adduser -D -h /app parley
WORKDIR /app
COPY --from=builder /parley-api .
USER parley
EXPOSE 8080
CMD ["./parley-api"]
