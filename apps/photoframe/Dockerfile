# syntax=docker/dockerfile:1
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
ENTRYPOINT ["/app/photoframe"]
