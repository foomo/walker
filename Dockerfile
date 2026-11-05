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
COPY go/cmd/walker/config-example.yaml /config.yaml
CMD ["/walker", "/config.yaml"]
