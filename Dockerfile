FROM golang:1.26.1-alpine AS build

WORKDIR /src

COPY go.mod go.sum ./
RUN go mod download

COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -trimpath -ldflags="-s -w" -o /out/alienshard .

FROM alpine:3.22

RUN addgroup -S -g 1000 alienshard \
    && adduser -S -u 1000 -G alienshard -h /data alienshard \
    && mkdir -p /data \
    && chown -R alienshard:alienshard /data

COPY --from=build /out/alienshard /usr/local/bin/alienshard

USER alienshard
WORKDIR /data

EXPOSE 8000

ENTRYPOINT ["alienshard"]
CMD ["serve", "--home-dir", "/data", "--bind", "0.0.0.0", "--port", "8000"]
