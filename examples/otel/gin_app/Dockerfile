FROM golang:1.12

# Based on https://hub.docker.com/_/golang/
WORKDIR /go/src/app
COPY . .
RUN go-wrapper download
RUN go-wrapper install

# Start app. APPOPTICS_SERVICE_KEY must be set to enable AppOptics.
ENV PORT=8080
CMD /go/bin/app
EXPOSE 8080
