FROM golang:1.27 AS build
WORKDIR /src/go
COPY go/go.mod go/go.sum ./
RUN go mod download
COPY go/ ./
RUN CGO_ENABLED=0 GOOS=linux go build -trimpath -o /walker ./cmd/walker
RUN CGO_ENABLED=0 GOOS=linux go build -trimpath -o /validator ./cmd/validator

FROM alpine:latest
RUN apk --no-cache add ca-certificates
COPY --from=build /walker /walker
COPY --from=build /validator /validator
# the example config and the demo shapes in the layout of the repository -
# a shaperoot is relative to its config file
COPY go/cmd/walker/config-example.yaml /app/go/cmd/walker/config.yaml
COPY go/internal/demoshop/shapes /app/go/internal/demoshop/shapes
CMD ["/walker", "/app/go/cmd/walker/config.yaml"]
