FROM golang:1.6

# Install AppNeta package dependencies and agent.
# http://www.appneta.com/products/traceview/
# requires key arg, e.g.: docker build --build-arg accesskey=TRACEVIEW_KEY .
RUN apt-get -y install wget
ARG accesskey
RUN wget https://files.appneta.com/install_appneta.sh
RUN bash ./install_appneta.sh $accesskey
RUN service tracelyzer start

# Based on Go 1.6-onbuild Dockerfile
RUN mkdir -p /go/src/app
WORKDIR /go/src/app
COPY . /go/src/app
RUN go-wrapper download -tags traceview
RUN go-wrapper install -tags traceview

# Start tracelyzer agent running alongside app, following:
#  https://docs.appneta.com/install-instrumentation#traceview--docker
# Example running agent in a separate container: https://github.com/tlunter/tracelytics-docker
CMD service tracelyzer start & /go/bin/app -testClients true -addr :8899
EXPOSE 8899
