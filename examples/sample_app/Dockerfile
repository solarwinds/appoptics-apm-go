FROM golang:1.9

# Based on https://hub.docker.com/_/golang/
WORKDIR /go/src/app
COPY . .
RUN go-wrapper download
RUN go-wrapper install

# Start app. APPOPTICS_SERVICE_KEY must be set to enable AppOptics.
CMD /go/bin/app -testClients true -addr :8899
EXPOSE 8899
