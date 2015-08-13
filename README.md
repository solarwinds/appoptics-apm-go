
# TraceView for Go

## Installing

To install, you should first [sign up for a TraceView account](http://www.appneta.com/products/traceview-free-account/).

Follow the instructions during signup to install the Host Agent (“tracelyzer”). This will also install the liboboe and liboboe-dev dependencies.

Then, install the following (which assumes you are running Ubuntu/Debian):

* [Go 1.2](http://code.google.com/p/go/)

* OpenSSL libraries and dev headers: apt-get install libssl-dev

* pkg-config: apt-get install pkg-config

* This code: go get github.com/appneta/go-traceview/traceview


## Demo

If all goes well, you can run the sample “web app” included with go-traceview:

    cd $GOPATH/src/github.com/appneta/go-traceview/sample_app
    go run main.go

A web server will run on port 8899. It doesn’t do much, except wait a bit and echo back your URL path:

    $ curl http://localhost:8899/hello
    Slow request... Path: /hello

You should see these requests appear on your TraceView dashboard.  


## License

Copyright (c) 2015 Appneta

Released under the [AppNeta Open License](http://www.appneta.com/appneta-license), Version 1.0

