# Multi-stage build: compiles inside the container so it works cross-platform
# (Mac ARM64 host building Linux AMD64 container images).

# ---- Builder ----
FROM golang:1.26-alpine AS builder

RUN apk add --no-cache gcc musl-dev

WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=1 go build -o /kafclaw ./cmd/kafclaw

# ---- Runtime ----
FROM alpine:3.21

RUN apk add --no-cache ca-certificates git

COPY --from=builder /kafclaw /usr/local/bin/kafclaw
COPY web /app/web
COPY scripts/docker-entrypoint.sh /app/docker-entrypoint.sh
RUN chmod +x /app/docker-entrypoint.sh

# Run non-root so the image admits under PSA `restricted`. The kafclaw
# binary opens its state directory under $HOME/.kafclaw; we point
# HOME at /var/lib/kafclaw, create it with the correct ownership, and
# the chart mounts a PVC there. PLAN-06 iter-7 E-10 (BUG-0009).
RUN addgroup -g 1000 -S kafclaw \
    && adduser -u 1000 -G kafclaw -S -h /var/lib/kafclaw kafclaw \
    && mkdir -p /var/lib/kafclaw \
    && chown -R 1000:1000 /var/lib/kafclaw /app

ENV HOME=/var/lib/kafclaw
WORKDIR /app
USER 1000:1000
EXPOSE 18790 18791

ENTRYPOINT ["/app/docker-entrypoint.sh"]
CMD ["gateway"]
