# syntax=docker/dockerfile:1
FROM golang:1.22-bookworm AS build
WORKDIR /src
COPY packages/go /src/packages/go
COPY apps/payroll /src/apps/payroll
WORKDIR /src/apps/payroll
RUN go mod download && CGO_ENABLED=0 GOOS=linux go build -o /out/payroll ./cmd/payroll

FROM gcr.io/distroless/static-debian12:nonroot
COPY --from=build /out/payroll /payroll
USER nonroot:nonroot
EXPOSE 8083
ENTRYPOINT ["/payroll"]
