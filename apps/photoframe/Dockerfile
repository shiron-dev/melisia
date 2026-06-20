# syntax=docker/dockerfile:1
# multi-arch cross-compile のため意図的に --platform=$BUILDPLATFORM を使う。
# kics-scan ignore-line
FROM --platform=$BUILDPLATFORM golang:1.26.4-alpine AS build
ARG TARGETOS
ARG TARGETARCH
WORKDIR /src
COPY go.mod ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 GOOS=${TARGETOS} GOARCH=${TARGETARCH} GOFLAGS=-trimpath \
    go build -ldflags="-s -w" -o /out/photoframe .

FROM gcr.io/distroless/static-debian12:nonroot
WORKDIR /app
COPY --from=build /out/photoframe /app/photoframe
EXPOSE 8080
USER nonroot:nonroot
HEALTHCHECK --interval=30s --timeout=10s --start-period=10s --retries=3 \
    CMD ["/app/photoframe", "healthcheck"]
ENTRYPOINT ["/app/photoframe"]
