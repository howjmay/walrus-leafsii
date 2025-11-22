# Build stage
FROM golang:1.23-alpine AS builder

# Install build dependencies
RUN apk add --no-cache git ca-certificates tzdata

# Set working directory
WORKDIR /app

# Copy go mod and sum files
COPY go.mod go.sum ./

# Download dependencies
RUN go mod download

# Copy source code
COPY . .

# Build the applications
RUN CGO_ENABLED=0 GOOS=linux go build -a -installsuffix cgo -o /bin/fx-api ./cmd/api
RUN CGO_ENABLED=0 GOOS=linux go build -a -installsuffix cgo -o /bin/fx-indexer ./cmd/indexer
RUN CGO_ENABLED=0 GOOS=linux go build -a -installsuffix cgo -o /bin/fx-migrate ./cmd/migrate

# Final stage - API server
FROM alpine:latest AS api

RUN apk --no-cache add ca-certificates tzdata
WORKDIR /root/

COPY --from=builder /bin/fx-api .
COPY --from=builder /app/sql ./sql

EXPOSE 8080

CMD ["./fx-api"]

# Final stage - Indexer
FROM alpine:latest AS indexer

RUN apk --no-cache add ca-certificates tzdata
WORKDIR /root/

COPY --from=builder /bin/fx-indexer .

CMD ["./fx-indexer"]

# Final stage - Migration tool
FROM alpine:latest AS migrate

RUN apk --no-cache add ca-certificates tzdata
WORKDIR /root/

COPY --from=builder /bin/fx-migrate .
COPY --from=builder /app/sql ./sql

CMD ["./fx-migrate"]