# --- build stage ---
FROM golang:1.25-alpine AS builder
WORKDIR /src

COPY go.mod go.sum ./
RUN go mod download

COPY . .
RUN CGO_ENABLED=0 go build -o /out/chikitsalaya ./cmd/chikitsalaya

# --- runtime stage ---
FROM alpine:3.20
RUN apk add --no-cache ca-certificates
WORKDIR /app

COPY --from=builder /out/chikitsalaya ./chikitsalaya
# Templates and static assets are read from disk at runtime (filepath.Glob /
# http.Dir in internal/web and cmd/chikitsalaya), not go:embed, so they travel
# alongside the binary rather than being baked into it.
COPY web ./web

EXPOSE 8090
ENTRYPOINT ["./chikitsalaya"]
