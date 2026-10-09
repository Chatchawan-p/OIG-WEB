# syntax=docker/dockerfile:1
FROM golang:1.22-alpine AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -trimpath -ldflags="-s -w" -o /out/oig-api ./cmd/api

FROM gcr.io/distroless/static-debian12:nonroot
WORKDIR /app
COPY --from=build /out/oig-api /oig-api
COPY --chown=nonroot:nonroot data/evidence /app/data/evidence
EXPOSE 8080
USER nonroot:nonroot
ENTRYPOINT ["/oig-api"]
