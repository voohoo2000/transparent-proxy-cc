FROM golang:1.22-alpine AS build
WORKDIR /src
COPY go.mod ./
COPY cmd ./cmd
COPY internal ./internal
RUN go build -o /out/transparent-proxy ./cmd/transparent-proxy

FROM alpine:3.20
RUN apk add --no-cache iptables iproute2 bind-tools ca-certificates
WORKDIR /app
COPY --from=build /out/transparent-proxy /app/transparent-proxy
COPY scripts /app/scripts
ENTRYPOINT ["/app/scripts/entrypoint.sh"]
CMD ["serve"]
