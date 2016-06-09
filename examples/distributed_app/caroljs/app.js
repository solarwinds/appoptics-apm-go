var tv = require('traceview');
var http = require('http');

var server = http.createServer(function(req, res) {
    console.log(new Date().toISOString(), req.method, req.url);
    res.writeHead(200);
    res.end('Hello from app.js\n');
});
server.listen(8082);
