# Stage 1: Build the Go application
FROM golang:1.24-alpine AS builder

WORKDIR /app

# Copy the Go module files and download dependencies
COPY go.mod go.sum ./
RUN go mod download

# Copy the rest of the application source code
COPY . .

# Build the application
RUN CGO_ENABLED=0 GOOS=linux go build -a -installsuffix cgo -o ledger ./cmd/ledger

# Stage 2: Create the final production image
FROM alpine:latest

WORKDIR /root/

# Install necessary tools for the entrypoint script
RUN apk add --no-cache curl jq bash netcat-openbsd

# Copy the compiled binary from the builder stage
COPY --from=builder /app/ledger .
# Copy the .env file from the builder stage to the final image
COPY --from=builder /app/.env .
# Copy the entrypoint scripts
COPY wait-for-it.sh entrypoint.sh .

# Make the entrypoint scripts executable
RUN chmod +x wait-for-it.sh entrypoint.sh

# Expose the port the app runs on
EXPOSE 8080

# Default command (can be overridden by docker-compose)
CMD ["./ledger"]
