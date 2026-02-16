FROM alpine:3.20

RUN apk add --no-cache ca-certificates

WORKDIR /app

# Expect host-built binary copied into image
COPY gomikrobot /usr/local/bin/gomikrobot
COPY web /app/web

EXPOSE 18790 18791

ENTRYPOINT ["/usr/local/bin/gomikrobot"]
CMD ["gateway"]
