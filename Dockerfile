# Start with the official Go image at version 1.23.2 to build the application
FROM golang:1.23.2 AS builder

# Set the working directory inside the container
WORKDIR /app

# Copy go.mod and go.sum files and download dependencies
COPY go.mod go.sum ./
RUN go mod download

# Copy the rest of the application code
COPY . .

# Build the application binary
RUN go build -o myapp -v ./...

# Use a minimal image to run the application
FROM alpine:latest

# Install any necessary dependencies
RUN apk --no-cache add ca-certificates

# Copy the binary from the builder stage
COPY --from=builder /app/myapp /usr/local/bin/myapp

# Expose the application port (modify if your app uses a different port)
EXPOSE 8080

# Set the entrypoint to the application binary
ENTRYPOINT ["/usr/local/bin/myapp"]
