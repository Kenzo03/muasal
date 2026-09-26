# The Go build runs on the builder's own platform and cross-compiles, so
# multi-arch release images (amd64, arm64) need no emulation for this stage.
FROM --platform=$BUILDPLATFORM golang:1.27-alpine AS build
ARG TARGETOS=linux TARGETARCH
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 GOOS=$TARGETOS GOARCH=$TARGETARCH go build -trimpath -ldflags="-s -w" -o /out/app ./cmd/app && mkdir -p /out/data/attachments

FROM gcr.io/distroless/static-debian12:nonroot
COPY --from=build /out/app /app
COPY --from=build --chown=nonroot:nonroot /out/data /data
USER nonroot:nonroot
EXPOSE 8080
ENTRYPOINT ["/app"]
CMD ["serve"]
