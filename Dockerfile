FROM golang:1.24-alpine AS build
ARG VERSION=dev
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -ldflags="-s -w -X main.version=${VERSION}" -o /probe ./cmd/probe
RUN mkdir -p /var/lib/krakenkey-probe && chown 65532:65532 /var/lib/krakenkey-probe

FROM gcr.io/distroless/static-debian12:nonroot
COPY --from=build /probe /probe
COPY probe.example.yaml /etc/krakenkey-probe/probe.yaml
COPY --from=build --chown=nonroot:nonroot /var/lib/krakenkey-probe /var/lib/krakenkey-probe
VOLUME ["/var/lib/krakenkey-probe"]
EXPOSE 8080
ENTRYPOINT ["/probe"]
CMD ["--config", "/etc/krakenkey-probe/probe.yaml"]
