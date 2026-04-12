# Use a valid stable Go version
FROM golang:1.24-alpine

# Install git (often needed for fetching go modules)
RUN apk add --no-cache git

WORKDIR /app

# Copy go.mod and go.sum first to leverage Docker caching
COPY go.mod go.sum* ./
RUN go mod download

# Copy the rest of the source code
COPY . .

# Build the application into a binary named 'server'
RUN go build -o server main.go

# Expose the port (informative only)
EXPOSE 1112

# Run the compiled binary
# We use 'sh -c' to ensure environment variables like $PORT are handled if needed
CMD ["./server"]