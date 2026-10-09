FROM golang:1.27-alpine AS build

WORKDIR /src

COPY go.mod go.sum ./
RUN go mod download

COPY . .
RUN CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o /out/abs-opds .

FROM scratch

COPY --from=build /etc/ssl/certs/ca-certificates.crt /etc/ssl/certs/ca-certificates.crt
COPY --from=build /out/abs-opds /abs-opds

USER 65532:65532
ENV LISTEN_ADDR=:12665
EXPOSE 12665

ENTRYPOINT ["/abs-opds"]
