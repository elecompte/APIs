FROM golang:1.23-alpine AS build
WORKDIR /src
COPY go.mod *.go ./
RUN CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o /upload-api .

FROM scratch
COPY --from=build /etc/ssl/certs/ca-certificates.crt /etc/ssl/certs/ca-certificates.crt
COPY --from=build /upload-api /upload-api
USER 1000:1000
EXPOSE 8080
ENTRYPOINT ["/upload-api"]
