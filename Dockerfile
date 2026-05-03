FROM golang:1.26-alpine AS build
WORKDIR /src

COPY go.mod go.sum ./
RUN go mod download

COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build \
        -trimpath \
        -ldflags='-s -w' \
        -o /out/postfix-delivery-exporter \
        .

FROM gcr.io/distroless/static-debian12:nonroot
COPY --from=build /out/postfix-delivery-exporter /usr/local/bin/postfix-delivery-exporter

EXPOSE 9620
USER nonroot:nonroot
ENTRYPOINT ["/usr/local/bin/postfix-delivery-exporter"]
