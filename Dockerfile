# Use an official Golang runtime as a parent image
FROM golang:1.21.5-bookworm as builder

# Set the working directory to /app
WORKDIR /app

# Copy the current directory contents into the container at /app
COPY . /app

# Download and install any required dependencies
RUN go mod download

# Build the Go app statically
RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -ldflags="-w -s" -o main .

# Outer layer with binaries only
FROM scratch

COPY --from=builder /app/main /bot
COPY --from=builder /app/.env.local /.env.local
COPY --from=builder /etc/ssl/certs/ca-certificates.crt /etc/ssl/certs/

# Define the command to run the app when the container starts
CMD ["/bot"]
