FROM golang:1.25-alpine
WORKDIR /app

# Cache module downloads as a separate layer
COPY go.mod go.sum ./
RUN go mod download

# Source is bind-mounted at runtime; this COPY is only used when building
# without a volume mount (e.g. docker build + docker run without -v)
COPY . .

CMD ["go", "run", "./cmd/api"]
