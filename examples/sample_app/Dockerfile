FROM golang:1.9

# Based on Go 1.9-onbuild Dockerfile
RUN mkdir -p /go/src/app
WORKDIR /go/src/app
COPY . /go/src/app
RUN go-wrapper download -tags traceview
RUN go-wrapper install -tags traceview

#ENV APPOPTICS_SERVICE_KEY

# Start app
CMD /go/bin/app -testClients true -addr :8899
EXPOSE 8899
