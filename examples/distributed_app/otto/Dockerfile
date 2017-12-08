FROM golang:1.9

# Based on Go 1.9-onbuild Dockerfile
WORKDIR /go/src/app
COPY . .
RUN go-wrapper download -tags traceview
RUN go-wrapper install -tags traceview

ENV APPOPTICS_SERVICE_KEY

# Start app
CMD /go/bin/app
EXPOSE 8084
