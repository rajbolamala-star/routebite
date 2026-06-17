FROM golang:1.21-alpine AS builder
WORKDIR /app
COPY go.mod go.sum* ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -o /routebite ./cmd/server

FROM alpine:3.19
RUN apk --no-cache add ca-certificates && adduser -D -H -u 10001 routebite
WORKDIR /app
COPY --from=builder /routebite ./routebite
USER routebite
EXPOSE 8080
CMD ["./routebite"]
