# syntax=docker/dockerfile:1

# The demo's stand-in for App Store Connect, built from this module: it offers the webhook event
# kinds internal/hooks knows.
FROM golang:1.27-alpine AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY internal ./internal
COPY dev/appstoreconnect ./dev/appstoreconnect
RUN CGO_ENABLED=0 go build -trimpath -o /out/appstoreconnect ./dev/appstoreconnect

FROM alpine:3
COPY --from=build /out/appstoreconnect /usr/local/bin/appstoreconnect
EXPOSE 443
ENTRYPOINT ["appstoreconnect"]
CMD ["serve"]
