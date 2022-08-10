##
## Build
##
FROM golang:1.18-buster AS build

WORKDIR /app

COPY go.mod ./
COPY go.sum ./
RUN go mod download

COPY doproxy ./doproxy
COPY pingpong ./pingpong
COPY main.go ./

#RUN go build -o /home-reverse-proxy
RUN CGO_ENABLED=0 go build -o /home-reverse-proxy

##
## Deploy
##
#FROM gcr.io/distroless/base-debian11
FROM gcr.io/distroless/static-debian11

WORKDIR /

COPY --from=build /home-reverse-proxy /home-reverse-proxy
COPY hrp_config.yaml ./hrp_config.yaml
COPY templates ./templates

EXPOSE 8080

#USER nonroot:nonroot

ENTRYPOINT ["/home-reverse-proxy"]
