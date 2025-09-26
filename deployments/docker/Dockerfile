# Build stage
FROM golang:1.21-alpine AS builder
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -o /out/app ./cmd/appserver


# Final minimal image
FROM scratch
COPY --from=builder /out/app /app
# (Nếu cần CA certs, chuyển sang alpine:3.x hoặc add ca-certificates)
EXPOSE 8080
ENTRYPOINT ["/app"]
