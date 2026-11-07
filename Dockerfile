FROM golang:1.24-alpine AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
ARG TARGETOS TARGETARCH
RUN CGO_ENABLED=0 GOOS=$TARGETOS GOARCH=$TARGETARCH go build -trimpath -ldflags="-s -w" -o /awtrix-bridge .

FROM gcr.io/distroless/static-debian12:nonroot
COPY --from=build /awtrix-bridge /awtrix-bridge
ENV CONFIG=/config/config.yaml TZ=Europe/Prague
USER nonroot
ENTRYPOINT ["/awtrix-bridge"]
