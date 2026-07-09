# syntax=docker/dockerfile:1
FROM golang:1.22-bookworm AS build
WORKDIR /src
COPY packages/go /src/packages/go
COPY apps/inventory /src/apps/inventory
WORKDIR /src/apps/inventory
RUN go mod download && CGO_ENABLED=0 GOOS=linux go build -o /out/inventory ./cmd/inventory

FROM gcr.io/distroless/static-debian12:nonroot
COPY --from=build /out/inventory /inventory
USER nonroot:nonroot
EXPOSE 8082
ENTRYPOINT ["/inventory"]
