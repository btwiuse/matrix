FROM btwiuse/arch:golang AS build

COPY . /app
WORKDIR /app

RUN go mod tidy && \
    CGO_ENABLED=0 go build -o /bin/matrix ./cmd/matrix && \
    CGO_ENABLED=0 go build -o /bin/inject-proxy ./cmd/inject-proxy

FROM btwiuse/arch

WORKDIR /app

COPY --from=build /bin/matrix /usr/local/bin/matrix
COPY --from=build /bin/inject-proxy /usr/local/bin/inject-proxy

ENV PORT=8080
EXPOSE 8080

CMD ["matrix"]
