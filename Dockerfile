# Build stage
FROM golang:1.21-alpine AS builder

WORKDIR /workspace

# Download dependencies first to leverage layer caching
COPY go.mod go.sum ./
RUN go mod download

# Copy source and build
COPY . .
RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 \
    go build -a -ldflags="-w -s" -o manager .

# Runtime stage — distroless has no shell, reduces attack surface
FROM gcr.io/distroless/static:nonroot

WORKDIR /
COPY --from=builder /workspace/manager .

USER 65532:65532

ENTRYPOINT ["/manager"]
