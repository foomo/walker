FROM golang:1.14
WORKDIR /go/src/github.com/foomo/walker
COPY ./ ./
RUN find /go/src/github.com/foomo/walker
RUN CGO_ENABLED=0 GOOS=linux go build -o /walker cmd/walker/walker.go
RUN CGO_ENABLED=0 GOOS=linux go build -o /validator cmd/validator/validator.go

FROM alpine:latest  
RUN apk --no-cache add ca-certificates
COPY --from=0 /walker /walker
COPY --from=0 /validator /validator
COPY cmd/walker/config-example.yaml /config.yaml
CMD ["/walker", "/config.yaml"]