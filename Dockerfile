# ---- frontend ----
FROM node:20-alpine AS web
WORKDIR /web
COPY frontend/package.json frontend/package-lock.json* ./
RUN npm install --no-audit --no-fund
COPY frontend/ ./
RUN npm run build

# ---- backend ----
FROM golang:1.23-alpine AS api
WORKDIR /src
COPY backend/go.mod backend/go.sum ./
RUN go mod download
COPY backend/ ./
RUN CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o /out/dashboard .

# ---- runtime ----
FROM alpine:3.20
RUN apk add --no-cache ca-certificates tzdata
WORKDIR /app
COPY --from=api /out/dashboard /app/dashboard
COPY --from=web /web/dist /app/web
ENV STATIC_DIR=/app/web LISTEN=:8080
EXPOSE 8080
ENTRYPOINT ["/app/dashboard"]
