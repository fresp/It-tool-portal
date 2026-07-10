FROM node:22-alpine AS web-build
WORKDIR /src/apps/web
COPY apps/web/package*.json ./
RUN npm ci
COPY apps/web/ ./
RUN npm run build

FROM golang:1.23-alpine AS go-build
WORKDIR /src
RUN apk add --no-cache ca-certificates
COPY go.mod go.sum ./
RUN go mod download
COPY . ./
RUN rm -rf internal/handlers/web/dist && mkdir -p internal/handlers/web/dist
COPY --from=web-build /src/apps/web/dist/ ./internal/handlers/web/dist/
RUN CGO_ENABLED=0 GOOS=linux go build -o /out/it-tools-portal ./cmd/server

FROM alpine:3.21 AS runtime
RUN addgroup -S app && adduser -S app -G app
WORKDIR /app
RUN apk add --no-cache ca-certificates wget
COPY --from=go-build /out/it-tools-portal /app/it-tools-portal
USER app
EXPOSE 8080
HEALTHCHECK --interval=30s --timeout=5s --start-period=5s --retries=3 CMD wget -qO- http://127.0.0.1:8080/healthz >/dev/null || exit 1
CMD ["/app/it-tools-portal"]
