var tv = require('traceview');
var http = require('http');

var server = http.createServer(function(req, res) {
    res.writeHead(200);
    res.end('Hello from app.js\n');
});
server.listen(8082);
